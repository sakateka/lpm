package lpm

import (
	"net/netip"
	"os"
	"path/filepath"
	"testing"
)

// TestSharedStoragePackAndLoad tests packing LPM to shared storage and loading it back
func TestSharedStoragePackAndLoad(t *testing.T) {
	// Create and populate an LPM
	lpm := New()

	testData := []struct {
		prefix string
		value  string
	}{
		{"192.168.1.0/24", "subnet1"},
		{"192.168.2.0/24", "subnet2"},
		{"10.0.0.0/8", "private"},
		{"2001:db8::/32", "ipv6-subnet"},
	}

	for _, td := range testData {
		prefix := netip.MustParsePrefix(td.prefix)
		lpm.Insert(prefix, td.value)
	}

	// Pack to shared storage
	storage, err := lpm.PackToSharedStorage()
	if err != nil {
		t.Fatalf("PackToSharedStorage failed: %v", err)
	}

	// Load from shared storage
	lpm2, err := NewWithSharedStorage(storage)
	if err != nil {
		t.Fatalf("NewWithSharedStorage failed: %v", err)
	}

	// Test lookups on the loaded LPM
	testLookups := []struct {
		addr     string
		expected string
		found    bool
	}{
		{"192.168.1.1", "subnet1", true},
		{"192.168.2.100", "subnet2", true},
		{"10.5.5.5", "private", true},
		{"2001:db8::1", "ipv6-subnet", true},
		{"8.8.8.8", "", false},
	}

	for _, tl := range testLookups {
		addr := netip.MustParseAddr(tl.addr)
		value, found := lpm2.Lookup(addr)

		if found != tl.found {
			t.Errorf("Lookup(%s): expected found=%v, got %v", tl.addr, tl.found, found)
		}
		if value != tl.expected {
			t.Errorf("Lookup(%s): expected value=%q, got %q", tl.addr, tl.expected, value)
		}
	}
}

// TestSharedStorageWithDynamicInserts tests inserting new items into shared storage LPM
func TestSharedStorageWithDynamicInserts(t *testing.T) {
	// Create and populate an LPM
	lpm := New()
	lpm.Insert(netip.MustParsePrefix("192.168.0.0/16"), "base")

	// Pack to shared storage
	storage, err := lpm.PackToSharedStorage()
	if err != nil {
		t.Fatalf("PackToSharedStorage failed: %v", err)
	}

	// Load from shared storage
	lpm2, err := NewWithSharedStorage(storage)
	if err != nil {
		t.Fatalf("NewWithSharedStorage failed: %v", err)
	}

	// Insert new items into the loaded LPM
	lpm2.Insert(netip.MustParsePrefix("192.168.1.0/24"), "subnet1")
	lpm2.Insert(netip.MustParsePrefix("192.168.2.0/24"), "subnet2")
	lpm2.Insert(netip.MustParsePrefix("10.0.0.0/8"), "private")

	// Test lookups - should find both shared and dynamic entries
	testLookups := []struct {
		addr     string
		expected string
	}{
		{"192.168.0.1", "base"},    // From shared storage
		{"192.168.1.1", "subnet1"}, // Newly inserted
		{"192.168.2.1", "subnet2"}, // Newly inserted
		{"10.5.5.5", "private"},    // Newly inserted
		{"192.168.100.1", "base"},  // Falls back to base from shared storage
	}

	for _, tl := range testLookups {
		addr := netip.MustParseAddr(tl.addr)
		value, found := lpm2.Lookup(addr)

		if !found {
			t.Errorf("Lookup(%s): expected to find value, got not found", tl.addr)
		}
		if value != tl.expected {
			t.Errorf("Lookup(%s): expected value=%q, got %q", tl.addr, tl.expected, value)
		}
	}
}

// TestSharedStorageReadOnly tests using shared storage without modifications
func TestSharedStorageReadOnly(t *testing.T) {
	// Create and populate an LPM with various prefixes
	lpm := New()

	prefixes := []struct {
		prefix string
		value  string
	}{
		{"0.0.0.0/0", "default"},
		{"10.0.0.0/8", "private-10"},
		{"172.16.0.0/12", "private-172"},
		{"192.168.0.0/16", "private-192"},
		{"8.8.8.0/24", "public-dns-1"},
		{"1.1.1.0/24", "public-dns-2"},
	}

	for _, p := range prefixes {
		lpm.Insert(netip.MustParsePrefix(p.prefix), p.value)
	}

	// Pack to shared storage
	storage, err := lpm.PackToSharedStorage()
	if err != nil {
		t.Fatalf("PackToSharedStorage failed: %v", err)
	}

	// Load from shared storage
	lpm2, err := NewWithSharedStorage(storage)
	if err != nil {
		t.Fatalf("NewWithSharedStorage failed: %v", err)
	}

	// Test lookups without any modifications
	testCases := []struct {
		addr     string
		expected string
	}{
		{"10.1.2.3", "private-10"},
		{"172.16.5.1", "private-172"},
		{"192.168.1.1", "private-192"},
		{"8.8.8.8", "public-dns-1"},
		{"1.1.1.1", "public-dns-2"},
		{"100.100.100.100", "default"},
	}

	for _, tc := range testCases {
		addr := netip.MustParseAddr(tc.addr)
		value, found := lpm2.Lookup(addr)

		if !found {
			t.Errorf("Lookup(%s): expected to find value, got not found", tc.addr)
		}
		if value != tc.expected {
			t.Errorf("Lookup(%s): expected value=%q, got %q", tc.addr, tc.expected, value)
		}
	}
}

// TestSharedStorageIPv6 tests IPv6 prefixes in shared storage
func TestSharedStorageIPv6(t *testing.T) {
	lpm := New()

	prefixes := []struct {
		prefix string
		value  string
	}{
		{"::/0", "default-v6"},
		{"2001:db8::/32", "documentation"},
		{"2001:db8:1::/48", "subnet1"},
		{"2001:db8:2::/48", "subnet2"},
		{"fe80::/10", "link-local"},
	}

	for _, p := range prefixes {
		lpm.Insert(netip.MustParsePrefix(p.prefix), p.value)
	}

	// Pack and reload
	storage, err := lpm.PackToSharedStorage()
	if err != nil {
		t.Fatalf("PackToSharedStorage failed: %v", err)
	}

	lpm2, err := NewWithSharedStorage(storage)
	if err != nil {
		t.Fatalf("NewWithSharedStorage failed: %v", err)
	}

	// Test lookups
	testCases := []struct {
		addr     string
		expected string
	}{
		{"2001:db8::1", "documentation"},
		{"2001:db8:1::1", "subnet1"},
		{"2001:db8:2::1", "subnet2"},
		{"fe80::1", "link-local"},
		{"2001:4860::1", "default-v6"},
	}

	for _, tc := range testCases {
		addr := netip.MustParseAddr(tc.addr)
		value, found := lpm2.Lookup(addr)

		if !found {
			t.Errorf("Lookup(%s): expected to find value, got not found", tc.addr)
		}
		if value != tc.expected {
			t.Errorf("Lookup(%s): expected value=%q, got %q", tc.addr, tc.expected, value)
		}
	}
}

// TestSharedStorageStats tests that Stats() correctly reports shared and dynamic storage
func TestSharedStorageStats(t *testing.T) {
	// Create LPM with some data
	lpm := New()
	lpm.Insert(netip.MustParsePrefix("192.168.0.0/16"), "base")
	lpm.Insert(netip.MustParsePrefix("10.0.0.0/8"), "private")

	stats1 := lpm.Stats()

	// Pack and reload
	storage, err := lpm.PackToSharedStorage()
	if err != nil {
		t.Fatalf("PackToSharedStorage failed: %v", err)
	}

	lpm2, err := NewWithSharedStorage(storage)
	if err != nil {
		t.Fatalf("NewWithSharedStorage failed: %v", err)
	}

	stats2 := lpm2.Stats()

	// Stats should be similar (shared storage might have slightly different overhead)
	if stats2.IPv4Blocks != stats1.IPv4Blocks {
		t.Errorf("IPv4Blocks mismatch: original=%d, loaded=%d", stats1.IPv4Blocks, stats2.IPv4Blocks)
	}

	// Now insert new data
	lpm2.Insert(netip.MustParsePrefix("172.16.0.0/12"), "new-private")

	stats3 := lpm2.Stats()

	// Should have more blocks now
	if stats3.IPv4Blocks <= stats2.IPv4Blocks {
		t.Errorf("Expected more blocks after insert: before=%d, after=%d", stats2.IPv4Blocks, stats3.IPv4Blocks)
	}
}

// TestSharedStorageValueTooLong tests that values exceeding 255 bytes are rejected
func TestSharedStorageValueTooLong(t *testing.T) {
	lpm := New()

	// Create a value that's too long (> 255 bytes)
	longValue := string(make([]byte, 256))
	for i := range longValue {
		longValue = longValue[:i] + "a"
	}

	lpm.Insert(netip.MustParsePrefix("192.168.1.0/24"), longValue)

	// Should fail to pack
	_, err := lpm.PackToSharedStorage()
	if err == nil {
		t.Error("Expected error for value exceeding 255 bytes, got nil")
	}
}

// TestSharedStorageEmptyLPM tests packing and loading an empty LPM
func TestSharedStorageEmptyLPM(t *testing.T) {
	lpm := New()

	// Pack empty LPM
	storage, err := lpm.PackToSharedStorage()
	if err != nil {
		t.Fatalf("PackToSharedStorage failed: %v", err)
	}

	// Load it back
	lpm2, err := NewWithSharedStorage(storage)
	if err != nil {
		t.Fatalf("NewWithSharedStorage failed: %v", err)
	}

	// Should not find anything
	addr := netip.MustParseAddr("192.168.1.1")
	_, found := lpm2.Lookup(addr)
	if found {
		t.Error("Expected not to find anything in empty LPM")
	}

	// Should be able to insert
	lpm2.Insert(netip.MustParsePrefix("192.168.1.0/24"), "test")
	value, found := lpm2.Lookup(addr)
	if !found || value != "test" {
		t.Errorf("Expected to find 'test', got found=%v, value=%q", found, value)
	}
}

// TestSharedStorageMixedProtocols tests both IPv4 and IPv6 in shared storage
func TestSharedStorageMixedProtocols(t *testing.T) {
	lpm := New()

	// Add both IPv4 and IPv6 prefixes
	lpm.Insert(netip.MustParsePrefix("192.168.0.0/16"), "ipv4-private")
	lpm.Insert(netip.MustParsePrefix("10.0.0.0/8"), "ipv4-private-10")
	lpm.Insert(netip.MustParsePrefix("2001:db8::/32"), "ipv6-doc")
	lpm.Insert(netip.MustParsePrefix("fe80::/10"), "ipv6-link-local")

	// Pack and reload
	storage, err := lpm.PackToSharedStorage()
	if err != nil {
		t.Fatalf("PackToSharedStorage failed: %v", err)
	}

	lpm2, err := NewWithSharedStorage(storage)
	if err != nil {
		t.Fatalf("NewWithSharedStorage failed: %v", err)
	}

	// Test both protocols
	v4Tests := []struct {
		addr     string
		expected string
	}{
		{"192.168.1.1", "ipv4-private"},
		{"10.5.5.5", "ipv4-private-10"},
	}

	for _, tc := range v4Tests {
		addr := netip.MustParseAddr(tc.addr)
		value, found := lpm2.Lookup(addr)
		if !found || value != tc.expected {
			t.Errorf("IPv4 Lookup(%s): expected %q, got found=%v, value=%q", tc.addr, tc.expected, found, value)
		}
	}

	v6Tests := []struct {
		addr     string
		expected string
	}{
		{"2001:db8::1", "ipv6-doc"},
		{"fe80::1", "ipv6-link-local"},
	}

	for _, tc := range v6Tests {
		addr := netip.MustParseAddr(tc.addr)
		value, found := lpm2.Lookup(addr)
		if !found || value != tc.expected {
			t.Errorf("IPv6 Lookup(%s): expected %q, got found=%v, value=%q", tc.addr, tc.expected, found, value)
		}
	}
}

// TestSharedStoragePersistence tests writing storage to file and loading it back
func TestSharedStoragePersistence(t *testing.T) {
	// Create LPM with mixed protocols
	lpm := New()

	testData := []struct {
		prefix string
		value  string
	}{
		// IPv4 prefixes
		{"0.0.0.0/0", "default-v4"},
		{"10.0.0.0/8", "private-10"},
		{"172.16.0.0/12", "private-172"},
		{"192.168.0.0/16", "private-192"},
		{"8.8.8.0/24", "public-dns-1"},
		{"1.1.1.0/24", "public-dns-2"},
		{"192.168.1.0/24", "subnet1"},
		{"192.168.2.0/24", "subnet2"},
		// IPv6 prefixes
		{"::/0", "default-v6"},
		{"2001:db8::/32", "documentation"},
		{"2001:db8:1::/48", "doc-subnet1"},
		{"2001:db8:2::/48", "doc-subnet2"},
		{"fe80::/10", "link-local"},
		{"2001:4860::/32", "public-v6-1"},
		{"2606:4700::/32", "public-v6-2"},
	}

	for _, td := range testData {
		prefix := netip.MustParsePrefix(td.prefix)
		lpm.Insert(prefix, td.value)
	}

	// Pack to shared storage
	storage, err := lpm.PackToSharedStorage()
	if err != nil {
		t.Fatalf("PackToSharedStorage failed: %v", err)
	}

	// Create a temporary file
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "lpm_storage.bin")

	// Write storage to file
	if err := os.WriteFile(filePath, storage, 0644); err != nil {
		t.Fatalf("Failed to write storage to file: %v", err)
	}

	// Read storage from file
	loadedStorage, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read storage from file: %v", err)
	}

	// Verify the loaded storage is identical
	if len(loadedStorage) != len(storage) {
		t.Fatalf("Storage size mismatch: original=%d, loaded=%d", len(storage), len(loadedStorage))
	}

	// Load LPM from the file storage
	lpm2, err := NewWithSharedStorage(loadedStorage)
	if err != nil {
		t.Fatalf("NewWithSharedStorage failed: %v", err)
	}

	// Test IPv4 lookups
	ipv4Tests := []struct {
		addr     string
		expected string
	}{
		{"10.5.5.5", "private-10"},
		{"172.16.1.1", "private-172"},
		{"192.168.1.1", "subnet1"},
		{"192.168.2.1", "subnet2"},
		{"192.168.100.1", "private-192"},
		{"8.8.8.8", "public-dns-1"},
		{"1.1.1.1", "public-dns-2"},
		{"100.100.100.100", "default-v4"},
	}

	for _, tc := range ipv4Tests {
		addr := netip.MustParseAddr(tc.addr)
		value, found := lpm2.Lookup(addr)

		if !found {
			t.Errorf("IPv4 Lookup(%s): expected to find value, got not found", tc.addr)
			continue
		}
		if value != tc.expected {
			t.Errorf("IPv4 Lookup(%s): expected value=%q, got %q", tc.addr, tc.expected, value)
		}
	}

	// Test IPv6 lookups
	ipv6Tests := []struct {
		addr     string
		expected string
	}{
		{"2001:db8::1", "documentation"},
		{"2001:db8:1::1", "doc-subnet1"},
		{"2001:db8:2::1", "doc-subnet2"},
		{"fe80::1", "link-local"},
		{"2001:4860::1", "public-v6-1"},
		{"2606:4700::1", "public-v6-2"},
		{"2001:500::1", "default-v6"},
	}

	for _, tc := range ipv6Tests {
		addr := netip.MustParseAddr(tc.addr)
		value, found := lpm2.Lookup(addr)

		if !found {
			t.Errorf("IPv6 Lookup(%s): expected to find value, got not found", tc.addr)
			continue
		}
		if value != tc.expected {
			t.Errorf("IPv6 Lookup(%s): expected value=%q, got %q", tc.addr, tc.expected, value)
		}
	}

	// Test that we can still insert new data after loading from file
	lpm2.Insert(netip.MustParsePrefix("203.0.113.0/24"), "test-net-3")
	lpm2.Insert(netip.MustParsePrefix("2001:db8:3::/48"), "doc-subnet3")

	// Verify new insertions work
	addr := netip.MustParseAddr("203.0.113.1")
	value, found := lpm2.Lookup(addr)
	if !found || value != "test-net-3" {
		t.Errorf("After insert Lookup(203.0.113.1): expected 'test-net-3', got found=%v, value=%q", found, value)
	}

	addr = netip.MustParseAddr("2001:db8:3::1")
	value, found = lpm2.Lookup(addr)
	if !found || value != "doc-subnet3" {
		t.Errorf("After insert Lookup(2001:db8:3::1): expected 'doc-subnet3', got found=%v, value=%q", found, value)
	}

	// Verify stats
	stats := lpm2.Stats()
	if stats.IPv4Blocks == 0 {
		t.Error("Expected non-zero IPv4 blocks")
	}
	if stats.IPv6Blocks == 0 {
		t.Error("Expected non-zero IPv6 blocks")
	}
	if stats.TotalSize == 0 {
		t.Error("Expected non-zero total size")
	}

	t.Logf("Stats after loading from file: IPv4Blocks=%d, IPv6Blocks=%d, TotalSize=%d bytes",
		stats.IPv4Blocks, stats.IPv6Blocks, stats.TotalSize)
}
