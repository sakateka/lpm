// These tests are ported from the map trie implementation.
package lpm

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_LPM_LookupEmpty(t *testing.T) {
	lpm := New()

	// Expect failed lookup in empty lpm.
	_, ok := lpm.Lookup(netip.MustParseAddr("192.168.9.1"))
	assert.False(t, ok)
}

func Test_LPM_LookupAfterInsert(t *testing.T) {
	cases := []struct {
		addr        string
		expectedOk  bool
		expectedVal string
	}{
		{"192.168.9.1", true, "dc0"},
		{"127.0.0.1", false, ""},
	}

	lpm := New()
	lpm.Insert(netip.MustParsePrefix("192.168.0.0/16"), "dc0")

	for _, c := range cases {
		val, ok := lpm.Lookup(netip.MustParseAddr(c.addr))
		require.Equal(t, c.expectedOk, ok)
		assert.Equal(t, c.expectedVal, val)
	}
}

func Test_LPM_LookupAfterInsertUpdate(t *testing.T) {
	cases := []struct {
		addr        string
		expectedOk  bool
		expectedVal string
	}{
		{"192.168.9.1", true, "dc1"},
		{"127.0.0.1", false, ""},
	}

	lpm := New()
	lpm.Insert(netip.MustParsePrefix("192.168.0.0/16"), "dc0")
	// This should update the value to dc1.
	lpm.Insert(netip.MustParsePrefix("192.168.0.0/16"), "dc1")

	for _, c := range cases {
		val, ok := lpm.Lookup(netip.MustParseAddr(c.addr))
		require.Equal(t, c.expectedOk, ok)
		assert.Equal(t, c.expectedVal, val)
	}
}

func Test_LPM_LookupAfterInsertNestedPrefixes(t *testing.T) {
	cases := []struct {
		addr        string
		expectedOk  bool
		expectedVal string
	}{
		{"192.168.1.1", true, "dc4"},
		{"192.168.1.2", true, "dc3"},
		{"192.168.2.2", true, "dc2"},
		{"192.200.1.1", true, "dc1"},
		{"127.0.0.1", true, "dc0"},
	}

	lpm := New()
	lpm.Insert(netip.MustParsePrefix("0.0.0.0/0"), "dc0")
	lpm.Insert(netip.MustParsePrefix("192.0.0.0/8"), "dc1")
	lpm.Insert(netip.MustParsePrefix("192.168.0.0/16"), "dc2")
	lpm.Insert(netip.MustParsePrefix("192.168.1.0/24"), "dc3")
	lpm.Insert(netip.MustParsePrefix("192.168.1.1/32"), "dc4")

	for _, c := range cases {
		val, ok := lpm.Lookup(netip.MustParseAddr(c.addr))
		require.Equal(t, c.expectedOk, ok)
		assert.Equal(t, c.expectedVal, val)
	}
}

func Test_LPM_Lookup6(t *testing.T) {
	cases := []struct {
		prefix      string
		expectedOk  bool
		expectedVal string
	}{
		{"fd25:cf19:6b13:cafe:babe:be57:f00d:04a5/128", false, ""},
		{"fd25:cf19:6b13:cafe:babe:be57:f00d:400/120", false, ""},
		{"fd25:cf19:6b13:cafe:babe:be57:f00d::/112", true, "dc2"},
		{"fd25:cf19:6b13:cafe::/64", true, "dc2"},
		{"fd25:8888:6b13:cafe::/64", true, "dc2"},
		{"fd25::/16", true, "dc2"},
	}

	addr := netip.MustParseAddr("fd25:cf19:6b13:cafe:babe:be57:f00d:0001")

	lpm := New()

	for idx, c := range cases {
		prefix := netip.MustParsePrefix(c.prefix).Masked()

		value := c.expectedVal
		if c.expectedVal == "" {
			value = "dc" + string(rune('0'+idx))
		}
		lpm.Insert(prefix, value)

		value, ok := lpm.Lookup(addr)
		require.Equal(t, c.expectedOk, ok,
			"lookup expected match==%t, but ok=%t, prefix=%s", c.expectedOk, ok, prefix)
		if c.expectedOk {
			require.Equal(t, c.expectedVal, value,
				"lookup expected value==%s, but value=%s, prefix=%s", c.expectedVal, value, prefix)
		}
	}
}

func Test_LPM_Lookup6TopDownInsert(t *testing.T) {
	cases := []struct {
		prefix      string
		expectedOk  bool
		expectedVal string
	}{
		{"fd25::/16", true, "dc0"},
		{"fd25:8888:6b13:cafe::/64", true, "dc0"},
		{"fd25:cf19:6b13:cafe::/64", true, "dc2"},
		{"fd25:cf19:6b13:cafe:babe:be57:f00d::/112", true, "dc3"},
		{"fd25:cf19:6b13:cafe:babe:be57:f00d:400/120", true, "dc3"},
		{"fd25:cf19:6b13:cafe:babe:be57:f00d:04a5/128", true, "dc3"},
		{"fd25:cf19:6b13:cafe:babe:be57:f00d:0001/128", true, "dc6"},
	}

	addr := netip.MustParseAddr("fd25:cf19:6b13:cafe:babe:be57:f00d:0001")

	lpm := New()

	for idx, c := range cases {
		prefix := netip.MustParsePrefix(c.prefix).Masked()

		lpm.Insert(prefix, "dc"+string(rune('0'+idx)))

		value, ok := lpm.Lookup(addr)
		require.Equal(t, c.expectedOk, ok,
			"lookup expected match==%t, but ok=%t, prefix=%s", c.expectedOk, ok, prefix)
		require.Equal(t, c.expectedVal, value,
			"lookup expected value==%s, but value=%s, prefix=%s", c.expectedVal, value, prefix)
	}
}

func Test_LPM_LookupTraverse(t *testing.T) {
	lpm := New()

	// Helper function to check if lookup returns expected value
	checkLookup := func(addr netip.Addr, expectedVal string, expectedOk bool) {
		val, ok := lpm.Lookup(addr)
		assert.Equal(t, expectedOk, ok)
		if expectedOk {
			assert.Equal(t, expectedVal, val)
		}
	}

	addr := netip.MustParseAddr("192.168.9.32")

	// Empty lookup
	checkLookup(addr, "", false)

	// Insert 192.168.9.32/32
	lpm.Insert(netip.MustParsePrefix("192.168.9.32/32"), "dc0")
	checkLookup(addr, "dc0", true)

	// Insert 192.168.9.0/24
	lpm.Insert(netip.MustParsePrefix("192.168.9.0/24"), "dc1")
	checkLookup(addr, "dc0", true) // More specific /32 should still match

	// Note, that 192.168.9.0/27 does not contain 192.168.9.32
	lpm.Insert(netip.MustParsePrefix("192.168.9.0/27"), "dc2")
	checkLookup(addr, "dc0", true) // /32 should still match

	// ... but 192.168.9.0/26 does contain 192.168.9.32
	lpm.Insert(netip.MustParsePrefix("192.168.9.0/26"), "dc3")
	checkLookup(addr, "dc0", true) // Most specific /32 should still match

	// Does not affect - different subnet
	lpm.Insert(netip.MustParsePrefix("192.168.10.0/24"), "dc4")
	checkLookup(addr, "dc0", true)

	// Insert broader prefix
	lpm.Insert(netip.MustParsePrefix("192.168.0.0/16"), "dc5")
	checkLookup(addr, "dc0", true) // Most specific /32 should still match

	// 192.168.0.0 in hex - IPv6 address, should not affect IPv4 lookup
	lpm.Insert(netip.MustParsePrefix("a8c0::/16"), "dc6")
	checkLookup(addr, "dc0", true)

	lpm.Insert(netip.MustParsePrefix("a8c0::/112"), "dc7")
	checkLookup(addr, "dc0", true)

	// v6 mapped ::ffff:168.192.1.9 - should not affect IPv4 lookup
	lpm.Insert(netip.MustParsePrefix("::ffff:a8c0:109/16"), "dc8")
	checkLookup(addr, "dc0", true)

	// v6 mapped ::ffff:168.192.1.9
	lpm.Insert(netip.MustParsePrefix("::ffff:a8c0:109/112"), "dc9")
	checkLookup(addr, "dc0", true)

	lpm.Insert(netip.MustParsePrefix("192.0.0.0/8"), "dc10")
	checkLookup(addr, "dc0", true) // Most specific /32 should still match

	lpm.Insert(netip.MustParsePrefix("193.168.9.1/8"), "dc11")
	checkLookup(addr, "dc0", true)

	// NOTE: this is very important case! No intermix between IPv4 and IPv6
	lpm.Insert(netip.MustParsePrefix("::/0"), "dc12")
	checkLookup(addr, "dc0", true)

	// ... but IPv4 UNSPECIFIED is okay - it should match as a less specific prefix
	lpm.Insert(netip.MustParsePrefix("0.0.0.0/0"), "dc13")
	checkLookup(addr, "dc0", true) // Most specific /32 should still match
}
