package lpm

import (
	"encoding/json"
	"fmt"
	"net/netip"
	// "os"
)

type DCNet struct {
	CIDR string `json:"cidr"`
	DC   string `json:"dc"`
}

func unwrap[V any](value V, err error) V {
	if err != nil {
		panic(err)
	}
	return value
}

func ExampleLPM_Insert() {
	data := []byte(`
	[
	  { "cidr": "192.168.1.0/18", "dc": "dc1" },
	  { "cidr": "10.45.19.0/24", "dc": "dc2" },
	  { "cidr": "10.4.1.0/24", "dc": "dc3" },
	  { "cidr": "0.0.0.0/0", "dc": "default-v4" },
	  { "cidr": "192.168.1.128/25", "dc": "dc1-edge" },
	  { "cidr": "8.8.8.0/24", "dc": "public-dns-1" },
	  { "cidr": "1.1.1.0/24", "dc": "public-dns-2" },
	  { "cidr": "203.0.113.0/24", "dc": "test-net-3" },
	  { "cidr": "::/0", "dc": "default-v6" },
	  { "cidr": "2001:db8::/32", "dc": "documentation" },
	  { "cidr": "2001:db8:1::/48", "dc": "doc-subnet1" },
	  { "cidr": "2001:db8:2::/48", "dc": "doc-subnet2" },
	  { "cidr": "fe80::/10", "dc": "link-local" },
	  { "cidr": "2001:4860::/32", "dc": "public-v6-1" },
	  { "cidr": "2606:4700::/32", "dc": "public-v6-2" }
	]
`)

	var resp []DCNet
	unwrap(0, json.Unmarshal(data, &resp))

	lpm := New()

	for _, dcnet := range resp {
		net := netip.MustParsePrefix(dcnet.CIDR)

		lpm.Insert(net, dcnet.DC)
	}

	stats := lpm.Stats()
	fmt.Printf("Size of v4 storage: %d\n", stats.IPv4StorageSize)
	fmt.Printf("Size of v6 storage: %d\n", stats.IPv6StorageSize)
	fmt.Printf("Number of v4 blocks: %d\n", stats.IPv4Blocks)
	fmt.Printf("Number of v6 blocks: %d\n", stats.IPv6Blocks)
	fmt.Printf("Size of the lpm: %d\n", stats.TotalSize)
	fmt.Printf("Values storage size: %d\n", stats.ValuesStorage)

	// storage := unwrap(lpm.PackToSharedStorage())
	// os.WriteFile("dcnets.lpm", storage, 0o755)

	// Output:
	// Size of v4 storage: 13440
	// Size of v6 storage: 11376
	// Number of v4 blocks: 13
	// Number of v6 blocks: 11
	// Size of the lpm: 25822
	// Values storage size: 1006
}
