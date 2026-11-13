package common

import (
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"
)

// PrivateNetworks permits explicitly configured private destinations in addition
// to public IPs. Callers opt in per connection path.
type PrivateNetworks struct {
	prefixes []netip.Prefix
}

var privateNetworkRanges = [...]netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("fc00::/7"),
}

// These metadata addresses are inside private ranges. Link-local metadata is
// already outside every configurable range.
var privateMetadataIPs = [...]netip.Addr{
	netip.MustParseAddr("100.100.100.200"),
	netip.MustParseAddr("fd00:ec2::254"),
	netip.MustParseAddr("fd20:ce::254"),
}

// ParsePrivateNetworks accepts individual IPs and CIDRs contained in private,
// shared-address, or IPv6 unique-local space.
func ParsePrivateNetworks(entries []string) (PrivateNetworks, error) {
	networks := PrivateNetworks{prefixes: make([]netip.Prefix, 0, len(entries))}
	for _, entry := range entries {
		prefix, err := parsePrivateNetwork(strings.TrimSpace(entry))
		if err != nil {
			return PrivateNetworks{}, err
		}
		if !slices.ContainsFunc(privateNetworkRanges[:], func(private netip.Prefix) bool {
			return prefix.Bits() >= private.Bits() && private.Contains(prefix.Addr())
		}) {
			return PrivateNetworks{}, fmt.Errorf("%q is outside the supported private address ranges", entry)
		}
		networks.prefixes = append(networks.prefixes, prefix)
	}
	return networks, nil
}

func parsePrivateNetwork(entry string) (netip.Prefix, error) {
	if addr, err := netip.ParseAddr(entry); err == nil && addr.Zone() == "" {
		addr = addr.Unmap()
		return netip.PrefixFrom(addr, addr.BitLen()), nil
	}
	prefix, err := netip.ParsePrefix(entry)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("%q must be an IP address or CIDR", entry)
	}
	if addr := prefix.Addr(); addr.Is4In6() && prefix.Bits() >= 96 {
		prefix = netip.PrefixFrom(addr.Unmap(), prefix.Bits()-96)
	}
	return prefix.Masked(), nil
}

func (n PrivateNetworks) permits(ip net.IP) bool {
	ip = effectiveIP(ip)
	if isGlobalIP(ip) {
		return true
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok || slices.Contains(privateMetadataIPs[:], addr) {
		return false
	}
	return slices.ContainsFunc(n.prefixes, func(prefix netip.Prefix) bool {
		return prefix.Contains(addr)
	})
}

// AssertURLSafe validates a URL against the public policy and these networks,
// then returns the first validated IP for a pinned connection.
func (n PrivateNetworks) AssertURLSafe(rawURL string, lookup func(string) ([]string, error)) (string, string, error) {
	return assertURLSafe(rawURL, lookup, false, n)
}

// AssertHostSafe applies the same address policy to a bare TCP host.
func (n PrivateNetworks) AssertHostSafe(host string) (string, error) {
	return assertHostSafe(host, false, n)
}
