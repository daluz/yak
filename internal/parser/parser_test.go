package parser_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/parser"
)

// dump renders a tree as a compact s-expression so that tests can state the
// expected shape on one line.
func dump(n ast.Node) string {
	switch t := n.(type) {
	case *ast.Mapping:
		parts := make([]string, 0, len(t.Binds)+len(t.Entries))
		parts = append(parts, dumpBinds(t.Binds)...)
		for _, e := range t.Entries {
			sep := ":"
			switch {
			case e.Hidden:
				sep = "::"
			case e.HideNull:
				sep = "::?"
			}
			key := dump(e.Key)
			if e.Computed {
				key = "[" + key + "]"
			}
			parts = append(parts, key+sep+dump(e.Value))
		}
		return "{" + strings.Join(parts, " ") + "}"
	case *ast.Sequence:
		parts := make([]string, 0, len(t.Items))
		for _, item := range t.Items {
			parts = append(parts, dump(item.Value))
		}
		return "[" + strings.Join(parts, " ") + "]"
	case *ast.String:
		if t.IsLiteral() {
			return fmt.Sprintf("%q", t.Literal())
		}
		parts := make([]string, 0, len(t.Parts))
		for _, p := range t.Parts {
			if p.Expr == nil {
				parts = append(parts, fmt.Sprintf("%q", p.Text))
				continue
			}
			parts = append(parts, dump(p.Expr))
		}
		return "concat(" + strings.Join(parts, ",") + ")"
	case *ast.Int:
		return fmt.Sprintf("%d", t.Value)
	case *ast.Float:
		return fmt.Sprintf("%g", t.Value)
	case *ast.Bool:
		return fmt.Sprintf("%t", t.Value)
	case *ast.Null:
		return "null"
	case *ast.Self:
		return "self+" + fmt.Sprint(t.Up)
	case *ast.Root:
		return "root"
	case *ast.Context:
		return "context"
	case *ast.Field:
		return dump(t.X) + optional(t.Optional) + "." + t.Name
	case *ast.Index:
		return dump(t.X) + optional(t.Optional) + "[" + dump(t.Index) + "]"
	case *ast.Coalesce:
		return "(" + dump(t.X) + "??" + dump(t.Y) + ")"
	case *ast.Local:
		return "local(" + strings.Join(dumpBinds(t.Binds), " ") + ";" + dump(t.Body) + ")"
	case *ast.Ident:
		return "ident(" + t.Name + ")"
	default:
		return fmt.Sprintf("?%T", n)
	}
}

func dumpBinds(binds []*ast.Binding) []string {
	parts := make([]string, 0, len(binds))
	for _, b := range binds {
		parts = append(parts, b.Name+"="+dump(b.Value))
	}
	return parts
}

func optional(v bool) string {
	if v {
		return "?"
	}
	return ""
}

func parseOne(t *testing.T, src string) string {
	t.Helper()
	stream, err := parser.Parse("test.yak", []byte(src))
	if err != nil {
		t.Fatalf("Parse(%q) returned error: %v", src, err)
	}
	if len(stream.Docs) != 1 {
		t.Fatalf("Parse(%q) produced %d documents, want 1", src, len(stream.Docs))
	}
	return dump(stream.Docs[0].Body)
}

func TestParseBlockStructures(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"simple mapping", "a: 1\nb: 2\n", `{"a":1 "b":2}`},
		{"nested mapping", "a:\n  b: 1\n", `{"a":{"b":1}}`},
		{"null value", "a:\nb: 1\n", `{"a":null "b":1}`},
		{"explicit null", "a: null\n", `{"a":null}`},
		{"booleans", "a: true\nb: false\n", `{"a":true "b":false}`},
		{"floats", "a: 1.5\n", `{"a":1.5}`},
		{"hidden field", "a:: 1\n", `{"a"::1}`},
		{"hidden if null field", "a::? 1\n", `{"a"::?1}`},
		{"quoted key", `"a b": 1`, `{"a b":1}`},
		{"kebab key", "a-b: 1\n", `{"a-b":1}`},
		{"computed key", "[$$.k]: 1\n", `{[context.k]:1}`},
		{"sequence under key", "a:\n  - 1\n  - 2\n", `{"a":[1 2]}`},
		{"sequence at key indent", "a:\n- 1\n- 2\n", `{"a":[1 2]}`},
		{"mapping in sequence", "- a: 1\n  b: 2\n", `[{"a":1 "b":2}]`},
		{"nested sequence", "- - 1\n  - 2\n", `[[1 2]]`},
		{"empty sequence item", "-\n- 1\n", `[null 1]`},
		{"flow sequence", "a: [1, 2]\n", `{"a":[1 2]}`},
		{"flow mapping", "a: {b: 1, c: 2}\n", `{"a":{"b":1 "c":2}}`},
		{"flow trailing comma", "a: [1, 2,]\n", `{"a":[1 2]}`},
		{"flow hidden field", "a: {b:: 1}\n", `{"a":{"b"::1}}`},
		{"flow hidden if null field", "a: {b::? 1}\n", `{"a":{"b"::?1}}`},
		{"scalar document", `"hello"`, `"hello"`},
		{"comment only lines", "# c\na: 1 # d\n", `{"a":1}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseOne(t, tc.src); got != tc.want {
				t.Errorf("Parse(%q) = %s, want %s", tc.src, got, tc.want)
			}
		})
	}
}

func TestParseReferences(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"self", "a: .\n", `{"a":self+0}`},
		{"self field", "a: .b\n", `{"a":self+0.b}`},
		{"self nested field", "a: .b.c\n", `{"a":self+0.b.c}`},
		{"parent", "a: ..\n", `{"a":self+1}`},
		{"parent field", "a: ..b\n", `{"a":self+1.b}`},
		{"grandparent field", "a: ...b\n", `{"a":self+2.b}`},
		{"root", "a: $\n", `{"a":root}`},
		{"root field", "a: $.b\n", `{"a":root.b}`},
		{"context long", "a: $context.b\n", `{"a":context.b}`},
		{"context short", "a: $$.b\n", `{"a":context.b}`},
		{"index", "a: $.b[0]\n", `{"a":root.b[0]}`},
		{"string index", `a: $.b["c"]`, `{"a":root.b["c"]}`},
		{"chained index", "a: $.b[0].c\n", `{"a":root.b[0].c}`},
		{"parenthesized", "a: ($.b).c\n", `{"a":root.b.c}`},
		{"optional field", "a: $$?.b\n", `{"a":context?.b}`},
		{"optional index", "a: $$?[0]\n", `{"a":context?[0]}`},
		{"optional chain", "a: $$?.b?.c\n", `{"a":context?.b?.c}`},
		{"mixed chain", "a: $$?.b.c\n", `{"a":context?.b.c}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseOne(t, tc.src); got != tc.want {
				t.Errorf("Parse(%q) = %s, want %s", tc.src, got, tc.want)
			}
		})
	}
}

func TestParseLocals(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"binding before a mapping", "local x = 1\na: x\n", `{x=1 "a":ident(x)}`},
		{"binding between entries", "a: x\nlocal x = 1\nb: 2\n", `{x=1 "a":ident(x) "b":2}`},
		{"several bindings", "local x = 1\nlocal y = 2\na: x\n", `{x=1 y=2 "a":ident(x)}`},
		{"block form", "local {\n  x = 1\n  y = 2\n}\na: x\n", `{x=1 y=2 "a":ident(x)}`},
		{"block form on one line", "local { x = 1, y = 2 }\na: x\n", `{x=1 y=2 "a":ident(x)}`},
		{"block value", "local x =\n  a: 1\nb: x\n", `{x={"a":1} "b":ident(x)}`},
		{"sequence value", "local x =\n  - 1\nb: x\n", `{x=[1] "b":ident(x)}`},
		{"nested scope", "a:\n  local x = 1\n  b: x\n", `{"a":{x=1 "b":ident(x)}}`},
		{"in a sequence item", "- local x = 1\n  a: x\n", `[{x=1 "a":ident(x)}]`},
		{"before a sequence", "local x = 1\n- x\n", `local(x=1;[ident(x)])`},
		{"before a scalar", "local x = 1\nx\n", `local(x=1;ident(x))`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseOne(t, tc.src); got != tc.want {
				t.Errorf("Parse(%q) = %s, want %s", tc.src, got, tc.want)
			}
		})
	}
}

func TestParseCoalesce(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"simple", "a: .b ?? 1\n", `{"a":(self+0.b??1)}`},
		{"without spaces", "a: .b??1\n", `{"a":(self+0.b??1)}`},
		{"chained", "a: .b ?? .c ?? 1\n", `{"a":((self+0.b??self+0.c)??1)}`},
		{"with optional access", "a: $$?.b ?? 1\n", `{"a":(context?.b??1)}`},
		{"inside a flow sequence", "a: [.b ?? 1]\n", `{"a":[(self+0.b??1)]}`},
		{"inside an index", "a: $.b[.c ?? 0]\n", `{"a":root.b[(self+0.c??0)]}`},
		{"postfix binds tighter", "a: .b ?? .c.d\n", `{"a":(self+0.b??self+0.c.d)}`},
		{"parentheses regroup", "a: (.b ?? .c).d\n", `{"a":(self+0.b??self+0.c).d}`},
		{"right hand side may wrap", "a: .b ??\n  1\n", `{"a":(self+0.b??1)}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseOne(t, tc.src); got != tc.want {
				t.Errorf("Parse(%q) = %s, want %s", tc.src, got, tc.want)
			}
		})
	}
}

func TestParseInterpolation(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"single", `a: "${.b}"`, `{"a":concat(self+0.b)}`},
		{"surrounded", `a: "x${.b}y"`, `{"a":concat("x",self+0.b,"y")}`},
		{"nested quotes", `a: "${$$["k"]}"`, `{"a":concat(context["k"])}`},
		{"raw", `a: r"${.b}"`, `{"a":"${.b}"}`},
		{"escaped", `a: "$${.b}"`, `{"a":"${.b}"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseOne(t, tc.src); got != tc.want {
				t.Errorf("Parse(%q) = %s, want %s", tc.src, got, tc.want)
			}
		})
	}
}

func TestParseDocuments(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{"single implicit", "a: 1\n", []string{`{"a":1}`}},
		{"two documents", "a: 1\n---\nb: 2\n", []string{`{"a":1}`, `{"b":2}`}},
		{"leading marker", "---\na: 1\n", []string{`{"a":1}`}},
		{"trailing end marker", "a: 1\n...\n", []string{`{"a":1}`}},
		{"empty file", "", []string{"null"}},
		{"comment only file", "# nothing\n", []string{"null"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stream, err := parser.Parse("test.yak", []byte(tc.src))
			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", tc.src, err)
			}
			if len(stream.Docs) != len(tc.want) {
				t.Fatalf("Parse(%q) produced %d documents, want %d", tc.src, len(stream.Docs), len(tc.want))
			}
			for i, want := range tc.want {
				if got := dump(stream.Docs[i].Body); got != want {
					t.Errorf("document %d = %s, want %s", i, got, want)
				}
			}
		})
	}
}

// dumpComments lists where each comment of a tree ended up, so that a test
// can state the whole outcome on one line.
func dumpComments(n ast.Node) []string {
	var out []string
	switch t := n.(type) {
	case *ast.Local:
		out = append(out, dumpComments(t.Body)...)
	case *ast.Mapping:
		for _, e := range t.Entries {
			out = append(out, labelComments(dump(e.Key), e.Comments)...)
			out = append(out, dumpComments(e.Value)...)
		}
	case *ast.Sequence:
		for i, item := range t.Items {
			out = append(out, labelComments(fmt.Sprintf("[%d]", i), item.Comments)...)
			out = append(out, dumpComments(item.Value)...)
		}
	}
	return out
}

func labelComments(at string, c ast.Comments) []string {
	var out []string
	for _, head := range c.Head {
		out = append(out, at+" head "+head)
	}
	if c.Line != "" {
		out = append(out, at+" line "+c.Line)
	}
	for _, foot := range c.Foot {
		out = append(out, at+" foot "+foot)
	}
	return out
}

func TestParseComments(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"above an entry", "# c\na: 1\n", `"a" head # c`},
		{"beside an entry", "a: 1 # c\n", `"a" line # c`},
		{"a whole paragraph", "# one\n# two\na: 1\n", `"a" head # one; "a" head # two`},
		{"above a nested entry", "a:\n  # c\n  b: 1\n", `"b" head # c`},
		{"beside a key that opens a block", "a: # c\n  b: 1\n", `"a" line # c`},
		{"at the end of a block", "a:\n  b: 1\n  # c\nd: 2\n", `"d" head # c`},
		{"on a sequence item", "a:\n  # c\n  - 1 # d\n", `[0] head # c; [0] line # d`},
		{"on a mapping in a sequence", "# c\n- a: 1 # d\n", `[0] head # c; "a" line # d`},
		{"at the end of a document", "a: 1\n# c\n", `"a" foot # c`},
		{"at the end of the first document", "a: 1\n# c\n---\nb: 2\n", `"a" foot # c`},

		{"above a binding", "# c\nlocal x = 1\na: x\n", `"a" head # c`},
		{"beside a binding", "local x = 1 # c\na: x\n", ""},
		{"inside a binding's value", "local x =\n  # c\n  b: 1\na: x\n", ""},
		{"inside a local block", "local {\n  # c\n  x = 1\n}\na: x\n", ""},
		{"above a binding in a block", "a:\n  # c\n  local x = 1\n  b: x\n", `"b" head # c`},

		{"marked for the template", "#local c\na: 1\n", ""},
		{"marked beside an entry", "a: 1 #local c\n", ""},
		{"marked with no text", "#local\na: 1\n", ""},
		{"a word that starts with local", "#localhost c\na: 1\n", `"a" head #localhost c`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stream, err := parser.Parse("test.yak", []byte(tc.src))
			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", tc.src, err)
			}
			var got []string
			for _, doc := range stream.Docs {
				got = append(got, dumpComments(doc.Body)...)
			}
			if joined := strings.Join(got, "; "); joined != tc.want {
				t.Errorf("Parse(%q) placed comments %q, want %q", tc.src, joined, tc.want)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"unquoted multiword string", "a: hello world\n", "strings must be quoted"},
		{"local as a value", "a: local\n", `a "local" binding is a statement of its own`},
		{"import keyword", "a: import\n", `"import" is not implemented yet`},
		{"schema keyword", "a: schema\n", `"schema" is not implemented yet`},
		{"reserved key", "local: 1\n", "reserved word"},
		{"local without a body", "local x = 1\n", "must be followed by a value"},
		{"local without a name", "local = 1\na: 1\n", "expected the name of a binding"},
		{"local without an assignment", "local x 1\na: 1\n", `expected "=" after the name of a binding`},
		{"local naming a keyword", "local null = 1\na: 1\n", "cannot name a binding"},
		{"local function", "local f(x) = x\na: 1\n", `"local" functions are not implemented yet`},
		{"over-indented local", "local x = 1\n  local y = 2\na: x\n", "unexpected indentation"},
		{"empty local block", "local {}\na: 1\n", "at least one binding"},
		{"unterminated local block", "local {\n  x = 1\n", "unterminated"},
		{"unseparated bindings", "local { x = 1 y = 2 }\na: x\n", "between bindings"},
		{"lone question mark", "a: 1 ? 2\n", "explicit key indicators"},
		{"coalesce on its own line", "a: .b\n?? 1\n", "expected a mapping key"},
		{"numeric key", "1: a\n", "numeric keys must be quoted"},
		{"bad indentation", "a: 1\n  b: 2\n", "unexpected indentation"},
		{"stray token after value", "a: 1 2\n", "unexpected"},
		{"unterminated flow sequence", "a: [1, 2\n", "unterminated flow sequence"},
		{"unterminated flow mapping", "a: {b: 1\n", "unterminated flow mapping"},
		{"dots in postfix", "a: $.b..c\n", "may only begin a reference"},
		{"dangling dot", "a: $. b\n", "expected a field name"},
		{"unknown special variable", "a: $ctx\n", "unknown special variable"},
		{"empty interpolation", `a: "${}"`, "empty string interpolation"},
		{"junk in interpolation", `a: "${.b .c}"`, "unexpected"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parser.Parse("test.yak", []byte(tc.src))
			if err == nil {
				t.Fatalf("Parse(%q) succeeded, want an error containing %q", tc.src, tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Parse(%q) error = %q, want it to contain %q", tc.src, err, tc.want)
			}
		})
	}
}

func TestErrorsCarryPositions(t *testing.T) {
	_, err := parser.Parse("test.yak", []byte("a: 1\nb: hello world\n"))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.HasPrefix(err.Error(), "test.yak:2:10:") {
		t.Errorf("error = %q, want it to start with test.yak:2:10:", err)
	}
}
