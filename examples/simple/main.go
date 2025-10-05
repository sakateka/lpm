package main

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"os"

	"github.com/sakateka/lpm"
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

func main() {
	data := unwrap(os.ReadFile("data.json"))

	var resp []DCNet
	unwrap(0, json.Unmarshal(data, &resp))

	lpm := lpm.New()

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

	storage := unwrap(lpm.PackToSharedStorage())
	os.WriteFile("dcnets.lpm", storage, 0o755)
}
