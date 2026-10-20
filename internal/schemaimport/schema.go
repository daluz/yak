package schemaimport

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"

	"github.com/daluz/yak/internal/token"
)

// typ is a yak type: a mapping type, a sequence type of item, or the
// alternatives of a union, and the checks of its "if".
type typ struct {
	mapping  *mapping
	item     *typ
	alts     []string
	nullable bool
	// admits holds the yak type names of the values the type can hold,
	// which decides the checks that need a guard.
	admits []string
	checks []check
}

type mapping struct {
	fields []field
	// rest is the type of the keys no field declares, or nil when the
	// mapping takes no other key.
	rest *typ
}

type field struct {
	name     string
	optional bool
	t        *typ
	def      string
}

// check is a test of the value v of one of the kinds it names.
type check struct {
	kinds []string
	expr  string
}

var (
	numbers   = []string{"integer", "float"}
	scalars   = []string{"null", "boolean", "integer", "float", "string"}
	everyKind = append(slices.Clone(scalars), "sequence", "mapping")
)

func builtin(name string) *typ {
	if name == "any" {
		return &typ{alts: []string{"any"}, admits: everyKind}
	}
	return &typ{alts: []string{name}, admits: []string{name}}
}

func literal(text string) *typ {
	return &typ{alts: []string{text}, admits: []string{"string"}}
}

// documentation holds the keywords that say nothing a value is checked
// against, and those that only say how the API server merges values.
var documentation = []string{
	"description", "title", "example", "externalDocs",
	"x-kubernetes-list-map-keys", "x-kubernetes-map-type",
}

// translated holds the keywords convert translates.
var translated = []string{
	"type", "properties", "required", "items", "additionalProperties", "default", "enum", "nullable",
	"x-kubernetes-preserve-unknown-fields", "x-kubernetes-int-or-string", "x-kubernetes-embedded-resource",
	"x-kubernetes-list-type",
	"minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf",
	"minLength", "maxLength", "pattern", "minItems", "maxItems", "uniqueItems",
	"minProperties", "maxProperties",
}

// convert translates the OpenAPI v3 schema n, which path names in warnings.
// The schema at the root of a CustomResourceDefinition always translates to
// a mapping type, and leaves out the fields that root adds.
func (im *importer) convert(n *yaml.Node, path string, root bool) *typ {
	n = deref(n)
	if n.Kind != yaml.MappingNode {
		im.warn(path, "is not a schema, so it takes any value")
		return builtin("any")
	}
	for i := 0; i < len(n.Content); i += 2 {
		key := n.Content[i].Value
		switch {
		case slices.Contains(documentation, key), slices.Contains(translated, key):
		default:
			im.warn(path, "%q is not imported", key)
		}
	}

	var t *typ
	switch name := str(get(n, "type")); {
	case isTrue(get(n, "x-kubernetes-int-or-string")):
		t = &typ{alts: []string{"integer", "string"}, admits: []string{"integer", "string"}}
	case name == "object" || root:
		t = &typ{mapping: im.object(n, path, root), admits: []string{"mapping"}}
		if t.mapping == nil {
			t.alts = []string{"mapping"}
		}
	case name == "array":
		t = builtin("sequence")
		if items := get(n, "items"); items != nil {
			item := im.convert(items, path+"[]", false)
			t.item = item
			if isTrue(get(n, "uniqueItems")) || str(get(n, "x-kubernetes-list-type")) == "set" {
				if item.mapping != nil || !subset(item.admits, scalars) {
					im.warn(path, "its items are not checked to be unique, since they are not scalars")
				} else {
					t.checks = append(t.checks, check{[]string{"sequence"}, "size(unique(v)) == size(v)"})
				}
			}
		}
		if str(get(n, "x-kubernetes-list-type")) == "map" {
			im.warn(path, "its items are not checked to be unique by their x-kubernetes-list-map-keys")
		}
	case name == "string", name == "integer", name == "boolean":
		t = builtin(name)
	case name == "number":
		t = &typ{alts: []string{"integer | float"}, admits: numbers}
	case name == "":
		t = builtin("any")
	default:
		im.warn(path, "the type %q is not imported, so it takes any value", name)
		t = builtin("any")
	}

	if enum := get(n, "enum"); enum != nil {
		var lits []string
		for _, v := range deref(enum).Content {
			lit, ok := scalar(v)
			if !ok {
				lits = nil
				im.warn(path, "its enum is not imported, since it holds a value that is not a scalar")
				break
			}
			lits = append(lits, lit)
		}
		if lits != nil {
			t.mapping, t.item, t.alts = nil, nil, lits
		}
	}
	if isTrue(get(n, "nullable")) && !slices.Contains(t.admits, "null") {
		t.nullable = true
		t.admits = append(slices.Clone(t.admits), "null")
	}
	im.addChecks(n, path, t)
	return t
}

// object translates the properties of the object schema n, answering nil
// for an object that holds whatever it is given.
func (im *importer) object(n *yaml.Node, path string, root bool) *mapping {
	props, extra := get(n, "properties"), get(n, "additionalProperties")
	preserve := isTrue(get(n, "x-kubernetes-preserve-unknown-fields"))
	embedded := isTrue(get(n, "x-kubernetes-embedded-resource"))
	if props == nil && extra == nil && preserve && !root && !embedded {
		return nil
	}
	m := &mapping{}
	required := map[string]bool{}
	for _, r := range deref(get(n, "required")).Content {
		required[r.Value] = true
	}
	props = deref(props)
	for i := 0; i < len(props.Content); i += 2 {
		name, schema := props.Content[i].Value, props.Content[i+1]
		at := path + "." + name
		if root && slices.Contains(envelope, name) {
			if name == "metadata" && len(deref(schema).Content) > 0 && !onlyKeys(schema, "type", "description") {
				im.warn(at, "its schema is not imported, since every object takes the same metadata")
			}
			continue
		}
		f := field{name: name, optional: !required[name], t: im.convert(schema, at, false)}
		if d := get(schema, "default"); d != nil {
			if def, ok := value(d); ok {
				f.def, f.optional = def, false
			} else {
				im.warn(at, "its default is not imported, since it holds a value yak cannot write")
			}
		}
		m.fields = append(m.fields, f)
	}
	if embedded {
		var meta []field
		for _, name := range envelope {
			if !slices.ContainsFunc(m.fields, func(f field) bool { return f.name == name }) {
				t := builtin("string")
				if name == "metadata" {
					t = builtin("mapping")
				}
				meta = append(meta, field{name: name, optional: true, t: t})
			}
		}
		m.fields = append(meta, m.fields...)
	}
	switch {
	case deref(extra).Kind == yaml.MappingNode:
		m.rest = im.convert(extra, path+"{}", false)
	case isTrue(extra), preserve:
		m.rest = builtin("any")
	}
	return m
}

// addChecks translates the keywords of n that limit the values of t.
func (im *importer) addChecks(n *yaml.Node, path string, t *typ) {
	bound := func(keyword, op string, kinds []string, of string) {
		v := get(n, keyword)
		if v == nil {
			return
		}
		lit, ok := scalar(v)
		if !ok || (deref(v).ShortTag() != "!!int" && deref(v).ShortTag() != "!!float") {
			im.warn(path, "%q is not imported, since it is not a number", keyword)
			return
		}
		t.checks = append(t.checks, check{kinds, of + " " + op + " " + lit})
	}
	minOp, maxOp := ">=", "<="
	if isTrue(get(n, "exclusiveMinimum")) {
		minOp = ">"
	}
	if isTrue(get(n, "exclusiveMaximum")) {
		maxOp = "<"
	}
	bound("minimum", minOp, numbers, "v")
	bound("maximum", maxOp, numbers, "v")
	if v := get(n, "multipleOf"); v != nil {
		if lit, ok := scalar(v); ok && deref(v).ShortTag() != "!!str" {
			t.checks = append(t.checks, check{numbers, "v % " + lit + " == 0"})
		} else {
			im.warn(path, "%q is not imported, since it is not a number", "multipleOf")
		}
	}
	bound("minLength", ">=", []string{"string"}, "size(v)")
	bound("maxLength", "<=", []string{"string"}, "size(v)")
	if p := get(n, "pattern"); p != nil {
		if _, err := regexp.Compile(str(p)); err != nil {
			im.warn(path, "its pattern is not imported, since it is not RE2: %v", err)
		} else {
			im.regex = true
			t.checks = append(t.checks, check{[]string{"string"}, "regex.match(v, " + raw(str(p)) + ")"})
		}
	}
	bound("minItems", ">=", []string{"sequence"}, "size(v)")
	bound("maxItems", "<=", []string{"sequence"}, "size(v)")
	bound("minProperties", ">=", []string{"mapping"}, "size(v)")
	bound("maxProperties", "<=", []string{"mapping"}, "size(v)")
}

// predicate answers the expression the checks of t make of v, each of
// them guarded to let through the values it does not apply to.
func (t *typ) predicate() string {
	var parts []string
	var guarded []bool
	for _, c := range t.checks {
		kinds := intersect(t.admits, c.kinds)
		if len(kinds) == 0 {
			continue
		}
		expr := c.expr
		if len(kinds) < len(t.admits) {
			guards := make([]string, len(kinds))
			for i, k := range kinds {
				guards[i] = "type(v) != " + quote(k)
			}
			guard := strings.Join(guards, " and ")
			if len(guards) > 1 {
				guard = "(" + guard + ")"
			}
			expr = guard + " or " + expr
		}
		parts = append(parts, expr)
		guarded = append(guarded, len(kinds) < len(t.admits))
	}
	if len(parts) > 1 {
		for i := range parts {
			if guarded[i] {
				parts[i] = "(" + parts[i] + ")"
			}
		}
	}
	return strings.Join(parts, " and ")
}

// text writes t where a type follows a field, on a line indented by
// indent.
func (t *typ) text(indent string) string {
	alts := t.alts
	switch {
	case t.mapping != nil:
		alts = []string{t.mapping.text(indent)}
	case t.item != nil:
		alts = []string{"[" + t.item.text(indent) + "]"}
	}
	if t.nullable {
		alts = append(slices.Clone(alts), "null")
	}
	s := strings.Join(alts, " | ")
	if p := t.predicate(); p != "" {
		s += " if (v) => " + p
	}
	return s
}

// block says whether t is written as a block mapping type, which takes no
// union, check or default.
func (t *typ) block() bool {
	return t.mapping != nil && len(t.mapping.fields) > 0 && !t.nullable && t.predicate() == ""
}

// text writes m in braces, a field to a line indented below indent, or on
// one line when it declares no field.
func (m *mapping) text(indent string) string {
	if len(m.fields) == 0 {
		if m.rest == nil {
			return "{}"
		}
		return "{(string): " + m.rest.text(indent) + "}"
	}
	var b strings.Builder
	b.WriteString("{\n")
	inner := indent + "  "
	for _, f := range m.fields {
		b.WriteString(inner + f.head() + " " + f.t.text(inner) + f.defaultText() + ",\n")
	}
	if m.rest != nil {
		b.WriteString(inner + "(string): " + m.rest.text(inner) + ",\n")
	}
	return b.String() + indent + "}"
}

func (m *mapping) write(b *strings.Builder, indent string) {
	for _, f := range m.fields {
		writeMember(b, indent, f.head(), f.t, f.defaultText())
	}
	if m.rest != nil {
		writeMember(b, indent, "(string):", m.rest, "")
	}
}

func writeMember(b *strings.Builder, indent, head string, t *typ, def string) {
	if def == "" && t.block() {
		b.WriteString(indent + head + "\n")
		t.mapping.write(b, indent+"  ")
		return
	}
	b.WriteString(indent + head + " " + t.text(indent) + def + "\n")
}

func (f field) head() string {
	if f.optional {
		return key(f.name) + ":?"
	}
	return key(f.name) + ":"
}

func (f field) defaultText() string {
	if f.def == "" {
		return ""
	}
	return " = " + f.def
}

// library writes the schemas im imported from the files names lists.
func (im *importer) library(names []string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "# Generated by yak schema import from %s.\n", strings.Join(names, ", "))
	if im.regex {
		b.WriteString("\nimport std.regex\n")
	}
	for _, d := range im.schemas {
		b.WriteString("\n")
		if d.typ.block() {
			b.WriteString("schema " + d.name + " =\n")
			d.typ.mapping.write(&b, "  ")
		} else {
			b.WriteString("schema " + d.name + " = " + d.typ.text("") + "\n")
		}
	}
	return []byte(b.String())
}

var plainKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// key writes a field name, quoted unless it is a plain identifier.
func key(name string) string {
	if plainKey.MatchString(name) && name != "_" && !token.IsReserved(name) && !token.IsKeyword(name) {
		return name
	}
	return quote(name)
}

// quote writes s as a quoted yak string, which interpolates nothing.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '\\' || r == '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\r':
			b.WriteString(`\r`)
		case r > 0xffff && !unicode.IsPrint(r):
			fmt.Fprintf(&b, `\U%08x`, r)
		case r != ' ' && !unicode.IsPrint(r):
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// raw writes s as a raw yak string when one can hold it, so that a pattern
// keeps its backslashes as written.
func raw(s string) string {
	if strings.ContainsFunc(s, func(r rune) bool { return r != ' ' && !unicode.IsPrint(r) }) {
		return quote(s)
	}
	switch {
	case !strings.Contains(s, `"`):
		return `r"` + s + `"`
	case !strings.Contains(s, `'`):
		return `r'` + s + `'`
	}
	return quote(s)
}

// value writes n as a yak value, answering false for one yak cannot write.
func value(n *yaml.Node) (string, bool) {
	n = deref(n)
	switch n.Kind {
	case yaml.SequenceNode:
		items := make([]string, len(n.Content))
		for i, item := range n.Content {
			s, ok := value(item)
			if !ok {
				return "", false
			}
			items[i] = s
		}
		return "[" + strings.Join(items, ", ") + "]", true
	case yaml.MappingNode:
		entries := make([]string, 0, len(n.Content)/2)
		for i := 0; i < len(n.Content); i += 2 {
			s, ok := value(n.Content[i+1])
			if !ok {
				return "", false
			}
			entries = append(entries, key(n.Content[i].Value)+": "+s)
		}
		return "{" + strings.Join(entries, ", ") + "}", true
	}
	return scalar(n)
}

// scalar writes the scalar n as a yak value.
func scalar(n *yaml.Node) (string, bool) {
	n = deref(n)
	if n.Kind != yaml.ScalarNode {
		return "", false
	}
	switch n.ShortTag() {
	case "!!str", "!!timestamp":
		return quote(n.Value), true
	case "!!int":
		i, err := strconv.ParseInt(n.Value, 0, 64)
		return strconv.FormatInt(i, 10), err == nil
	case "!!float":
		f, err := strconv.ParseFloat(n.Value, 64)
		if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
			return "", false
		}
		s := strconv.FormatFloat(f, 'f', -1, 64)
		if !strings.Contains(s, ".") {
			s += ".0"
		}
		return s, true
	case "!!bool":
		var b bool
		if err := n.Decode(&b); err != nil {
			return "", false
		}
		return strconv.FormatBool(b), true
	case "!!null":
		return "null", true
	}
	return "", false
}

func deref(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	if n == nil {
		return &yaml.Node{}
	}
	return n
}

// get answers the value of key in the mapping n, or nil.
func get(n *yaml.Node, key string) *yaml.Node {
	n = deref(n)
	if n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func str(n *yaml.Node) string {
	if n = deref(n); n.Kind != yaml.ScalarNode {
		return ""
	}
	return n.Value
}

func isTrue(n *yaml.Node) bool {
	n = deref(n)
	return n.ShortTag() == "!!bool" && n.Value == "true"
}

// onlyKeys says whether the mapping n holds no key but those given.
func onlyKeys(n *yaml.Node, keys ...string) bool {
	n = deref(n)
	for i := 0; i < len(n.Content); i += 2 {
		if !slices.Contains(keys, n.Content[i].Value) {
			return false
		}
	}
	return true
}

func intersect(a, b []string) []string {
	var out []string
	for _, s := range a {
		if slices.Contains(b, s) {
			out = append(out, s)
		}
	}
	return out
}

func subset(a, b []string) bool {
	return len(intersect(a, b)) == len(a)
}
