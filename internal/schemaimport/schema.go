package schemaimport

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"

	"github.com/daluz/yak/internal/token"
)

// typ is a yak type: a mapping type, a sequence type of item, the
// alternatives of a union, written as they are or as types, and the checks
// of its "if".
type typ struct {
	mapping  *mapping
	item     *typ
	alts     []string
	union    []*typ
	nullable bool
	// admits holds the yak type names of the values the type can hold,
	// which decides the checks that need a guard.
	admits []string
	checks []check
}

type mapping struct {
	// merges names the schemas whose members the mapping brings in first.
	merges []string
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
	// implied says the field is required but has no schema of its own.
	implied bool
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

// annotations holds the keywords of JSON Schema and OpenAPI that say
// nothing a value is checked against, and composing those convert
// translates in them but not in a CustomResourceDefinition.
var (
	annotations = []string{
		"examples", "$comment", "$id", "$schema", "$defs", "definitions",
		"readOnly", "writeOnly", "deprecated", "xml",
	}
	composing = []string{"$ref", "allOf", "anyOf", "oneOf", "const"}
)

// convert translates the OpenAPI v3 or JSON Schema schema n, which path
// names in warnings. The schema at the root of a CustomResourceDefinition
// always translates to a mapping type, and leaves out the fields that root
// adds.
func (im *importer) convert(n *yaml.Node, path string, root bool) *typ {
	n = deref(n)
	if im.open && n.ShortTag() == "!!bool" {
		if n.Value != "true" {
			im.warn(path, "takes no value, which is not imported, so it takes any value")
		}
		return builtin("any")
	}
	if n.Kind != yaml.MappingNode {
		im.warn(path, "is not a schema, so it takes any value")
		return builtin("any")
	}
	if im.open && get(n, "$ref") != nil {
		return im.ref(n, path)
	}
	for i := 0; i < len(n.Content); i += 2 {
		key := n.Content[i].Value
		switch {
		case slices.Contains(documentation, key), slices.Contains(translated, key):
		case im.open && (slices.Contains(annotations, key) || slices.Contains(composing, key)):
		default:
			im.warn(path, "%q is not imported", key)
		}
	}

	var t *typ
	own := get(n, "type") != nil || describesObject(n)
	alts := ""
	for _, key := range []string{"anyOf", "oneOf"} {
		if !im.open || get(n, key) == nil {
			continue
		}
		switch {
		case own || alts != "":
			im.warn(path, "%q beside a type of its own is not imported", key)
		case get(n, "allOf") != nil:
			im.warn(path, "%q beside allOf is not imported", key)
		default:
			alts = key
		}
	}
	switch name := str(get(n, "type")); {
	case im.open && get(n, "allOf") != nil && (name == "" || name == "object"):
		t = im.allOf(n, path)
	case alts != "":
		t = im.anyOf(n, alts, path)
	default:
		if im.open && get(n, "allOf") != nil {
			im.warn(path, "%q beside a type that is not an object is not imported", "allOf")
		}
		t = im.typed(n, path, root)
	}

	enum, keyword := deref(get(n, "enum")).Content, "enum"
	if c := get(n, "const"); im.open && c != nil {
		enum, keyword = []*yaml.Node{c}, "const"
	}
	if enum != nil {
		var lits []string
		for _, v := range enum {
			lit, ok := scalar(v)
			if !ok {
				lits = nil
				im.warn(path, "its %s is not imported, since it holds a value that is not a scalar", keyword)
				break
			}
			lits = append(lits, lit)
		}
		if lits != nil {
			t.mapping, t.item, t.union, t.alts = nil, nil, nil, lits
		}
	}
	if isTrue(get(n, "nullable")) && !slices.Contains(t.admits, "null") {
		t.nullable = true
		t.admits = append(slices.Clone(t.admits), "null")
	}
	im.addChecks(n, path, t)
	return t
}

// ref translates the schema n, which is a $ref, to the schema it names.
func (im *importer) ref(n *yaml.Node, path string) *typ {
	for i := 0; i < len(n.Content); i += 2 {
		key := n.Content[i].Value
		if key != "$ref" && key != "default" && !slices.Contains(documentation, key) && !slices.Contains(annotations, key) {
			im.warn(path, "%q beside a $ref is not imported", key)
		}
	}
	name, ok := im.refName(n)
	if !ok {
		im.warn(path, "its $ref %q is not imported, so it takes any value", str(get(n, "$ref")))
		return builtin("any")
	}
	return &typ{alts: []string{name}, admits: everyKind[1:]}
}

// refName answers the schema the $ref of n names, when it names one this
// document declares.
func (im *importer) refName(n *yaml.Node) (string, bool) {
	r := str(get(n, "$ref"))
	if name, ok := im.refs[r]; ok {
		return name, true
	}
	if u, err := url.PathUnescape(r); err == nil {
		name, ok := im.refs[u]
		return name, ok
	}
	return "", false
}

// isObject says whether the schema n, or the one its $ref names, describes
// an object.
func (im *importer) isObject(n *yaml.Node) bool {
	if name, ok := im.refName(n); ok {
		n = im.defs[name]
	}
	switch str(get(n, "type")) {
	case "object":
		return true
	case "":
		return describesObject(n) || get(n, "allOf") != nil
	}
	return false
}

// allOf translates the parts of the allOf of n, and the object n describes
// beside them, into one mapping type: a part that is a $ref is merged, and
// the fields of the others are brought in. Unless every part is an object
// only a lone part is imported, with the keywords of n beside it.
func (im *importer) allOf(n *yaml.Node, path string) *typ {
	parts := deref(get(n, "allOf")).Content
	own := get(n, "type") != nil || describesObject(n)
	if len(parts) == 1 && !own {
		return im.convert(parts[0], path, false)
	}
	for i, p := range parts {
		if !im.isObject(p) {
			im.warn(path, "its allOf is not imported, since its part %d is not an object", i+1)
			return im.typed(n, path, false)
		}
	}
	m := &mapping{}
	for _, p := range parts {
		if name, ok := im.refName(p); ok {
			m.merges = append(m.merges, name)
			continue
		}
		if pm := im.convert(p, path, false).mapping; pm != nil {
			m.merges = append(m.merges, pm.merges...)
			for _, f := range pm.fields {
				m.add(f)
			}
			if pm.rest != nil {
				m.rest = pm.rest
			}
		}
	}
	if own {
		if om := im.object(n, path, false); om != nil {
			for _, f := range om.fields {
				m.add(f)
			}
			m.rest = om.rest
			if len(m.merges) > 0 && deref(get(n, "additionalProperties")).ShortTag() == "!!bool" && !isTrue(get(n, "additionalProperties")) {
				im.warn(path, "its additionalProperties is not imported, since the keys of the schemas it merges are")
			}
		} else {
			m.rest = builtin("any")
		}
	}
	if len(m.merges) > 0 {
		fields := m.fields[:0]
		for _, f := range m.fields {
			if f.implied {
				if f.t = im.inherited(m.merges, f.name, path); f.t == nil {
					im.warn(path+"."+f.name, "it is required, which is not imported, since no schema it merges declares it")
					continue
				}
			}
			fields = append(fields, f)
		}
		m.fields = fields
	}
	return &typ{mapping: m, admits: []string{"mapping"}}
}

// add declares f in m, where a field of the same name the allOf of a
// schema declared already takes the type of the later one, and is required
// when either is.
func (m *mapping) add(f field) {
	i := slices.IndexFunc(m.fields, func(g field) bool { return g.name == f.name })
	if i < 0 {
		m.fields = append(m.fields, f)
		return
	}
	old := &m.fields[i]
	switch {
	case f.implied:
		old.optional = false
	case old.implied || !old.optional:
		f.optional = false
		*old = f
	default:
		*old = f
	}
}

// inherited answers the type of the property name that one of the schemas
// merges declares, or nil, warning about nothing translating it has warned
// about already.
func (im *importer) inherited(merges []string, name, path string) *typ {
	for _, merge := range slices.Backward(merges) {
		if p := get(get(im.defs[merge], "properties"), name); p != nil {
			warnings, regex := len(im.warnings), im.regex
			t := im.convert(p, path+"."+name, false)
			im.warnings, im.regex = im.warnings[:warnings], regex || im.regex
			return t
		}
	}
	return nil
}

// anyOf translates the alternatives the keyword of n lists into a union.
func (im *importer) anyOf(n *yaml.Node, keyword, path string) *typ {
	if keyword == "oneOf" {
		im.warn(path, "its oneOf is imported as an anyOf, which lets a value match more than one alternative")
	}
	var parts []*typ
	for _, p := range deref(get(n, keyword)).Content {
		parts = append(parts, im.convert(p, path, false))
	}
	return im.union(parts, path)
}

// union answers the union of parts. A check of one of them is checked on
// the union when no other part takes a value of the kinds it checks.
func (im *importer) union(parts []*typ, path string) *typ {
	if len(parts) == 1 {
		return parts[0]
	}
	t := &typ{union: parts}
	for _, p := range parts {
		for _, k := range p.admits {
			if !slices.Contains(t.admits, k) {
				t.admits = append(t.admits, k)
			}
		}
		if p.nullable {
			p.nullable, t.nullable = false, true
		}
	}
	for i, p := range parts {
		for _, c := range p.checks {
			kinds := intersect(p.admits, c.kinds)
			shared := slices.ContainsFunc(parts, func(q *typ) bool {
				return q != p && len(intersect(q.admits, kinds)) > 0
			})
			if shared {
				im.warn(path, "a check of its alternative %d is not imported, since another alternative takes a %s as well", i+1, kinds[0])
				continue
			}
			t.checks = append(t.checks, check{kinds, c.expr})
		}
		p.checks = nil
	}
	return t
}

// typed translates the type of n, a union for a list of types.
func (im *importer) typed(n *yaml.Node, path string, root bool) *typ {
	if names := deref(get(n, "type")); names.Kind == yaml.SequenceNode {
		var parts []*typ
		for _, name := range names.Content {
			parts = append(parts, im.ofType(n, str(name), path, false))
		}
		return im.union(parts, path)
	}
	name := str(get(n, "type"))
	if name == "" && im.open && describesObject(n) {
		name = "object"
	}
	return im.ofType(n, name, path, root)
}

// ofType translates n as a schema of the type name.
func (im *importer) ofType(n *yaml.Node, name, path string, root bool) *typ {
	var t *typ
	switch {
	case isTrue(get(n, "x-kubernetes-int-or-string")):
		t = &typ{alts: []string{"integer", "string"}, admits: []string{"integer", "string"}}
	case name == "object" || root:
		t = &typ{mapping: im.object(n, path, root), admits: []string{"mapping"}}
		if t.mapping == nil {
			t.alts = []string{"mapping"}
		}
	case name == "array":
		t = builtin("sequence")
		if items := get(n, "items"); deref(items).Kind == yaml.SequenceNode {
			im.warn(path, "its items are not imported, since they are a list of schemas")
		} else if items != nil {
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
	case name == "string", name == "integer", name == "boolean", name == "null" && im.open:
		t = builtin(name)
	case name == "number":
		t = &typ{alts: []string{"integer | float"}, admits: numbers}
	case name == "":
		t = builtin("any")
	default:
		im.warn(path, "the type %q is not imported, so it takes any value", name)
		t = builtin("any")
	}
	return t
}

// describesObject reports whether n declares the properties, the
// additionalProperties or the required fields of an object.
func describesObject(n *yaml.Node) bool {
	return get(n, "properties") != nil || get(n, "additionalProperties") != nil || get(n, "required") != nil
}

// object translates the properties of the object schema n, answering nil
// for an object that holds whatever it is given.
func (im *importer) object(n *yaml.Node, path string, root bool) *mapping {
	props, extra := get(n, "properties"), get(n, "additionalProperties")
	preserve := isTrue(get(n, "x-kubernetes-preserve-unknown-fields"))
	embedded := isTrue(get(n, "x-kubernetes-embedded-resource"))
	open := preserve || (im.open && (extra == nil || isTrue(extra)))
	if props == nil && get(n, "required") == nil && (extra == nil || im.open) && open && !root && !embedded {
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
	for _, r := range deref(get(n, "required")).Content {
		if get(props, r.Value) == nil && !(root && slices.Contains(envelope, r.Value)) {
			m.fields = append(m.fields, field{name: r.Value, t: builtin("any"), implied: true})
		}
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
	case open:
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
	} else if deref(get(n, "exclusiveMinimum")).ShortTag() != "!!bool" {
		bound("exclusiveMinimum", ">", numbers, "v")
	}
	if isTrue(get(n, "exclusiveMaximum")) {
		maxOp = "<"
	} else if deref(get(n, "exclusiveMaximum")).ShortTag() != "!!bool" {
		bound("exclusiveMaximum", "<", numbers, "v")
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
	case t.union != nil:
		alts = nil
		for _, p := range t.union {
			alts = append(alts, p.text(indent))
		}
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
	return t.mapping != nil && len(t.mapping.fields)+len(t.mapping.merges) > 0 && !t.nullable && t.predicate() == ""
}

// text writes m in braces, a member to a line indented below indent, or on
// one line when it declares no field and merges nothing.
func (m *mapping) text(indent string) string {
	if len(m.fields) == 0 && len(m.merges) == 0 {
		if m.rest == nil {
			return "{}"
		}
		return "{(string): " + m.rest.text(indent) + "}"
	}
	var b strings.Builder
	b.WriteString("{\n")
	inner := indent + "  "
	for _, name := range m.merges {
		b.WriteString(inner + "<<: " + name + ",\n")
	}
	for _, f := range m.fields {
		b.WriteString(inner + f.head() + " " + f.t.text(inner) + f.defaultText() + ",\n")
	}
	if m.rest != nil {
		b.WriteString(inner + "(string): " + m.rest.text(inner) + ",\n")
	}
	return b.String() + indent + "}"
}

func (m *mapping) write(b *strings.Builder, indent string) {
	for _, name := range m.merges {
		b.WriteString(indent + "<<: " + name + "\n")
	}
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
