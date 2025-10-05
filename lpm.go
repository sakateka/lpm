// Package lpm implements a Longest Prefix Match (LPM) trie for fast IP address lookups.
//
// # Overview
//
// This package provides an efficient data structure for storing and querying IP prefixes
// (CIDR blocks) with associated values. It supports both IPv4 and IPv6 addresses and uses
// a 256-way trie structure for optimal lookup performance.
//
// # Basic Usage
//
// Create a new LPM trie and insert prefixes:
//
//	lpm := lpm.New()
//	lpm.Insert(netip.MustParsePrefix("192.168.1.0/24"), "local-network")
//	lpm.Insert(netip.MustParsePrefix("10.0.0.0/8"), "private-network")
//	lpm.Insert(netip.MustParsePrefix("2001:db8::/32"), "ipv6-network")
//
// Lookup an IP address to find the most specific matching prefix:
//
//	value, found := lpm.Lookup(netip.MustParseAddr("192.168.1.100"))
//	if found {
//	    fmt.Println("Matched:", value) // Output: Matched: local-network
//	}
//
// # Longest Prefix Match Behavior
//
// The trie implements longest prefix matching, meaning more specific prefixes
// take precedence over less specific ones:
//
//	lpm.Insert(netip.MustParsePrefix("10.0.0.0/8"), "broad")
//	lpm.Insert(netip.MustParsePrefix("10.1.0.0/16"), "specific")
//
//	value, _ := lpm.Lookup(netip.MustParseAddr("10.1.2.3"))
//	// Returns "specific" because /16 is more specific than /8
//
// # Shared Memory Support
//
// For high-performance scenarios with multiple processes, the trie can be serialized
// to shared memory:
//
//	// Process 1: Build and serialize the trie
//	lpm := lpm.New()
//	lpm.Insert(netip.MustParsePrefix("192.168.0.0/16"), "data")
//	storage, err := lpm.PackToSharedStorage()
//	// Write storage to shared memory or file
//
//	// Process 2: Load from shared memory
//	lpm, err := lpm.NewWithSharedStorage(storage)
//	value, found := lpm.Lookup(netip.MustParseAddr("192.168.1.1"))
//
// Shared storage provides zero-copy access to the trie data, making it ideal for
// read-heavy workloads across multiple processes.
//
// # Performance Characteristics
//
//   - Lookup: O(address_length) - constant time for IPv4 (4 bytes), IPv6 (16 bytes)
//   - Insert: O(address_length) - may allocate new blocks as needed
//   - Memory: Each block uses 1KB (256 × 4 bytes), allocated on-demand
//
// # Statistics
//
// Get memory usage and block allocation statistics:
//
//	stats := lpm.Stats()
//	fmt.Printf("IPv4 blocks: %d, IPv6 blocks: %d\n",
//	    stats.IPv4Blocks, stats.IPv6Blocks)
//	fmt.Printf("Total memory: %d bytes\n", stats.TotalSize)
//
// # Thread Safety
//
// The LPM trie is NOT thread-safe. External synchronization is required for
// concurrent access. For read-only workloads after initial construction,
// consider using shared memory with separate LPM instances per process.
package lpm

import (
	"fmt"
	"net/netip"
	"unsafe"
)

// Encoding scheme:
// - Invalid value: 0x00000000 (zero value)
// - Block reference: top 2 bits set to 1 (0xC0000000 | block_index)
//   * Bits 30-31: 11 (block marker)
//   * Bits 0-29: block index (supports up to ~1 billion blocks)
// - Value reference: prefix length + 1 in top byte (1-129 for IPv4/IPv6)
//   * Bits 24-31: prefix length + 1 (1-129, since max prefix is 128)
//   * Bits 0-23: value index (supports up to ~16 million values)

const (
	v4LPM = 0
	v6LPM = 1

	blockRefMask   = 0xC0000000 // Top 2 bits set to 1
	blockIndexMask = 0x3FFFFFFF // Bottom 30 bits for block index
	prefixLenShift = 24
	valueIndexMask = 0x00FFFFFF // Bottom 24 bits for value index

	blockSize = 256

	magicNumber    = 0x4C504D00 // "LPM\0"
	currentVersion = 1
)

// StorageHeader describes the layout of preallocated storage
type StorageHeader struct {
	Magic          uint32 // Magic number: 0x4C504D00 ("LPM\0")
	Version        uint32 // Format version
	V4BlockCount   uint32 // Number of IPv4 blocks
	V6BlockCount   uint32 // Number of IPv6 blocks
	ValueCount     uint32 // Number of preallocated values
	ValueSlotSize  uint32 // Size of each value slot in bytes
	V4BlocksOffset uint32 // Offset to IPv4 blocks data
	V6BlocksOffset uint32 // Offset to IPv6 blocks data
	ValuesOffset   uint32 // Offset to values data
}

type LPMBlock [blockSize]uint32

type LPM struct {
	shared               [2][]LPMBlock
	sharedValues         []byte
	sharedValuesSlotSize int
	sharedValueCount     int

	dynamic   [2][]*LPMBlock
	values    map[string]int // value -> index
	revValues []string       // index -> value
}

func New() *LPM {
	return &LPM{
		dynamic: [2][]*LPMBlock{{{}}, {{}}},
		values:  make(map[string]int),
	}
}

// NewWithSharedStorage creates a new LPM instance with shared storage from a byte slice.
// The storage must start with a StorageHeader followed by the data sections.
func NewWithSharedStorage(storage []byte) (*LPM, error) {
	if len(storage) < int(unsafe.Sizeof(StorageHeader{})) {
		return nil, fmt.Errorf("storage too small: need at least %d bytes for header, got %d",
			unsafe.Sizeof(StorageHeader{}), len(storage))
	}

	// Cast byte slice to header struct
	header := (*StorageHeader)(unsafe.Pointer(&storage[0]))

	// Validate header
	if header.Magic != magicNumber {
		return nil, fmt.Errorf("invalid magic number: expected 0x%08X, got 0x%08X", magicNumber, header.Magic)
	}

	if header.Version != currentVersion {
		return nil, fmt.Errorf("unsupported version: expected %d, got %d", currentVersion, header.Version)
	}

	// Validate offsets and sizes
	blockByteSize := blockSize * 4 // 256 uint32s = 1024 bytes per block

	if header.V4BlockCount > 0 {
		requiredSize := int(header.V4BlocksOffset) + (int(header.V4BlockCount) * blockByteSize)
		if len(storage) < requiredSize {
			return nil, fmt.Errorf("storage too small for IPv4 blocks: need %d bytes, got %d", requiredSize, len(storage))
		}
	}

	if header.V6BlockCount > 0 {
		requiredSize := int(header.V6BlocksOffset) + (int(header.V6BlockCount) * blockByteSize)
		if len(storage) < requiredSize {
			return nil, fmt.Errorf("storage too small for IPv6 blocks: need %d bytes, got %d", requiredSize, len(storage))
		}
	}

	if header.ValueCount > 0 && header.ValueSlotSize > 0 {
		requiredSize := int(header.ValuesOffset) + (int(header.ValueCount) * int(header.ValueSlotSize))
		if len(storage) < requiredSize {
			return nil, fmt.Errorf("storage too small for values: need %d bytes, got %d", requiredSize, len(storage))
		}
	}

	// Create LPM instance
	lpm := &LPM{
		sharedValuesSlotSize: int(header.ValueSlotSize),
		sharedValueCount:     int(header.ValueCount),
		values:               make(map[string]int),
	}

	// Map IPv4 blocks using unsafe pointer casting
	if header.V4BlockCount > 0 {
		v4Data := storage[header.V4BlocksOffset:]
		lpm.shared[v4LPM] = unsafe.Slice((*LPMBlock)(unsafe.Pointer(&v4Data[0])), header.V4BlockCount)
		lpm.dynamic[v4LPM] = []*LPMBlock{}
	} else {
		lpm.dynamic[v4LPM] = []*LPMBlock{{}}
	}

	// Map IPv6 blocks using unsafe pointer casting
	if header.V6BlockCount > 0 {
		v6Data := storage[header.V6BlocksOffset:]
		lpm.shared[v6LPM] = unsafe.Slice((*LPMBlock)(unsafe.Pointer(&v6Data[0])), header.V6BlockCount)
		lpm.dynamic[v6LPM] = []*LPMBlock{}
	} else {
		lpm.dynamic[v6LPM] = []*LPMBlock{{}}
	}

	// Map values
	if header.ValueCount > 0 && header.ValueSlotSize > 0 {
		valuesEnd := int(header.ValuesOffset) + (int(header.ValueCount) * int(header.ValueSlotSize))
		lpm.sharedValues = storage[header.ValuesOffset:valuesEnd]
	}

	return lpm, nil
}

// PackToSharedStorage serializes the LPM trie into a byte slice suitable for shared memory.
// The returned byte slice contains a StorageHeader followed by the block and value data.
// It automatically determines the maximum value length and returns an error if any value exceeds 255 bytes.
func (m *LPM) PackToSharedStorage() ([]byte, error) {
	// Find maximum value length and validate
	maxValueLen := 0

	// Check shared values
	if m.sharedValueCount > 0 && len(m.sharedValues) > 0 {
		for i := 0; i < m.sharedValueCount; i++ {
			offset := i * m.sharedValuesSlotSize
			strLen := int(m.sharedValues[offset])
			if strLen > 255 {
				return nil, fmt.Errorf("shared value at index %d exceeds 255 bytes: %d", i, strLen)
			}
			if strLen > maxValueLen {
				maxValueLen = strLen
			}
		}
	}

	// Check dynamic values
	for i, val := range m.revValues {
		if len(val) > 255 {
			return nil, fmt.Errorf("value at index %d exceeds 255 bytes: %d", i, len(val))
		}
		if len(val) > maxValueLen {
			maxValueLen = len(val)
		}
	}

	// Calculate sizes
	headerSize := int(unsafe.Sizeof(StorageHeader{}))
	blockByteSize := blockSize * 4   // 256 uint32s = 1024 bytes per block
	valueSlotSize := maxValueLen + 1 // +1 for length byte

	v4BlockCount := len(m.shared[v4LPM]) + len(m.dynamic[v4LPM])
	v6BlockCount := len(m.shared[v6LPM]) + len(m.dynamic[v6LPM])
	valueCount := m.sharedValueCount + len(m.revValues)

	// Calculate offsets
	v4BlocksOffset := headerSize
	v6BlocksOffset := v4BlocksOffset + (v4BlockCount * blockByteSize)
	valuesOffset := v6BlocksOffset + (v6BlockCount * blockByteSize)
	totalSize := valuesOffset + (valueCount * valueSlotSize)

	// Allocate storage
	storage := make([]byte, totalSize)

	// Write header
	header := (*StorageHeader)(unsafe.Pointer(&storage[0]))
	header.Magic = magicNumber
	header.Version = currentVersion
	header.V4BlockCount = uint32(v4BlockCount)
	header.V6BlockCount = uint32(v6BlockCount)
	header.ValueCount = uint32(valueCount)
	header.ValueSlotSize = uint32(valueSlotSize)
	header.V4BlocksOffset = uint32(v4BlocksOffset)
	header.V6BlocksOffset = uint32(v6BlocksOffset)
	header.ValuesOffset = uint32(valuesOffset)

	// Write IPv4 blocks
	offset := v4BlocksOffset
	for i := 0; i < len(m.shared[v4LPM]); i++ {
		block := &m.shared[v4LPM][i]
		blockBytes := unsafe.Slice((*byte)(unsafe.Pointer(&block[0])), blockByteSize)
		copy(storage[offset:offset+blockByteSize], blockBytes)
		offset += blockByteSize
	}
	for i := 0; i < len(m.dynamic[v4LPM]); i++ {
		block := m.dynamic[v4LPM][i]
		blockBytes := unsafe.Slice((*byte)(unsafe.Pointer(&block[0])), blockByteSize)
		copy(storage[offset:offset+blockByteSize], blockBytes)
		offset += blockByteSize
	}

	// Write IPv6 blocks
	offset = v6BlocksOffset
	for i := 0; i < len(m.shared[v6LPM]); i++ {
		block := &m.shared[v6LPM][i]
		blockBytes := unsafe.Slice((*byte)(unsafe.Pointer(&block[0])), blockByteSize)
		copy(storage[offset:offset+blockByteSize], blockBytes)
		offset += blockByteSize
	}
	for i := 0; i < len(m.dynamic[v6LPM]); i++ {
		block := m.dynamic[v6LPM][i]
		blockBytes := unsafe.Slice((*byte)(unsafe.Pointer(&block[0])), blockByteSize)
		copy(storage[offset:offset+blockByteSize], blockBytes)
		offset += blockByteSize
	}

	// Write values
	offset = valuesOffset

	// Write shared values first
	if m.sharedValueCount > 0 && len(m.sharedValues) > 0 {
		for i := 0; i < m.sharedValueCount; i++ {
			srcOffset := i * m.sharedValuesSlotSize
			strLen := int(m.sharedValues[srcOffset])
			storage[offset] = byte(strLen)
			copy(storage[offset+1:offset+1+strLen], m.sharedValues[srcOffset+1:srcOffset+1+strLen])
			offset += valueSlotSize
		}
	}

	// Write dynamic values
	for _, val := range m.revValues {
		storage[offset] = byte(len(val))
		copy(storage[offset+1:], []byte(val))
		offset += valueSlotSize
	}

	return storage, nil
}

func blockWithValue(initValue uint32) *LPMBlock {
	blk := &LPMBlock{}

	if isInvalid(initValue) {
		return blk
	}

	for idx := range blk {
		blk[idx] = initValue
	}
	return blk
}

func (m *LPM) addValue(value string) int {
	if valueIdx, ok := m.values[value]; ok {
		return valueIdx
	}
	valueIdx := m.sharedValueCount + len(m.revValues)
	m.values[value] = valueIdx
	m.revValues = append(m.revValues, value)
	return valueIdx
}

// getValueByIndex retrieves a value by its index, supporting both shared and dynamic values
func (m *LPM) getValueByIndex(valueIdx int) (string, bool) {
	if valueIdx < m.sharedValueCount {
		// Value is in shared storage
		if m.sharedValues == nil || m.sharedValuesSlotSize == 0 {
			return "", false
		}
		offset := valueIdx * m.sharedValuesSlotSize
		if offset+m.sharedValuesSlotSize > len(m.sharedValues) {
			return "", false
		}
		// First byte contains the string length
		strLen := int(m.sharedValues[offset])
		if strLen == 0 || offset+1+strLen > len(m.sharedValues) {
			return "", false
		}
		return string(m.sharedValues[offset+1 : offset+1+strLen]), true
	}
	// Value is in dynamic storage
	dynamicIdx := valueIdx - m.sharedValueCount
	if dynamicIdx >= len(m.revValues) {
		return "", false
	}
	return m.revValues[dynamicIdx], true
}

// encodeValue encodes a value index with its prefix length
func encodeValue(valueIdx int, prefixLen int) uint32 {
	// Store (prefixLen + 1) in the most significant byte
	return (uint32(prefixLen+1) << prefixLenShift) | uint32(valueIdx)
}

// encodeBlockRef encodes a block index as a block reference
func encodeBlockRef(blockIdx int) uint32 {
	return blockRefMask | uint32(blockIdx)
}

// decodeValue extracts the value index and prefix length from an encoded value
func decodeValue(encoded uint32) (valueIdx int, prefixLen int) {
	prefixLen = int(encoded>>prefixLenShift) - 1
	valueIdx = int(encoded & valueIndexMask)
	return
}

// decodeBlockRef extracts the block index from a block reference
func decodeBlockRef(encoded uint32) int {
	return int(encoded & blockIndexMask)
}

// isBlockRef checks if the encoded value is a block reference (top 2 bits are 11)
func isBlockRef(encoded uint32) bool {
	return (encoded & blockRefMask) == blockRefMask
}

// isInvalid checks if the slot is invalid (zero value)
func isInvalid(encoded uint32) bool {
	return encoded == 0
}

func (m *LPM) getBlockRef(proto int, blockIdx int) *LPMBlock {
	sharedLen := len(m.shared[proto])
	if blockIdx < sharedLen {
		return &m.shared[proto][blockIdx]
	}
	return m.dynamic[proto][blockIdx-sharedLen]
}

func (m *LPM) getValue(proto int, block int, slot uint8) uint32 {
	sharedLen := len(m.shared[proto])
	if block < sharedLen {
		return m.shared[proto][block][slot]
	}
	return m.dynamic[proto][block-sharedLen][slot]
}

func (m *LPM) setValue(proto int, block int, slot uint8, value uint32) {
	sharedLen := len(m.shared[proto])
	if block < sharedLen {
		m.shared[proto][block][slot] = value
		return
	}
	m.dynamic[proto][block-sharedLen][slot] = value
}

func (m *LPM) propagateValue(proto int, blockIdx int, valueIdx int, prefixLen int, startIdx, endIdx uint8) {
	// Propagate the value to all slots in the range [startIdx, endIdx]
	newValue := encodeValue(valueIdx, prefixLen)
	for inBlockIdx := int(startIdx); inBlockIdx <= int(endIdx); inBlockIdx++ {
		currentVal := m.getValue(proto, blockIdx, uint8(inBlockIdx))

		if isBlockRef(currentVal) {
			// Block reference for a narrower subnet - propagate into it
			// but only fill invalid slots (don't override existing values)
			innerBlockIdx := decodeBlockRef(currentVal)
			innerBlockRef := m.getBlockRef(proto, innerBlockIdx)
			for idx, val := range innerBlockRef {
				if isInvalid(val) {
					innerBlockRef[idx] = newValue
				}
			}
		} else if isInvalid(currentVal) {
			m.setValue(proto, blockIdx, uint8(inBlockIdx), newValue)
		} else {
			// It's a value - check if our prefix is longer (more specific) or equal
			_, existingPrefixLen := decodeValue(currentVal)
			if prefixLen >= existingPrefixLen {
				// Our prefix is more specific or equal, override
				m.setValue(proto, blockIdx, uint8(inBlockIdx), newValue)
			}
			// If existing prefix is more specific, keep it
		}
	}
}

func (m *LPM) Insert(net netip.Prefix, value string) {
	valueIdx := m.addValue(value)

	proto := v4LPM
	if net.Addr().Is6() {
		proto = v6LPM
	}

	prefixLen := net.Bits()

	blockIdx := 0
	// Insertion process
	for idx, inBlockIdx := range net.Addr().AsSlice() {
		tail := int((idx+1)*8) - prefixLen
		if tail >= 0 {
			// This is the last byte - propagate to the range
			mask := uint8(0xff << tail)
			startIdx := inBlockIdx & mask
			endIdx := startIdx | ^mask

			m.propagateValue(proto, blockIdx, valueIdx, prefixLen, startIdx, endIdx)
			return
		}

		currentVal := m.getValue(proto, blockIdx, inBlockIdx)

		if isBlockRef(currentVal) {
			// Already a block reference, continue traversal
			blockIdx = decodeBlockRef(currentVal)
		} else {
			// Need to create a new block
			// Remember the old value (could be invalid or a value)
			oldVal := currentVal

			// Create new block
			newBlockIdx := len(m.shared[proto]) + len(m.dynamic[proto])
			m.setValue(proto, blockIdx, inBlockIdx, encodeBlockRef(newBlockIdx))

			// Initialize new block
			blk := blockWithValue(oldVal)
			// Add new block to the tree
			m.dynamic[proto] = append(m.dynamic[proto], blk)
			blockIdx = newBlockIdx
		}
	}
}

func (m *LPM) Lookup(addr netip.Addr) (string, bool) {
	proto := v4LPM
	if addr.Is6() {
		proto = v6LPM
	}

	blockIdx := 0
	for _, inBlockIdx := range addr.AsSlice() {
		value := m.getValue(proto, blockIdx, inBlockIdx)

		if isBlockRef(value) {
			// Continue traversal
			blockIdx = decodeBlockRef(value)
		} else if isInvalid(value) {
			// No match
			return "", false
		} else {
			// Found a value
			valueIdx, _ := decodeValue(value)
			return m.getValueByIndex(valueIdx)
		}
	}
	return "", false
}

// Stats contains statistics about the LPM trie
type Stats struct {
	IPv4Blocks      int // Number of blocks allocated for IPv4
	IPv6Blocks      int // Number of blocks allocated for IPv6
	IPv4StorageSize int // Storage size in bytes for IPv4 trie
	IPv6StorageSize int // Storage size in bytes for IPv6 trie
	ValuesStorage   int // Storage size in bytes for values
	TotalSize       int // Total storage size in bytes
}

// Stats returns statistics about the LPM trie including block counts and storage sizes
func (m *LPM) Stats() Stats {
	// Count shared blocks
	v4SharedLen := len(m.shared[v4LPM])
	v6SharedLen := len(m.shared[v6LPM])

	// Count dynamic blocks
	v4DynamicLen := len(m.dynamic[v4LPM])
	v6DynamicLen := len(m.dynamic[v6LPM])

	// Total blocks
	v4TotalLen := v4SharedLen + v4DynamicLen
	v6TotalLen := v6SharedLen + v6DynamicLen

	// Calculate IPv4 storage size
	v4StorageSize := 0
	// Shared blocks: just the block data (stored in shared memory, no Go overhead)
	if v4SharedLen > 0 {
		v4StorageSize += v4SharedLen * blockSize * 4 // 256 uint32s per block
	}
	// Dynamic blocks: slice overhead + block data + pointer overhead
	if v4DynamicLen > 0 {
		v4StorageSize += 3 * 8                        // slice header (ptr, len, cap)
		v4StorageSize += v4DynamicLen * blockSize * 4 // block data
		v4StorageSize += v4DynamicLen * 8             // pointers to blocks
	}

	// Calculate IPv6 storage size
	v6StorageSize := 0
	// Shared blocks: just the block data (stored in shared memory, no Go overhead)
	if v6SharedLen > 0 {
		v6StorageSize += v6SharedLen * blockSize * 4 // 256 uint32s per block
	}
	// Dynamic blocks: slice overhead + block data + pointer overhead
	if v6DynamicLen > 0 {
		v6StorageSize += 3 * 8                        // slice header (ptr, len, cap)
		v6StorageSize += v6DynamicLen * blockSize * 4 // block data
		v6StorageSize += v6DynamicLen * 8             // pointers to blocks
	}

	// Calculate values storage size
	valStorageSize := 0

	// Shared values: stored in shared memory
	if m.sharedValueCount > 0 && m.sharedValuesSlotSize > 0 {
		valStorageSize += m.sharedValueCount * m.sharedValuesSlotSize
	}

	// Dynamic values: string data + Go overhead
	if len(m.revValues) > 0 {
		// String data
		for _, val := range m.revValues {
			valStorageSize += len(val)
		}
		// String struct overhead (ptr, len) for each string
		valStorageSize += len(m.revValues) * 2 * 8
		// revValues slice overhead
		valStorageSize += 3 * 8
		// values map overhead (approximate: map header + entries)
		valStorageSize += 8 * 8                 // map header
		valStorageSize += len(m.revValues) * 32 // approximate per-entry overhead
		valStorageSize += len(m.revValues) * 4  // int values in map
	}

	return Stats{
		IPv4Blocks:      v4TotalLen,
		IPv6Blocks:      v6TotalLen,
		IPv4StorageSize: v4StorageSize,
		IPv6StorageSize: v6StorageSize,
		ValuesStorage:   valStorageSize,
		TotalSize:       v4StorageSize + v6StorageSize + valStorageSize,
	}
}
