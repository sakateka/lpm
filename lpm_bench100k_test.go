package lpm

import (
	"fmt"
	"math/rand"
	"net/netip"
	"runtime"
	"testing"
)

// BenchmarkLPMInsert100k benchmarks insertion of 100k prefixes
func BenchmarkLPMInsert100k(b *testing.B) {
	benchmarks := []struct {
		name     string
		prefixes []netip.Prefix
		values   []string
	}{
		{
			name: "ipv4_100k_prefixes",
			prefixes: func() []netip.Prefix {
				rng := rand.New(rand.NewSource(42))
				prefixes := make([]netip.Prefix, 1000_000)
				for i := range prefixes {
					// Generate diverse prefixes with random masks (8-32)
					addr := netip.AddrFrom4([4]byte{
						byte((i >> 16) & 0xff),
						byte((i >> 8) & 0xff),
						byte(i & 0xff),
						byte(rng.Intn(256)),
					})
					prefixLen := 8 + rng.Intn(25)
					prefixes[i] = netip.PrefixFrom(addr, prefixLen).Masked()
				}
				return prefixes
			}(),
			values: func() []string {
				values := make([]string, 1000_000)
				for i := range values {
					values[i] = fmt.Sprintf("DC%d", i)
				}
				return values
			}(),
		},
		{
			name: "ipv6_100k_prefixes",
			prefixes: func() []netip.Prefix {
				rng := rand.New(rand.NewSource(42))
				prefixes := make([]netip.Prefix, 1000_000)
				for i := range prefixes {
					// Generate diverse prefixes with random masks (32-128)
					addr := netip.AddrFrom16([16]byte{
						0x20, 0x01, 0x0d, 0xb8,
						byte((i >> 24) & 0xff), byte((i >> 16) & 0xff),
						byte((i >> 8) & 0xff), byte(i & 0xff),
						byte(rng.Intn(256)), byte(rng.Intn(256)),
						byte(rng.Intn(256)), byte(rng.Intn(256)),
						byte(rng.Intn(256)), byte(rng.Intn(256)),
						byte(rng.Intn(256)), byte(rng.Intn(256)),
					})
					prefixLen := 32 + rng.Intn(97)
					prefixes[i] = netip.PrefixFrom(addr, prefixLen).Masked()
				}
				return prefixes
			}(),
			values: func() []string {
				values := make([]string, 1000_000)
				for i := range values {
					values[i] = fmt.Sprintf("DC%d", i)
				}
				return values
			}(),
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()

			lpm := New()
			idx := 0

			for b.Loop() {
				lpm.Insert(bm.prefixes[idx], bm.values[idx])
				idx = (idx + 1) % 1000_000
			}
		})
	}
}

// BenchmarkLPMLookup100k benchmarks lookups in an LPM with 100k prefixes
func BenchmarkLPMLookup100k(b *testing.B) {
	benchmarks := []struct {
		name     string
		prefixes []netip.Prefix
		values   []string
		addrs    []netip.Addr
	}{
		{
			name: "ipv4_100k_prefixes",
			prefixes: func() []netip.Prefix {
				rng := rand.New(rand.NewSource(42))
				prefixes := make([]netip.Prefix, 1000_000)
				for i := range prefixes {
					addr := netip.AddrFrom4([4]byte{
						byte((i >> 16) & 0xff),
						byte((i >> 8) & 0xff),
						byte(i & 0xff),
						byte(rng.Intn(256)),
					})
					prefixLen := 8 + rng.Intn(25)
					prefixes[i] = netip.PrefixFrom(addr, prefixLen).Masked()
				}
				return prefixes
			}(),
			values: func() []string {
				values := make([]string, 1000_000)
				for i := range values {
					values[i] = fmt.Sprintf("DC%d", i)
				}
				return values
			}(),
			addrs: func() []netip.Addr {
				rng := rand.New(rand.NewSource(43))
				addrs := make([]netip.Addr, 1000)
				for i := range addrs {
					addrs[i] = netip.AddrFrom4([4]byte{
						byte(rng.Intn(256)),
						byte(rng.Intn(256)),
						byte(rng.Intn(256)),
						byte(rng.Intn(256)),
					})
				}
				return addrs
			}(),
		},
		{
			name: "ipv6_100k_prefixes",
			prefixes: func() []netip.Prefix {
				rng := rand.New(rand.NewSource(42))
				prefixes := make([]netip.Prefix, 1000_000)
				for i := range prefixes {
					addr := netip.AddrFrom16([16]byte{
						0x20, 0x01, 0x0d, 0xb8,
						byte((i >> 24) & 0xff), byte((i >> 16) & 0xff),
						byte((i >> 8) & 0xff), byte(i & 0xff),
						byte(rng.Intn(256)), byte(rng.Intn(256)),
						byte(rng.Intn(256)), byte(rng.Intn(256)),
						byte(rng.Intn(256)), byte(rng.Intn(256)),
						byte(rng.Intn(256)), byte(rng.Intn(256)),
					})
					prefixLen := 32 + rng.Intn(97)
					prefixes[i] = netip.PrefixFrom(addr, prefixLen).Masked()
				}
				return prefixes
			}(),
			values: func() []string {
				values := make([]string, 1000_000)
				for i := range values {
					values[i] = fmt.Sprintf("DC%d", i)
				}
				return values
			}(),
			addrs: func() []netip.Addr {
				rng := rand.New(rand.NewSource(43))
				addrs := make([]netip.Addr, 1000)
				for i := range addrs {
					addrs[i] = netip.AddrFrom16([16]byte{
						0x20, 0x01, 0x0d, 0xb8,
						byte(rng.Intn(256)), byte(rng.Intn(256)),
						byte(rng.Intn(256)), byte(rng.Intn(256)),
						byte(rng.Intn(256)), byte(rng.Intn(256)),
						byte(rng.Intn(256)), byte(rng.Intn(256)),
						byte(rng.Intn(256)), byte(rng.Intn(256)),
						byte(rng.Intn(256)), byte(rng.Intn(256)),
					})
				}
				return addrs
			}(),
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			// Measure memory before insertion
			runtime.GC()
			var memBefore runtime.MemStats
			runtime.ReadMemStats(&memBefore)

			// Setup: Insert 100k prefixes
			lpm := New()

			for i := range 1000_000 {
				lpm.Insert(bm.prefixes[i], bm.values[i])
			}

			// Measure memory after insertion
			runtime.GC()
			var memAfter runtime.MemStats
			runtime.ReadMemStats(&memAfter)

			allocDiff := memAfter.Alloc - memBefore.Alloc
			totalAllocDiff := memAfter.TotalAlloc - memBefore.TotalAlloc

			b.Logf("Memory usage after 100k inserts: Alloc=%d bytes (%.2f MB), TotalAlloc=%d bytes (%.2f MB)",
				allocDiff, float64(allocDiff)/(1024*1024),
				totalAllocDiff, float64(totalAllocDiff)/(1024*1024))
			stats := lpm.Stats()
			b.Logf("lpm.v4StorageSize: %d, lpm.v6StorageSize: %d", stats.IPv4StorageSize, stats.IPv6StorageSize)
			b.Logf("lpm.v4Blocks: %d, lpm.v6Blocks: %d, total size: %d", stats.IPv4Blocks, stats.IPv6Blocks, stats.TotalSize)

			b.ResetTimer()
			b.ReportAllocs()

			idx := 0
			foundCount := 0
			for b.Loop() {
				val, ok := lpm.Lookup(bm.addrs[idx])
				if ok && val != "" {
					foundCount++
				}
				idx = (idx + 1) % len(bm.addrs)
			}

			if foundCount == 0 {
				b.Fatalf("No successful lookups in %d iterations", b.N)
			}
		})
	}
}
