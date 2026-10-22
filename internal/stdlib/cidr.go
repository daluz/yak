package stdlib

import (
	"fmt"
	"math/big"
	"net/netip"
	"strings"

	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/module"
)

// The cidr module works out the addresses and subnets of IPv4 and IPv6
// prefixes written in CIDR notation, such as "10.0.0.0/16" and
// "2001:db8::/32". Addresses and prefixes are strings, answered in their
// canonical form.
func init() {
	module.Register(module.BuiltinModule{Path: "std.cidr", Builtins: []*eval.Builtin{
		builtin("host", params("prefix", "n"), cidrHost),
		builtin("subnet", params("prefix", "newbits", "n"), cidrSubnet),
		builtin("subnets", params("prefix", "newbits"), cidrSubnets),
		builtin("split", params("prefix", "newbits"), cidrSplit),
		builtin("network", params("prefix"), cidrNetwork),
		builtin("address", params("prefix"), cidrAddress),
		builtin("prefixLength", params("prefix"), cidrPrefixLength),
		builtin("netmask", params("prefix"), cidrNetmask),
		builtin("broadcast", params("prefix"), cidrBroadcast),
		builtin("size", params("prefix"), cidrSize),
		builtin("hosts", params("prefix"), cidrHosts),
		builtin("contains", params("prefix", "value"), cidrContains),
		builtin("overlaps", params("a", "b"), cidrOverlaps),
		builtin("version", valueParam, cidrVersion),
		builtin("isValid", valueParam, cidrIsValid),
	}})
}

// maxSplit is the most newbits split takes, so that a mistake cannot answer
// millions of subnets.
const maxSplit = 16

// parsePrefix reads argument i as a prefix, keeping any host bits it was
// written with.
func parsePrefix(c call, i int) (netip.Prefix, error) {
	s, err := c.str(i)
	if err != nil {
		return netip.Prefix{}, err
	}
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return netip.Prefix{}, c.errorf(i, "needs a prefix such as \"10.0.0.0/16\", found %q", s)
	}
	return p, nil
}

// parseAddrOrPrefix reads argument i as a prefix, or as an address, which it
// takes for the prefix holding only that address.
func parseAddrOrPrefix(c call, i int) (netip.Prefix, error) {
	s, err := c.str(i)
	if err != nil {
		return netip.Prefix{}, err
	}
	p, ok := readAddrOrPrefix(s)
	if !ok {
		return netip.Prefix{}, c.errorf(i, "needs an address or a prefix, found %q", s)
	}
	return p, nil
}

func readAddrOrPrefix(s string) (netip.Prefix, bool) {
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		return p, err == nil
	}
	a, err := netip.ParseAddr(s)
	if err != nil || a.Zone() != "" {
		return netip.Prefix{}, false
	}
	return netip.PrefixFrom(a, a.BitLen()), true
}

// family is 4 or 6. An IPv4-mapped IPv6 address is IPv6.
func family(p netip.Prefix) int {
	if p.Addr().Is4() {
		return 4
	}
	return 6
}

// sameFamily refuses to compare prefixes of different families, reporting
// argument i.
func sameFamily(c call, i int, p, q netip.Prefix) error {
	if family(p) != family(q) {
		return c.errorf(i, "cannot compare IPv%d %s with IPv%d %s", family(p), p, family(q), shown(q))
	}
	return nil
}

// shown writes a prefix of a single address as just the address.
func shown(p netip.Prefix) string {
	if p.IsSingleIP() {
		return p.Addr().String()
	}
	return p.String()
}

func hostBits(p netip.Prefix) int { return p.Addr().BitLen() - p.Bits() }

func pow2(k int) *big.Int { return new(big.Int).Lsh(big.NewInt(1), uint(k)) }

// addressAt answers the address that lies off past the start of p.
func addressAt(p netip.Prefix, off *big.Int) netip.Addr {
	base := p.Masked().Addr()
	n := new(big.Int).Add(new(big.Int).SetBytes(base.AsSlice()), off)
	a, _ := netip.AddrFromSlice(n.FillBytes(make([]byte, base.BitLen()/8)))
	return a
}

// index reads argument i as the position of one of the count things of p,
// where a negative position counts back from the end.
func index(c call, i int, count *big.Int, things string, p netip.Prefix) (*big.Int, error) {
	n, err := c.integer(i)
	if err != nil {
		return nil, err
	}
	idx := big.NewInt(int64(n))
	if n < 0 {
		idx.Add(idx, count)
	}
	if idx.Sign() < 0 || idx.Cmp(count) >= 0 {
		return nil, c.errorf(i, "index %d is out of range for the %s %s of %s", int64(n), count, things, p)
	}
	return idx, nil
}

// checkNewbits refuses n, as argument i, when a prefix n bits longer than p
// would be longer than an address.
func checkNewbits(c call, i int, p netip.Prefix, n eval.Int) error {
	if n < 0 || int64(n) > int64(hostBits(p)) {
		return c.errorf(i, "needs newbits from 0 to %d for %s, found %d", hostBits(p), p, int64(n))
	}
	return nil
}

func newbits(c call, i int, p netip.Prefix) (int, error) {
	n, err := c.integer(i)
	if err != nil {
		return 0, err
	}
	return int(n), checkNewbits(c, i, p, n)
}

// subnetAt answers the subnet of p, newbits longer, at idx.
func subnetAt(p netip.Prefix, nb int, idx *big.Int) string {
	off := new(big.Int).Lsh(idx, uint(hostBits(p)-nb))
	return netip.PrefixFrom(addressAt(p, off), p.Bits()+nb).String()
}

func cidrHost(c call) (eval.Value, error) {
	p, err := parsePrefix(c, 0)
	if err != nil {
		return nil, err
	}
	idx, err := index(c, 1, pow2(hostBits(p)), "addresses", p)
	if err != nil {
		return nil, err
	}
	return eval.String(addressAt(p, idx).String()), nil
}

func cidrSubnet(c call) (eval.Value, error) {
	p, err := parsePrefix(c, 0)
	if err != nil {
		return nil, err
	}
	nb, err := newbits(c, 1, p)
	if err != nil {
		return nil, err
	}
	idx, err := index(c, 2, pow2(nb), fmt.Sprintf("/%d subnets", p.Bits()+nb), p)
	if err != nil {
		return nil, err
	}
	return eval.String(subnetAt(p, nb, idx)), nil
}

// cidrSubnets allocates consecutive subnets of p, one for each item of
// newbits, starting each at the first address after the one before that is
// aligned to its size.
func cidrSubnets(c call) (eval.Value, error) {
	p, err := parsePrefix(c, 0)
	if err != nil {
		return nil, err
	}
	_, values, err := c.seq(1)
	if err != nil {
		return nil, err
	}
	total := pow2(hostBits(p))
	next := new(big.Int)
	items := make([]*eval.Elem, 0, len(values))
	for j, v := range values {
		n, ok := v.(eval.Int)
		if !ok {
			return nil, c.errorf(1, "needs a sequence of integers, found %s at index %d", v.TypeName(), j)
		}
		if err := checkNewbits(c, 1, p, n); err != nil {
			return nil, err
		}
		block := pow2(hostBits(p) - int(n))
		next.Add(next, block).Sub(next, big.NewInt(1)).Div(next, block).Mul(next, block)
		end := new(big.Int).Add(next, block)
		if end.Cmp(total) > 0 {
			return nil, c.errorf(1, "runs out of room in %s at index %d", p, j)
		}
		prefix := netip.PrefixFrom(addressAt(p, next), p.Bits()+int(n))
		items = append(items, eval.Item(eval.String(prefix.String())))
		next = end
	}
	return eval.NewArray(items), nil
}

func cidrSplit(c call) (eval.Value, error) {
	p, err := parsePrefix(c, 0)
	if err != nil {
		return nil, err
	}
	nb, err := newbits(c, 1, p)
	if err != nil {
		return nil, err
	}
	if nb > maxSplit {
		return nil, c.errorf(1, "answers at most %d subnets, so newbits must be %d or less, found %d", 1<<maxSplit, maxSplit, nb)
	}
	items := make([]*eval.Elem, 1<<nb)
	for j := range items {
		items[j] = eval.Item(eval.String(subnetAt(p, nb, big.NewInt(int64(j)))))
	}
	return eval.NewArray(items), nil
}

func cidrNetwork(c call) (eval.Value, error) {
	p, err := parsePrefix(c, 0)
	if err != nil {
		return nil, err
	}
	return eval.String(p.Masked().String()), nil
}

func cidrAddress(c call) (eval.Value, error) {
	p, err := parsePrefix(c, 0)
	if err != nil {
		return nil, err
	}
	return eval.String(p.Addr().String()), nil
}

func cidrPrefixLength(c call) (eval.Value, error) {
	p, err := parsePrefix(c, 0)
	if err != nil {
		return nil, err
	}
	return eval.Int(p.Bits()), nil
}

// ipv4Prefix reads argument i as a prefix that has to be IPv4.
func ipv4Prefix(c call, i int) (netip.Prefix, error) {
	p, err := parsePrefix(c, i)
	if err != nil {
		return netip.Prefix{}, err
	}
	if family(p) != 4 {
		return netip.Prefix{}, c.errorf(i, "needs an IPv4 prefix, found %s", p)
	}
	return p, nil
}

func cidrNetmask(c call) (eval.Value, error) {
	p, err := ipv4Prefix(c, 0)
	if err != nil {
		return nil, err
	}
	mask := netip.PrefixFrom(netip.IPv4Unspecified(), 0)
	ones := new(big.Int).Sub(pow2(32), pow2(hostBits(p)))
	return eval.String(addressAt(mask, ones).String()), nil
}

func cidrBroadcast(c call) (eval.Value, error) {
	p, err := ipv4Prefix(c, 0)
	if err != nil {
		return nil, err
	}
	last := new(big.Int).Sub(pow2(hostBits(p)), big.NewInt(1))
	return eval.String(addressAt(p, last).String()), nil
}

// addressCount answers how many addresses p holds, refusing a count too large
// for an integer.
func addressCount(c call, p netip.Prefix) (eval.Int, error) {
	if hostBits(p) > 62 {
		return 0, c.errorf(0, "of %s is 2^%d, too large for an integer", p, hostBits(p))
	}
	return eval.Int(1) << hostBits(p), nil
}

func cidrSize(c call) (eval.Value, error) {
	p, err := parsePrefix(c, 0)
	if err != nil {
		return nil, err
	}
	return addressCount(c, p)
}

// cidrHosts leaves out the network and broadcast addresses of an IPv4
// prefix, except for a /31, whose two addresses are both hosts on a
// point-to-point link, and a /32. IPv6 has no broadcast address.
func cidrHosts(c call) (eval.Value, error) {
	p, err := parsePrefix(c, 0)
	if err != nil {
		return nil, err
	}
	n, err := addressCount(c, p)
	if err != nil {
		return nil, err
	}
	if family(p) == 4 && hostBits(p) > 1 {
		n -= 2
	}
	return n, nil
}

func cidrContains(c call) (eval.Value, error) {
	p, err := parsePrefix(c, 0)
	if err != nil {
		return nil, err
	}
	q, err := parseAddrOrPrefix(c, 1)
	if err != nil {
		return nil, err
	}
	if err := sameFamily(c, 1, p, q); err != nil {
		return nil, err
	}
	return eval.Bool(q.Bits() >= p.Bits() && p.Masked().Contains(q.Addr())), nil
}

func cidrOverlaps(c call) (eval.Value, error) {
	p, err := parsePrefix(c, 0)
	if err != nil {
		return nil, err
	}
	q, err := parsePrefix(c, 1)
	if err != nil {
		return nil, err
	}
	if err := sameFamily(c, 1, p, q); err != nil {
		return nil, err
	}
	return eval.Bool(p.Masked().Overlaps(q.Masked())), nil
}

func cidrVersion(c call) (eval.Value, error) {
	p, err := parseAddrOrPrefix(c, 0)
	if err != nil {
		return nil, err
	}
	return eval.Int(family(p)), nil
}

func cidrIsValid(c call) (eval.Value, error) {
	s, err := c.str(0)
	if err != nil {
		return nil, err
	}
	_, ok := readAddrOrPrefix(s)
	return eval.Bool(ok), nil
}
