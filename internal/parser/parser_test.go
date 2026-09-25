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
		parts := make([]string, 0, len(t.Entries))
		for _, e := range t.Entries {
			sep := ":"
			if e.Hidden {
				sep = "::"
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
			parts = append(parts, dump(item))
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
		return dump(t.X) + "." + t.Name
	case *ast.Index:
		return dump(t.X) + "[" + dump(t.Index) + "]"
	case *ast.Ident:
		return "ident(" + t.Name + ")"
	default:
		return fmt.Sprintf("?%T", n)
	}
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

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"unquoted multiword string", "a: hello world\n", "strings must be quoted"},
		{"yaml yes", "a: yes\n", `"yes" is not a boolean in yak`},
		{"yaml off", "a: off\n", `"off" is not a boolean in yak`},
		{"local keyword", "a: local\n", `"local" bindings are not implemented yet`},
		{"import keyword", "a: import\n", `"import" is not implemented yet`},
		{"schema keyword", "a: schema\n", `"schema" is not implemented yet`},
		{"reserved key", "local: 1\n", "reserved word"},
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
