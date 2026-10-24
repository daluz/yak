package stdlib

import (
	"cmp"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/module"
)

// The semver module reads, compares and constrains versions written in
// Semantic Versioning 2.0.0, such as "1.2.3-rc.1+build.5", optionally after a
// "v". Versions are strings, and answers keep the ones they were handed.
func init() {
	constrained := append(params("values"), optional("constraint", &ast.Null{}))
	module.Register(module.BuiltinModule{Path: "std.semver", Builtins: []*eval.Builtin{
		builtin("isValid", valueParam, semverIsValid),
		builtin("parse", valueParam, semverParse),
		builtin("compare", params("a", "b"), semverCompare),
		builtin("sort", valuesParam, semverSort),
		builtin("min", constrained, semverExtreme(-1)),
		builtin("max", constrained, semverExtreme(1)),
		builtin("bump", params("value", "part"), semverBump),
		builtin("satisfies", params("value", "constraint"), semverSatisfies),
	}})
}

type version struct {
	prefixed            bool
	major, minor, patch int64
	pre, build          []string
}

func (v *version) parts() [3]*int64 { return [3]*int64{&v.major, &v.minor, &v.patch} }

func readVersion(s string) (version, bool) {
	var v version
	s, v.prefixed = strings.CutPrefix(s, "v")
	s, build, hasBuild := strings.Cut(s, "+")
	s, pre, hasPre := strings.Cut(s, "-")
	core := strings.Split(s, ".")
	if len(core) != 3 {
		return v, false
	}
	for i, n := range v.parts() {
		var ok bool
		if *n, ok = readNumber(core[i]); !ok {
			return v, false
		}
	}
	if hasPre {
		v.pre = strings.Split(pre, ".")
		for _, id := range v.pre {
			if !isIdentifier(id) {
				return v, false
			}
			if _, ok := readNumber(id); isDigits(id) && !ok {
				return v, false
			}
		}
	}
	if hasBuild {
		v.build = strings.Split(build, ".")
		for _, id := range v.build {
			if !isIdentifier(id) {
				return v, false
			}
		}
	}
	return v, true
}

// readNumber reads a numeric part, which has no leading zeros and has to fit
// in an integer.
func readNumber(s string) (int64, bool) {
	if !isDigits(s) || len(s) > 1 && s[0] == '0' {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func isIdentifier(s string) bool {
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-') {
			return false
		}
	}
	return s != ""
}

// compareVersions orders versions by precedence, which leaves out the "v" and
// the build metadata. A prerelease comes before its release.
func compareVersions(a, b version) int {
	if c := cmp.Or(cmp.Compare(a.major, b.major), cmp.Compare(a.minor, b.minor), cmp.Compare(a.patch, b.patch)); c != 0 {
		return c
	}
	if len(a.pre) == 0 || len(b.pre) == 0 {
		return cmp.Compare(len(b.pre), len(a.pre))
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		if c := compareIdentifiers(a.pre[i], b.pre[i]); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(a.pre), len(b.pre))
}

// compareIdentifiers puts numeric identifiers, by value, before the others,
// which are in ASCII order.
func compareIdentifiers(x, y string) int {
	xn, yn := isDigits(x), isDigits(y)
	switch {
	case xn && yn:
		return cmp.Or(cmp.Compare(len(x), len(y)), strings.Compare(x, y))
	case xn:
		return -1
	case yn:
		return 1
	}
	return strings.Compare(x, y)
}

func sameRelease(a, b version) bool {
	return a.major == b.major && a.minor == b.minor && a.patch == b.patch
}

type comparator struct {
	op string
	v  version
}

// constraint is a choice of ranges, each of which a version meets when it
// meets all of its comparators.
type constraint [][]comparator

// readConstraint reads s, or answers the comparator it cannot read, which is
// empty for a range with none.
func readConstraint(s string) (constraint, string, bool) {
	var out constraint
	for _, alt := range strings.Split(s, "||") {
		fields := strings.Fields(strings.ReplaceAll(alt, ",", " "))
		if len(fields) == 0 {
			return nil, "", false
		}
		group := []comparator{}
		for i := 0; i < len(fields); i++ {
			f := fields[i]
			if strings.TrimLeft(f, "<>=!~^") == "" && i+1 < len(fields) {
				i++
				f += fields[i]
			}
			cs, ok := readComparator(f)
			if !ok {
				return nil, f, false
			}
			group = append(group, cs...)
		}
		out = append(out, group)
	}
	return out, "", true
}

// partial is a version that may leave out its minor and patch parts, or write
// them as "x", "X" or "*". The parts left out are zero.
type partial struct {
	v version
	n int // how many of the major, minor and patch parts it has
}

func readPartial(s string) (partial, bool) {
	if v, ok := readVersion(s); ok {
		return partial{v, 3}, true
	}
	var p partial
	s, p.v.prefixed = strings.CutPrefix(s, "v")
	parts := strings.Split(s, ".")
	if len(parts) > 3 {
		return p, false
	}
	for i, part := range parts {
		if part == "*" || part == "x" || part == "X" {
			for _, rest := range parts[i+1:] {
				if rest != "*" && rest != "x" && rest != "X" {
					return p, false
				}
			}
			return p, true
		}
		n, ok := readNumber(part)
		if !ok {
			return p, false
		}
		*p.v.parts()[i] = n
		p.n++
	}
	return p, true
}

// below answers the comparator for the versions below the next release that
// changes one of the first k parts of p, or none when there is no such
// release, either because k is 0 or because that part cannot grow.
func below(p partial, k int) []comparator {
	if k == 0 {
		return nil
	}
	next := version{}
	parts := next.parts()
	for i := 0; i < k; i++ {
		*parts[i] = *p.v.parts()[i]
	}
	if *parts[k-1] == math.MaxInt64 {
		return nil
	}
	*parts[k-1]++
	return []comparator{{"<", next}}
}

func readComparator(s string) ([]comparator, bool) {
	op := s[:len(s)-len(strings.TrimLeft(s, "<>=!~^"))]
	p, ok := readPartial(s[len(op):])
	if !ok {
		return nil, false
	}
	atLeast := []comparator{{">=", p.v}}
	if p.n == 0 {
		switch op {
		case "", "=", ">=", "<=", "~", "^":
			return nil, true
		}
		return nil, false
	}
	switch op {
	case "", "=":
		if p.n == 3 {
			return []comparator{{"=", p.v}}, true
		}
		return append(atLeast, below(p, p.n)...), true
	case "!=":
		return []comparator{{"!=", p.v}}, p.n == 3
	case ">", "<=":
		if p.n == 3 {
			return []comparator{{op, p.v}}, true
		}
		if op == "<=" {
			return below(p, p.n), true
		}
		up := below(p, p.n)
		if up == nil {
			return []comparator{{">", p.v}, {"<", p.v}}, true
		}
		return []comparator{{">=", up[0].v}}, true
	case ">=", "<":
		return []comparator{{op, p.v}}, true
	case "~":
		return append(atLeast, below(p, min(p.n, 2))...), true
	case "^":
		k := 3
		switch {
		case p.v.major > 0 || p.n == 1:
			k = 1
		case p.v.minor > 0 || p.n == 2:
			k = 2
		}
		return append(atLeast, below(p, k)...), true
	}
	return nil, false
}

// allows answers whether v meets the constraint. A prerelease meets a range
// only when one of its comparators names a prerelease of the same release.
func (c constraint) allows(v version) bool {
	for _, group := range c {
		named := len(v.pre) == 0
		met := true
		for _, k := range group {
			d := compareVersions(v, k.v)
			switch k.op {
			case "=":
				met = d == 0
			case "!=":
				met = d != 0
			case ">":
				met = d > 0
			case ">=":
				met = d >= 0
			case "<":
				met = d < 0
			case "<=":
				met = d <= 0
			}
			if !met {
				break
			}
			if k.op != "!=" && len(k.v.pre) > 0 && sameRelease(v, k.v) {
				named = true
			}
		}
		if met && named {
			return true
		}
	}
	return false
}

func parseVersion(c call, i int) (version, error) {
	s, err := c.str(i)
	if err != nil {
		return version{}, err
	}
	v, ok := readVersion(s)
	if !ok {
		return version{}, c.errorf(i, "needs a version such as \"1.2.3\", found %q", s)
	}
	return v, nil
}

// parseConstraint reads s, the string of argument i, as a constraint.
func parseConstraint(c call, i int, s string) (constraint, error) {
	k, bad, ok := readConstraint(s)
	if !ok {
		if bad == "" {
			return nil, c.errorf(i, "has an empty range in the constraint %q", s)
		}
		return nil, c.errorf(i, "cannot read %q in the constraint %q", bad, s)
	}
	return k, nil
}

// versions reads argument i as a sequence of versions, along with its items.
func versions(c call, i int) (*eval.Array, []version, error) {
	a, values, err := c.seq(i)
	if err != nil {
		return nil, nil, err
	}
	out := make([]version, len(values))
	for j, v := range values {
		s, ok := v.(eval.String)
		if !ok {
			return nil, nil, c.errorf(i, "needs a sequence of versions, found %s at index %d", v.TypeName(), j)
		}
		if out[j], ok = readVersion(string(s)); !ok {
			return nil, nil, c.errorf(i, "needs a sequence of versions, found %q at index %d", string(s), j)
		}
	}
	return a, out, nil
}

func semverIsValid(c call) (eval.Value, error) {
	s, err := c.str(0)
	if err != nil {
		return nil, err
	}
	_, ok := readVersion(s)
	return eval.Bool(ok), nil
}

func semverParse(c call) (eval.Value, error) {
	v, err := parseVersion(c, 0)
	if err != nil {
		return nil, err
	}
	pre := make([]*eval.Elem, len(v.pre))
	for i, id := range v.pre {
		if n, ok := readNumber(id); ok {
			pre[i] = eval.Item(eval.Int(n))
		} else {
			pre[i] = eval.Item(eval.String(id))
		}
	}
	build := make([]*eval.Elem, len(v.build))
	for i, id := range v.build {
		build[i] = eval.Item(eval.String(id))
	}
	o := eval.NewObject()
	o.Set("major", false, eval.Done(eval.Int(v.major)))
	o.Set("minor", false, eval.Done(eval.Int(v.minor)))
	o.Set("patch", false, eval.Done(eval.Int(v.patch)))
	o.Set("prerelease", false, eval.Done(eval.NewArray(pre)))
	o.Set("build", false, eval.Done(eval.NewArray(build)))
	return o, nil
}

func semverCompare(c call) (eval.Value, error) {
	a, err := parseVersion(c, 0)
	if err != nil {
		return nil, err
	}
	b, err := parseVersion(c, 1)
	if err != nil {
		return nil, err
	}
	return eval.Int(compareVersions(a, b)), nil
}

func semverSort(c call) (eval.Value, error) {
	a, vs, err := versions(c, 0)
	if err != nil {
		return nil, err
	}
	order := make([]int, len(vs))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool { return compareVersions(vs[order[i]], vs[order[j]]) < 0 })
	items := make([]*eval.Elem, len(order))
	for i, at := range order {
		items[i] = a.Items()[at]
	}
	return eval.NewArray(items), nil
}

// semverExtreme makes min or max, which answer the first of the lowest or
// highest versions, of those that meet the constraint when there is one, or
// null when none does. want is the sign of a comparison that makes a new
// version the answer.
func semverExtreme(want int) func(call) (eval.Value, error) {
	return func(c call) (eval.Value, error) {
		a, vs, err := versions(c, 0)
		if err != nil {
			return nil, err
		}
		s, constrained, err := c.optionalStr(1)
		if err != nil {
			return nil, err
		}
		var k constraint
		if constrained {
			if k, err = parseConstraint(c, 1, s); err != nil {
				return nil, err
			}
		}
		best := -1
		for i, v := range vs {
			if constrained && !k.allows(v) {
				continue
			}
			if best < 0 || compareVersions(v, vs[best])*want > 0 {
				best = i
			}
		}
		switch {
		case best >= 0:
			return a.Items()[best].Value.Value()
		case constrained:
			return eval.Null{}, nil
		}
		return nil, c.errorf(0, "needs a sequence with at least one item")
	}
}

// semverBump answers the next release that changes part. A prerelease whose
// lower parts are zero already comes before that release, so the release is
// the prerelease without its prerelease.
func semverBump(c call) (eval.Value, error) {
	v, err := parseVersion(c, 0)
	if err != nil {
		return nil, err
	}
	part, err := c.str(1)
	if err != nil {
		return nil, err
	}
	k := map[string]int{"major": 0, "minor": 1, "patch": 2}
	at, ok := k[part]
	if !ok {
		return nil, c.errorf(1, "needs \"major\", \"minor\" or \"patch\", found %q", part)
	}
	parts := v.parts()
	lowerZero := true
	for _, n := range parts[at+1:] {
		lowerZero = lowerZero && *n == 0
		*n = 0
	}
	if len(v.pre) == 0 || !lowerZero {
		if *parts[at] == math.MaxInt64 {
			return nil, c.errorf(0, "cannot bump the %s part past %d", part, int64(math.MaxInt64))
		}
		*parts[at]++
	}
	prefix := ""
	if v.prefixed {
		prefix = "v"
	}
	return eval.String(fmt.Sprintf("%s%d.%d.%d", prefix, v.major, v.minor, v.patch)), nil
}

func semverSatisfies(c call) (eval.Value, error) {
	v, err := parseVersion(c, 0)
	if err != nil {
		return nil, err
	}
	s, err := c.str(1)
	if err != nil {
		return nil, err
	}
	k, err := parseConstraint(c, 1, s)
	if err != nil {
		return nil, err
	}
	return eval.Bool(k.allows(v)), nil
}
