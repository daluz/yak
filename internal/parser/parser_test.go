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
			parts = append(parts, dumpEntry(e))
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
	case *ast.Unary:
		return "!" + dump(t.X)
	case *ast.Binary:
		return "(" + dump(t.X) + strings.Trim(t.Op.String(), `"`) + dump(t.Y) + ")"
	case *ast.If:
		out := "if(" + dump(t.Cond) + ";" + dump(t.Then)
		if t.Else != nil {
			out += ";" + dump(t.Else)
		}
		return out + ")"
	case *ast.SeqComp:
		return "[" + dump(t.Item) + dumpLoop(t.Loop) + "]"
	case *ast.MapComp:
		return "{" + dumpEntry(t.Entry) + dumpLoop(t.Loop) + "}"
	case *ast.Local:
		return "local(" + strings.Join(dumpBinds(t.Binds), " ") + ";" + dump(t.Body) + ")"
	case *ast.Function:
		params := make([]string, 0, len(t.Params))
		for _, p := range t.Params {
			if p.Default == nil {
				params = append(params, p.Name)
				continue
			}
			params = append(params, p.Name+"="+dump(p.Default))
		}
		return "fn(" + strings.Join(params, ",") + ";" + dump(t.Body) + ")"
	case *ast.Call:
		args := make([]string, 0, len(t.Args))
		for _, a := range t.Args {
			if a.Name == "" {
				args = append(args, dump(a.Value))
				continue
			}
			args = append(args, a.Name+"="+dump(a.Value))
		}
		return dump(t.Fn) + "(" + strings.Join(args, ",") + ")"
	case *ast.Ident:
		return "ident(" + t.Name + ")"
	default:
		return fmt.Sprintf("?%T", n)
	}
}

func dumpEntry(e *ast.Entry) string {
	sep := ":"
	switch {
	case e.Hidden:
		sep = "::"
	case e.HideNull:
		sep = ":?"
	}
	key := dump(e.Key)
	if e.Computed {
		key = "[" + key + "]"
	}
	return key + sep + dump(e.Value)
}

func dumpLoop(l ast.Loop) string {
	out := " for " + l.Var + " in " + dump(l.Source)
	if l.Filter != nil {
		out += " if " + dump(l.Filter)
	}
	return out
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
		{"hidden if null field", "a:? 1\n", `{"a":?1}`},
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
		{"flow hidden if null field", "a: {b:? 1}\n", `{"a":{"b":?1}}`},
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

func TestParseFunctions(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"one parameter", "local f(a) = a\nb: 1\n", `{f=fn(a;ident(a)) "b":1}`},
		{"several parameters", "local f(a, b) = a\nc: 1\n", `{f=fn(a,b;ident(a)) "c":1}`},
		{"no parameters", "local f() = 1\nb: 1\n", `{f=fn(;1) "b":1}`},
		{"a default", "local f(a, b = 123) = a\nc: 1\n", `{f=fn(a,b=123;ident(a)) "c":1}`},
		{"a default expression", "local f(a = .x ?? 1) = a\nb: 1\n", `{f=fn(a=(self+0.x??1);ident(a)) "b":1}`},
		{"trailing comma", "local f(a,) = a\nb: 1\n", `{f=fn(a;ident(a)) "b":1}`},
		{"block body", "local f(a) =\n  b: a\nc: 1\n", `{f=fn(a;{"b":ident(a)}) "c":1}`},
		{"in a local block", "local { f(a) = a, g = 1 }\nb: 1\n", `{f=fn(a;ident(a)) g=1 "b":1}`},

		{"positional call", "local f(a) = a\nb: f(1)\n", `{f=fn(a;ident(a)) "b":ident(f)(1)}`},
		{"empty call", "local f() = 1\nb: f()\n", `{f=fn(;1) "b":ident(f)()}`},
		{"named call", "local f(a, b) = a\nc: f(b = 2, a = 1)\n", `{f=fn(a,b;ident(a)) "c":ident(f)(b=2,a=1)}`},
		{"mixed call", "local f(a, b) = a\nc: f(1, b = 2)\n", `{f=fn(a,b;ident(a)) "c":ident(f)(1,b=2)}`},
		{"call spanning lines", "local f(a) = a\nb: f(\n  1,\n)\n", `{f=fn(a;ident(a)) "b":ident(f)(1)}`},
		{"call on a field", "a: $.f(1)\n", `{"a":root.f(1)}`},
		{"chained after a call", "local f(a) = a\nb: f(1).c[0]\n", `{f=fn(a;ident(a)) "b":ident(f)(1).c[0]}`},
		{"call of a call", "local f(a) = a\nb: f(1)(2)\n", `{f=fn(a;ident(a)) "b":ident(f)(1)(2)}`},
		{"call binds tighter than an operator", "local f(a) = a\nb: f(1) == 2\n", `{f=fn(a;ident(a)) "b":(ident(f)(1)==2)}`},
		{"call in an interpolation", "local f(a) = a\nb: \"{f(1)}\"\n", `{f=fn(a;ident(a)) "b":concat(ident(f)(1))}`},
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

func TestParseOperators(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"equality", "a: 1 == 2\n", `{"a":(1==2)}`},
		{"inequality", "a: 1 != 2\n", `{"a":(1!=2)}`},
		{"ordering", "a: 1 < 2\nb: 1 <= 2\nc: 1 > 2\nd: 1 >= 2\n", `{"a":(1<2) "b":(1<=2) "c":(1>2) "d":(1>=2)}`},
		{"negation", "a: !true\n", `{"a":!true}`},
		{"double negation needs a space", "a: ! !true\n", `{"a":!!true}`},
		{"conjunction", "a: true && false\n", `{"a":(true&&false)}`},
		{"comparison binds tighter than and", "a: 1 < 2 && 3 < 4\n", `{"a":((1<2)&&(3<4))}`},
		{"and binds tighter than or", "a: true || true && false\n", `{"a":(true||(true&&false))}`},
		{"equality binds looser than ordering", "a: 1 < 2 == true\n", `{"a":((1<2)==true)}`},
		{"negation binds tighter than comparison", "a: !true == false\n", `{"a":(!true==false)}`},
		{"coalesce binds looser than or", "a: .x ?? true || false\n", `{"a":(self+0.x??(true||false))}`},
		{"parentheses regroup", "a: (1 < 2) == (3 < 4)\n", `{"a":((1<2)==(3<4))}`},
		{"left associative", "a: 1 == 2 == true\n", `{"a":((1==2)==true)}`},
		{"in an interpolation", `a: "{1 < 2}"`, `{"a":concat((1<2))}`},
		{"greater than beats a folded scalar", "a: .x > 2\n", `{"a":(self+0.x>2)}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseOne(t, tc.src); got != tc.want {
				t.Errorf("Parse(%q) = %s, want %s", tc.src, got, tc.want)
			}
		})
	}
}

func TestParseConditionals(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"full form", "a: if true then 1 else 2\n", `{"a":if(true;1;2)}`},
		{"without else", "a: if true then 1\n", `{"a":if(true;1)}`},
		{"chained", "a: if true then 1 else if false then 2 else 3\n", `{"a":if(true;1;if(false;2;3))}`},
		{"with a comparison", "a: if .n > 1 then 1 else 2\n", `{"a":if((self+0.n>1);1;2)}`},
		{"wrapped over lines", "a:\n  if true\n  then 1\n  else 2\n", `{"a":if(true;1;2)}`},
		{"branches may be collections", "a: if true then [1] else {b: 2}\n", `{"a":if(true;[1];{"b":2})}`},
		{"inside an interpolation", `a: "{if true then "y" else "n"}"`, `{"a":concat(if(true;"y";"n"))}`},
		{"in a flow sequence", "a: [if true then 1 else 2]\n", `{"a":[if(true;1;2)]}`},
		{"parenthesized as an operand", "a: (if true then 1 else 2) == 1\n", `{"a":(if(true;1;2)==1)}`},
		{"a key named else ends it", "a: if true then 1\nelse: 2\n", `{"a":if(true;1) "else":2}`},
		{"keywords stay usable as keys", "if: 1\nthen: 2\nfor: 3\nin: 4\n", `{"if":1 "then":2 "for":3 "in":4}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseOne(t, tc.src); got != tc.want {
				t.Errorf("Parse(%q) = %s, want %s", tc.src, got, tc.want)
			}
		})
	}
}

func TestParseComprehensions(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"sequence", "a: [x for x in $$.list]\n", `{"a":[ident(x) for x in context.list]}`},
		{"sequence with a filter", "a: [x for x in $$.list if x > 1]\n", `{"a":[ident(x) for x in context.list if (ident(x)>1)]}`},
		{"sequence over a literal", "a: [x for x in [1, 2]]\n", `{"a":[ident(x) for x in [1 2]]}`},
		{"mapping with a computed key", "a: {[x]: 1 for x in $$.list}\n", `{"a":{[ident(x)]:1 for x in context.list}}`},
		{"mapping with an interpolated key", `a: {"k{x}": x for x in $$.list}`, `{"a":{concat("k",ident(x)):ident(x) for x in context.list}}`},
		{"mapping with a filter", "a: {[x]: 1 for x in $$.list if true}\n", `{"a":{[ident(x)]:1 for x in context.list if true}}`},
		{"mapping of hidden entries", "a: {[x]:: 1 for x in $$.list}\n", `{"a":{[ident(x)]::1 for x in context.list}}`},
		{"a conditional item", "a: [if x then 1 else 2 for x in $$.list]\n", `{"a":[if(ident(x);1;2) for x in context.list]}`},
		{"a conditional source", "a: [x for x in if true then [1] else [2]]\n", `{"a":[ident(x) for x in if(true;[1];[2])]}`},
		{"wrapped over lines", "a: [\n  x\n  for x in $$.list\n]\n", `{"a":[ident(x) for x in context.list]}`},
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
		{"single", `a: "{.b}"`, `{"a":concat(self+0.b)}`},
		{"surrounded", `a: "x{.b}y"`, `{"a":concat("x",self+0.b,"y")}`},
		{"nested quotes", `a: "{$$["k"]}"`, `{"a":concat(context["k"])}`},
		{"raw", `a: r"{.b}"`, `{"a":"{.b}"}`},
		{"escaped", `a: "{{.b}}"`, `{"a":"{.b}"}`},
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
		{"bindings only document", "local x = 1\n", []string{"local(x=1;null)"}},
		{"bindings only first document", "local x = 1\n---\na: 1\n", []string{"local(x=1;null)", `{"a":1}`}},
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
		{"local without a body", "a:\n  local x = 1\n", "must be followed by a value"},
		{"local without a name", "local = 1\na: 1\n", "expected the name of a binding"},
		{"local without an assignment", "local x 1\na: 1\n", `expected "=" after the name of a binding`},
		{"local naming a keyword", "local null = 1\na: 1\n", "cannot name a binding"},
		{"parameter list without an assignment", "local f(a) 1\nb: 1\n", `expected "=" after the name of a binding`},
		{"parameter naming a keyword", "local f(for) = 1\na: 1\n", `"for" is a keyword and cannot name a parameter`},
		{"parameter that is not a name", "local f(1) = 1\na: 1\n", "expected the name of a parameter"},
		{"duplicate parameter", "local f(a, a) = a\nb: 1\n", `parameter "a" is declared twice`},
		{"unseparated parameters", "local f(a b) = a\nc: 1\n", `expected "," or ")" in a parameter list`},
		{"unterminated parameter list", "local f(a = 1", "unterminated parameter list"},
		{"unseparated arguments", "local f(a) = a\nb: f(1 2)\n", `expected "," or ")" in an argument list`},
		{"unterminated argument list", "local f(a) = a\nb: f(1", "unterminated argument list"},
		{"positional argument after a named one", "local f(a, b) = a\nc: f(a = 1, 2)\n", "cannot follow a named one"},
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
		{"empty interpolation", `a: "{}"`, "empty string interpolation"},
		{"junk in interpolation", `a: "{.b .c}"`, "unexpected"},
		{"double bang is still a tag", "a: !!true\n", "tags are not supported"},
		{"if without then", "a: if true 1\n", `expected "then"`},
		{"if as an operand", "a: 1 == if true then 1 else 2\n", "wrap it in parentheses"},
		{"a keyword as a value", "a: then\n", `"then" is a keyword`},
		{"local named for", "local for = 1\na: 1\n", `"for" is a keyword and cannot name a binding`},
		{"comprehension without in", "a: [x for x of $$.l]\n", `expected "in"`},
		{"comprehension without a variable", "a: [x for 1 in $$.l]\n", "expected the name of the loop variable"},
		{"comprehension naming a keyword", "a: [x for in in $$.l]\n", "cannot name a loop variable"},
		{"comprehension after several items", "a: [1, 2 for x in $$.l]\n", `expected "," or "]"`},
		{"unterminated comprehension", "a: [x for x in $$.l\n", `expected "]" to close a comprehension`},
		{"unterminated mapping comprehension", "a: {[x]: 1 for x in $$.l\n", `expected "}" to close a comprehension`},
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
