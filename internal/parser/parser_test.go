package parser_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/parser"
	"github.com/daluz/yak/internal/token"
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
	case *ast.Yak:
		return "yak"
	case *ast.FilePath:
		return "filepath"
	case *ast.ContextPaths:
		return "contextpaths"
	case *ast.Workdir:
		return "workdir"
	case *ast.Field:
		return dump(t.X) + optional(t.Optional) + "." + t.Name
	case *ast.Index:
		return dump(t.X) + optional(t.Optional) + "[" + dump(t.Index) + "]"
	case *ast.Coalesce:
		return "(" + dump(t.X) + "??" + dump(t.Y) + ")"
	case *ast.Unary:
		if t.Op == token.Not {
			return "not(" + dump(t.X) + ")"
		}
		return strings.Trim(t.Op.String(), `"`) + dump(t.X)
	case *ast.Binary:
		op := strings.Trim(t.Op.String(), `"`)
		if t.Op == token.And || t.Op == token.Or || t.Op == token.In || t.Op == token.NotIn {
			op = " " + op + " "
		}
		return "(" + dump(t.X) + op + dump(t.Y) + ")"
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
	case *ast.Let:
		return "let(" + strings.Join(dumpBinds(t.Binds), " ") + ";" + dump(t.Body) + ")"
	case *ast.Function:
		params := make([]string, 0, len(t.Params))
		for _, p := range t.Params {
			name := p.Name
			if p.Pattern != nil {
				name = dumpPattern(p.Pattern)
			}
			if p.Default == nil {
				params = append(params, name)
				continue
			}
			params = append(params, name+"="+dump(p.Default))
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
	key := "<<"
	if !e.Merge {
		key = dump(e.Key)
	}
	if e.Computed {
		key = "(" + key + ")"
	}
	return key + sep + dump(e.Value)
}

func dumpLoop(l ast.Loop) string {
	out := " for " + dumpPattern(l.Pattern) + " in " + dump(l.Source)
	if l.Filter != nil {
		out += " if " + dump(l.Filter)
	}
	return out
}

func dumpPattern(p *ast.Pattern) string {
	if p.Items == nil {
		return p.Name
	}
	parts := make([]string, len(p.Items))
	for i, item := range p.Items {
		parts[i] = dumpPattern(item)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func bindingName(b *ast.Binding) string {
	if b.Pattern != nil {
		return dumpPattern(b.Pattern)
	}
	return b.Name
}

func dumpBinds(binds []*ast.Binding) []string {
	parts := make([]string, 0, len(binds))
	for _, b := range binds {
		parts = append(parts, bindingName(b)+"="+dump(b.Value))
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
		{"merge entry", "<<: x\na: 1\n", `{<<:ident(x) "a":1}`},
		{"hidden merge entry", "<<:: x\n", `{<<::ident(x)}`},
		{"optional merge entry", "<<:? x\n", `{<<:?ident(x)}`},
		{"flow merge entry", "a: {<<: x, b: 1}\n", `{"a":{<<:ident(x) "b":1}}`},
		{"a quoted merge is an ordinary key", `"<<": 1`, `{"<<":1}`},
		{"quoted key", `"a b": 1`, `{"a b":1}`},
		{"kebab key", "a-b: 1\n", `{"a-b":1}`},
		{"computed key", "($$.k): 1\n", `{(context.k):1}`},
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
		{"root long", "a: $root\n", `{"a":root}`},
		{"root long field", "a: $root.b\n", `{"a":root.b}`},
		{"context long", "a: $context.b\n", `{"a":context.b}`},
		{"context short", "a: $$.b\n", `{"a":context.b}`},
		{"context ctx", "a: $ctx.b\n", `{"a":context.b}`},
		{"yak", "a: $yak\n", `{"a":yak}`},
		{"yak field", "a: $yak.version\n", `{"a":yak.version}`},
		{"filepath", "a: $filepath\n", `{"a":filepath}`},
		{"contextpaths index", "a: $contextpaths[0]\n", `{"a":contextpaths[0]}`},
		{"workdir", "a: $workdir\n", `{"a":workdir}`},
		{"filepath in an interpolation", `a: "\($filepath)"`, `{"a":concat(filepath)}`},
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

func TestParseLets(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"binding before a mapping", "let x = 1\na: x\n", `{x=1 "a":ident(x)}`},
		{"binding between entries", "a: x\nlet x = 1\nb: 2\n", `{x=1 "a":ident(x) "b":2}`},
		{"several bindings", "let x = 1\nlet y = 2\na: x\n", `{x=1 y=2 "a":ident(x)}`},
		{"block form", "let {\n  x = 1\n  y = 2\n}\na: x\n", `{x=1 y=2 "a":ident(x)}`},
		{"block form on one line", "let { x = 1, y = 2 }\na: x\n", `{x=1 y=2 "a":ident(x)}`},
		{"block value", "let x =\n  a: 1\nb: x\n", `{x={"a":1} "b":ident(x)}`},
		{"sequence value", "let x =\n  - 1\nb: x\n", `{x=[1] "b":ident(x)}`},
		{"pattern", "let [a, b] = p\nc: a\n", `{[a b]=ident(p) "c":ident(a)}`},
		{"nested pattern with a wildcard", "let [a, [_, b]] = p\nc: a\n", `{[a [_ b]]=ident(p) "c":ident(a)}`},
		{"pattern in a block", "let {\n  p = [1, 2]\n  [a, b] = p\n}\nc: a\n", `{p=[1 2] [a b]=ident(p) "c":ident(a)}`},
		{"pattern in a block on one line", "let { p = [1, 2], [a, b] = p }\nc: a\n", `{p=[1 2] [a b]=ident(p) "c":ident(a)}`},
		{"pattern with a block value", "let [a, b] =\n  - 1\n  - 2\nc: a\n", `{[a b]=[1 2] "c":ident(a)}`},
		{"pattern without brackets", "let a, b = p\nc: a\n", `{[a b]=ident(p) "c":ident(a)}`},
		{"pattern without brackets around a nested one", "let _, [a, b] = p\nc: a\n", `{[_ [a b]]=ident(p) "c":ident(a)}`},
		{"pattern without brackets after a bracketed one", "let [a, b], c = p\nd: a\n", `{[[a b] c]=ident(p) "d":ident(a)}`},
		{"one-item pattern", "let [a] = p\nc: a\n", `{[a]=ident(p) "c":ident(a)}`},
		{"pattern without brackets in a block", "let { x = 1, a, b = p, y = 2 }\nc: a\n", `{x=1 [a b]=ident(p) y=2 "c":ident(a)}`},
		{"nested scope", "a:\n  let x = 1\n  b: x\n", `{"a":{x=1 "b":ident(x)}}`},
		{"in a sequence item", "- let x = 1\n  a: x\n", `[{x=1 "a":ident(x)}]`},
		{"before a sequence", "let x = 1\n- x\n", `let(x=1;[ident(x)])`},
		{"before a scalar", "let x = 1\nx\n", `let(x=1;ident(x))`},
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
		{"one parameter", "let f(a) = a\nb: 1\n", `{f=fn(a;ident(a)) "b":1}`},
		{"several parameters", "let f(a, b) = a\nc: 1\n", `{f=fn(a,b;ident(a)) "c":1}`},
		{"no parameters", "let f() = 1\nb: 1\n", `{f=fn(;1) "b":1}`},
		{"a default", "let f(a, b = 123) = a\nc: 1\n", `{f=fn(a,b=123;ident(a)) "c":1}`},
		{"a default expression", "let f(a = .x ?? 1) = a\nb: 1\n", `{f=fn(a=(self+0.x??1);ident(a)) "b":1}`},
		{"trailing comma", "let f(a,) = a\nb: 1\n", `{f=fn(a;ident(a)) "b":1}`},
		{"block body", "let f(a) =\n  b: a\nc: 1\n", `{f=fn(a;{"b":ident(a)}) "c":1}`},
		{"in a let block", "let { f(a) = a, g = 1 }\nb: 1\n", `{f=fn(a;ident(a)) g=1 "b":1}`},
		{"a pattern parameter", "let f([a, b], c) = a\nd: 1\n", `{f=fn([a b],c;ident(a)) "d":1}`},
		{"a nested pattern parameter", "let f([a, [_, b]]) = a\nc: 1\n", `{f=fn([a [_ b]];ident(a)) "c":1}`},
		{"a pattern parameter with a default", "let f([a, b] = [1, 2]) = a\nc: 1\n", `{f=fn([a b]=[1 2];ident(a)) "c":1}`},
		{"a pattern parameter of a lambda", "a: ([k, v]) => v\n", `{"a":fn([k v];ident(v))}`},

		{"positional call", "let f(a) = a\nb: f(1)\n", `{f=fn(a;ident(a)) "b":ident(f)(1)}`},
		{"empty call", "let f() = 1\nb: f()\n", `{f=fn(;1) "b":ident(f)()}`},
		{"named call", "let f(a, b) = a\nc: f(b = 2, a = 1)\n", `{f=fn(a,b;ident(a)) "c":ident(f)(b=2,a=1)}`},
		{"mixed call", "let f(a, b) = a\nc: f(1, b = 2)\n", `{f=fn(a,b;ident(a)) "c":ident(f)(1,b=2)}`},
		{"call spanning lines", "let f(a) = a\nb: f(\n  1,\n)\n", `{f=fn(a;ident(a)) "b":ident(f)(1)}`},
		{"call on a field", "a: $.f(1)\n", `{"a":root.f(1)}`},
		{"chained after a call", "let f(a) = a\nb: f(1).c[0]\n", `{f=fn(a;ident(a)) "b":ident(f)(1).c[0]}`},
		{"call of a call", "let f(a) = a\nb: f(1)(2)\n", `{f=fn(a;ident(a)) "b":ident(f)(1)(2)}`},
		{"call binds tighter than an operator", "let f(a) = a\nb: f(1) == 2\n", `{f=fn(a;ident(a)) "b":(ident(f)(1)==2)}`},
		{"call in an interpolation", "let f(a) = a\nb: \"\\(f(1))\"\n", `{f=fn(a;ident(a)) "b":concat(ident(f)(1))}`},
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
		{"negation", "a: not true\n", `{"a":not(true)}`},
		{"double negation", "a: not not true\n", `{"a":not(not(true))}`},
		{"not binds looser than comparison", "a: not 1 == 2\n", `{"a":not((1==2))}`},
		{"not binds looser than membership", "a: not x in y\n", `{"a":not((ident(x) in ident(y)))}`},
		{"not binds tighter than and", "a: not x and y\n", `{"a":(not(ident(x)) and ident(y))}`},
		{"not on the right of and", "a: x or not y\n", `{"a":(ident(x) or not(ident(y)))}`},
		{"not in", "a: x not in y\n", `{"a":(ident(x) not in ident(y))}`},
		{"not in binds like ordering", "a: x not in y == true\n", `{"a":((ident(x) not in ident(y))==true)}`},
		{"not in a condition", "a: if not x then 1\n", `{"a":if(not(ident(x));1)}`},
		{"not stays usable as a key", "not: 1\n", `{"not":1}`},
		{"conjunction", "a: true and false\n", `{"a":(true and false)}`},
		{"comparison binds tighter than and", "a: 1 < 2 and 3 < 4\n", `{"a":((1<2) and (3<4))}`},
		{"and binds tighter than or", "a: true or true and false\n", `{"a":(true or (true and false))}`},
		{"membership", "a: x in y\n", `{"a":(ident(x) in ident(y))}`},
		{"membership binds like ordering", "a: x + 1 in y == true\n", `{"a":(((ident(x)+1) in ident(y))==true)}`},
		{"membership binds tighter than and", "a: x in y and z\n", `{"a":((ident(x) in ident(y)) and ident(z))}`},
		{"membership in a comprehension filter", "a: [x for x in y if x in z]\n", `{"a":[ident(x) for x in ident(y) if (ident(x) in ident(z))]}`},
		{"word operators stay usable as keys", "a: 1\nand: 2\nor: 3\n", `{"a":1 "and":2 "or":3}`},
		{"equality binds looser than ordering", "a: 1 < 2 == true\n", `{"a":((1<2)==true)}`},
		{"coalesce binds looser than or", "a: .x ?? true or false\n", `{"a":(self+0.x??(true or false))}`},
		{"parentheses regroup", "a: (1 < 2) == (3 < 4)\n", `{"a":((1<2)==(3<4))}`},
		{"left associative", "a: 1 == 2 == true\n", `{"a":((1==2)==true)}`},
		{"in an interpolation", `a: "\(1 < 2)"`, `{"a":concat((1<2))}`},
		{"greater than beats a folded scalar", "a: .x > 2\n", `{"a":(self+0.x>2)}`},
		{"addition", "a: 1 + 2\n", `{"a":(1+2)}`},
		{"subtraction", "a: 1 - 2\n", `{"a":(1-2)}`},
		{"multiplication", "a: 1 * 2\n", `{"a":(1*2)}`},
		{"division", "a: 1 / 2\n", `{"a":(1/2)}`},
		{"remainder", "a: 1 % 2\n", `{"a":(1%2)}`},
		{"multiplication binds tighter than addition", "a: 1 + 2 * 3\n", `{"a":(1+(2*3))}`},
		{"division binds tighter than subtraction", "a: 1 - 4 / 2\n", `{"a":(1-(4/2))}`},
		{"addition binds tighter than comparison", "a: 1 + 2 < 4\n", `{"a":((1+2)<4)}`},
		{"arithmetic is left associative", "a: 1 - 2 - 3\n", `{"a":((1-2)-3)}`},
		{"merge binds like addition", "a: x + y << z\n", `{"a":((ident(x)+ident(y))<<ident(z))}`},
		{"merge binds tighter than comparison", "a: x << y == z\n", `{"a":((ident(x)<<ident(y))==ident(z))}`},
		{"negating a reference", "a: -x\n", `{"a":-ident(x)}`},
		{"a negative literal keeps its sign", "a: -1\n", `{"a":-1}`},
		{"subtracting a negation", "a: 1 - -x\n", `{"a":(1--ident(x))}`},
		{"negation binds tighter than multiplication", "a: -x * 2\n", `{"a":(-ident(x)*2)}`},
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
		{"inside an interpolation", `a: "\(if true then "y" else "n")"`, `{"a":concat(if(true;"y";"n"))}`},
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
		{"mapping with a computed key", "a: {(x): 1 for x in $$.list}\n", `{"a":{(ident(x)):1 for x in context.list}}`},
		{"mapping with an interpolated key", `a: {"k\(x)": x for x in $$.list}`, `{"a":{concat("k",ident(x)):ident(x) for x in context.list}}`},
		{"mapping with a filter", "a: {(x): 1 for x in $$.list if true}\n", `{"a":{(ident(x)):1 for x in context.list if true}}`},
		{"mapping of hidden entries", "a: {(x):: 1 for x in $$.list}\n", `{"a":{(ident(x))::1 for x in context.list}}`},
		{"a conditional item", "a: [if x then 1 else 2 for x in $$.list]\n", `{"a":[if(ident(x);1;2) for x in context.list]}`},
		{"a conditional source", "a: [x for x in if true then [1] else [2]]\n", `{"a":[ident(x) for x in if(true;[1];[2])]}`},
		{"wrapped over lines", "a: [\n  x\n  for x in $$.list\n]\n", `{"a":[ident(x) for x in context.list]}`},
		{"a bracketed pattern", "a: [k for [k, v] in $$.list]\n", `{"a":[ident(k) for [k v] in context.list]}`},
		{"several names", "a: [k for k, v in $$.list]\n", `{"a":[ident(k) for [k v] in context.list]}`},
		{"three names", "a: [b for a, b, c in $$.list]\n", `{"a":[ident(b) for [a b c] in context.list]}`},
		{"a nested pattern", "a: [x for k, [x, y] in $$.list]\n", `{"a":[ident(x) for [k [x y]] in context.list]}`},
		{"wildcards", "a: [v for _, v, _ in $$.list]\n", `{"a":[ident(v) for [_ v _] in context.list]}`},
		{"a lone wildcard", "a: [1 for _ in $$.list]\n", `{"a":[1 for _ in context.list]}`},
		{"a pattern in a mapping", "a: {(k): v for k, v in $$.list}\n", `{"a":{(ident(k)):ident(v) for [k v] in context.list}}`},
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
		{"single", `a: "\(.b)"`, `{"a":concat(self+0.b)}`},
		{"surrounded", `a: "x\(.b)y"`, `{"a":concat("x",self+0.b,"y")}`},
		{"nested quotes", `a: "\($$["k"])"`, `{"a":concat(context["k"])}`},
		{"raw", `a: r"\(.b)"`, `{"a":"\\(.b)"}`},
		{"escaped", `a: "\\(.b)"`, `{"a":"\\(.b)"}`},
		{"braces", `a: "{.b}"`, `{"a":"{.b}"}`},
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
		{"bindings only document", "let x = 1\n", []string{"let(x=1;null)"}},
		{"bindings only first document", "let x = 1\n---\na: 1\n", []string{"let(x=1;null)", `{"a":1}`}},
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
	case *ast.Let:
		out = append(out, dumpBindings(t.Binds)...)
		out = append(out, dumpComments(t.Body)...)
	case *ast.Mapping:
		out = append(out, dumpBindings(t.Binds)...)
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

// dumpBindings lists the comments that the bindings of a block carry, which
// travel to wherever each binding is read back.
func dumpBindings(binds []*ast.Binding) []string {
	var out []string
	for _, b := range binds {
		out = append(out, labelComments("let "+bindingName(b), b.Comments)...)
		out = append(out, dumpComments(b.Value)...)
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

		{"above a binding", "# c\nlet x = 1\na: x\n", `let x head # c`},
		{"beside a binding", "let x = 1 # c\na: x\n", `let x line # c`},
		{"inside a binding's value", "let x =\n  # c\n  b: 1\na: x\n", `"b" head # c`},
		{"inside a let block", "let {\n  # c\n  x = 1\n}\na: x\n", `let x head # c`},
		{"above a let block", "# c\nlet {\n  x = 1\n}\na: x\n", `let x head # c`},
		{"beside a binding in a block", "let {\n  x = 1 # c\n  y = 2\n}\na: x\n", `let x line # c`},
		{"above a binding in a block", "a:\n  # c\n  let x = 1\n  b: x\n", `let x head # c`},
		{"inside a schema", "schema A =\n  # c\n  b: string # d\na: 1\n", ""},
		{"below a schema", "schema A =\n  b: string\n# c\na: 1\n", `"a" head # c`},

		{"marked for the template", "## c\na: 1\n", ""},
		{"marked beside an entry", "a: 1 ## c\n", ""},
		{"marked with no text", "##\na: 1\n", ""},
		{"no space after the marker", "##c\na: 1\n", `"a" head ##c`},
		{"a banner of three hashes", "### c\na: 1\n", `"a" head ### c`},
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
		{"let as a value", "a: let\n", `a "let" binding is a statement of its own`},
		{"import as a value", "a: import\n", `an "import" is a statement of its own`},
		{"import without a body", "a:\n  import m\n", `an "import" must be followed by a value`},
		{"import without a path", "import\na: 1\n", "expected the path of a module"},
		{"import with a trailing dot", "import a.\nb: 1\n", "expected a segment of the module path"},
		{"import with a double dot", "import a..b\nc: 1\n", "segments are separated by a single"},
		{"import with spaced dots", "import a .b\nc: 1\n", `unexpected "." after the path of an "import"`},
		{"import of a string", "import \"a\"\nb: 1\n", "expected the path of a module"},
		{"import naming a keyword", "import a.for\nb: 1\n", `bind it to another name with "import name = a.for"`},
		{"import alias naming a keyword", "import null = a\nb: 1\n", "cannot name a module"},
		{"empty import block", "import {}\na: 1\n", `the "import" block must declare at least one binding`},
		{"unseparated imports", "import { a = x b = y }\nc: 1\n", "between bindings"},
		{"schema as a value", "a: schema\n", `a "schema" is a statement of its own, not a value`},
		{"schema without a value", "a:\n  schema A = string\n", `a "schema" must be followed by a value`},
		{"schema without an assignment", "schema A: string\na: 1\n", `expected "=" after the name of a schema`},
		{"schema without a type", "schema A =\na: 1\n", `expected a type after "="`},
		{"schema named after a type", "schema string = integer\na: 1\n", `"string" is a built-in type and cannot name a schema`},
		{"schema named after a keyword", "schema if = integer\na: 1\n", `"if" is a keyword and cannot name a schema`},
		{"spaced star", "schema *A = integer\na: 1\n", `an open schema is written "schema*"`},
		{"schema declared twice", "schema A = string\nschema A = integer\na: 1\n", `schema "A" is declared twice in this block`},
		{"schema and binding share a name", "schema A = string\nlet A = 1\na: 1\n", `"A" names both a schema and a binding`},
		{"root schema twice", "schema $ = mapping\nschema { $ = mapping }\na: 1\n", `the document is given a schema twice`},
		{"nested root schema", "a:\n  schema $$ = mapping\n  b: 1\n", `"schema $$" can only be declared at the top of a document`},
		{"hidden schema field", "schema A =\n  b:: string\na: 1\n", "a schema has no hidden fields"},
		{"optional key type", "schema A = {(string):? integer}\na: 1\n", `only a field can be optional`},
		{"default of a merge", "schema A = {<<: B = {}}\na: 1\n", "only a field can have a default"},
		{"interpolated type", `schema A = "\(x)"` + "\na: 1\n", "a string written as a type cannot interpolate"},
		{"unclosed sequence type", "schema A = [string\na: 1\n", `expected "]" to close a sequence type`},
		{"junk after a type", "schema A = string string\na: 1\n", `unexpected "string" after a type`},
		{"three-part schema name", "a: !a.b.c 1\n", `a schema is named "Name", or "module.Name"`},
		{"tag without a value", "a: [!A]\n", `expected a value after "!A"`},
		{"reserved key", "let: 1\n", "reserved word"},
		{"let without a body", "a:\n  let x = 1\n", "must be followed by a value"},
		{"let without a name", "let = 1\na: 1\n", "expected the name of a binding"},
		{"let without an assignment", "let x 1\na: 1\n", `expected "=" after the name of a binding`},
		{"let naming a keyword", "let null = 1\na: 1\n", "cannot name a binding"},
		{"parameter list without an assignment", "let f(a) 1\nb: 1\n", `expected "=" after the name of a binding`},
		{"parameter naming a keyword", "let f(for) = 1\na: 1\n", `"for" is a keyword and cannot name a parameter`},
		{"parameter that is not a name", "let f(1) = 1\na: 1\n", "expected the name of a parameter"},
		{"duplicate parameter", "let f(a, a) = a\nb: 1\n", `parameter "a" is declared twice`},
		{"unseparated parameters", "let f(a b) = a\nc: 1\n", `expected "," or ")" in a parameter list`},
		{"unterminated parameter list", "let f(a = 1", "unterminated parameter list"},
		{"unseparated arguments", "let f(a) = a\nb: f(1 2)\n", `expected "," or ")" in an argument list`},
		{"unterminated argument list", "let f(a) = a\nb: f(1", "unterminated argument list"},
		{"positional argument after a named one", "let f(a, b) = a\nc: f(a = 1, 2)\n", "cannot follow a named one"},
		{"over-indented let", "let x = 1\n  let y = 2\na: x\n", "unexpected indentation"},
		{"empty let block", "let {}\na: 1\n", "at least one binding"},
		{"unterminated let block", "let {\n  x = 1\n", "unterminated"},
		{"unseparated bindings", "let { x = 1 y = 2 }\na: x\n", "between bindings"},
		{"lone question mark", "a: 1 ? 2\n", "explicit key indicators"},
		{"coalesce on its own line", "a: .b\n?? 1\n", "expected a mapping key"},
		{"numeric key", "1: a\n", "numeric keys must be quoted"},
		{"bad indentation", "a: 1\n  b: 2\n", "unexpected indentation"},
		{"stray token after value", "a: 1 2\n", "unexpected"},
		{"unterminated flow sequence", "a: [1, 2\n", "unterminated flow sequence"},
		{"unterminated flow mapping", "a: {b: 1\n", "unterminated flow mapping"},
		{"dots in postfix", "a: $.b..c\n", "may only begin a reference"},
		{"dangling dot", "a: $. b\n", "expected a field name"},
		{"unknown builtin variable", "a: $cwd\n", "unknown builtin variable"},
		{"self is only a dot", "a: $self.b\n", `unknown builtin variable "$self"`},
		{"empty interpolation", `a: "\()"`, "empty string interpolation"},
		{"junk in interpolation", `a: "\(.b .c)"`, "unexpected"},
		{"double bang is still a tag", "a: !!true\n", "tags are not supported"},
		{"bang is written not", "a: !true\n", `"!" is written "not"`},
		{"symbolic or in an interpolation", `a: "\(b || c)"`, `"||" is written "or"`},
		{"not as an operand", "a: 1 == not true\n", `a "not" cannot be used as an operand here`},
		{"not as a binding name", "let not = 1\na: 1\n", `"not" is a keyword and cannot name a binding`},
		{"if without then", "a: if true 1\n", `expected "then"`},
		{"if as an operand", "a: 1 == if true then 1 else 2\n", "wrap it in parentheses"},
		{"a keyword as a value", "a: then\n", `"then" is a keyword`},
		{"let named for", "let for = 1\na: 1\n", `"for" is a keyword and cannot name a binding`},
		{"comprehension without in", "a: [x for x of $$.l]\n", `expected "in"`},
		{"comprehension without a variable", "a: [x for 1 in $$.l]\n", "expected the name of the loop variable"},
		{"comprehension naming a keyword", "a: [x for in in $$.l]\n", "cannot name a loop variable"},
		{"comprehension after several items", "a: [1, 2 for x in $$.l]\n", `expected "," or "]"`},
		{"empty pattern", "a: [1 for [] in $$.l]\n", "a pattern needs at least one item"},
		{"pattern with a trailing comma", "a: [1 for [x,] in $$.l]\n", `expected a pattern after ","`},
		{"unseparated pattern", "a: [1 for [x y] in $$.l]\n", `expected "," or "]" in a pattern`},
		{"pattern with a value", "a: [1 for [x, 2] in $$.l]\n", "expected the name of a loop variable"},
		{"name bound twice in a loop", "a: [1 for x, [y, x] in $$.l]\n", `"x" is bound twice in one pattern`},
		{"pattern naming a keyword", "a: [1 for [x, if] in $$.l]\n", `"if" is a keyword and cannot name a loop variable`},
		{"name bound twice in a let", "let [a, a] = [1, 2]\nb: a\n", `"a" is bound twice in one pattern`},
		{"let pattern of wildcards", "let [_, [_]] = [1, [2]]\nb: 1\n", "has to bind at least one name"},
		{"let pattern with parameters", "let [a, b](x) = x\nc: a\n", `expected "=" after the name of a binding`},
		{"values of a pattern without brackets", "let a, b = 1, 2\nc: a\n", `the values go in brackets`},
		{"name bound twice without brackets", "let a, [b, a] = p\nc: a\n", `"a" is bound twice in one pattern`},
		{"parameter pattern of wildcards", "let f([_, _]) = 1\na: 1\n", "the pattern of a parameter has to bind at least one name"},
		{"name bound twice in a parameter pattern", "let f([a, a]) = a\nb: 1\n", `"a" is bound twice in one pattern`},
		{"parameter pattern repeating a parameter", "let f(a, [b, a]) = a\nc: 1\n", `parameter "a" is declared twice`},
		{"parameter repeating a pattern's name", "let f([a, b], b) = a\nc: 1\n", `parameter "b" is declared twice`},
		{"lambda pattern repeating a parameter", "a: ([x, y], x) => x\n", `parameter "x" is declared twice`},
		{"wildcard as a binding", "let _ = 1\na: 1\n", `"_" discards a value and cannot name a binding`},
		{"wildcard as a parameter", "let f(_) = 1\na: 1\n", `"_" discards a value and cannot name a parameter`},
		{"wildcard read as a value", "a: _\n", `"_" discards a value and cannot be read`},
		{"unterminated comprehension", "a: [x for x in $$.l\n", `expected "]" to close a comprehension`},
		{"unterminated mapping comprehension", "a: {(x): 1 for x in $$.l\n", `expected "}" to close a comprehension`},
		{"alias", "a: *x\n", "anchors and aliases are not supported"},
		{"minus without a space after it", "a: 1 -2\n", "a subtraction needs whitespace"},
		{"minus against a reference", "a: $.b -$.c\n", "a subtraction needs whitespace"},
		{"minus without a space before it", "a: 1- 2\n", "a subtraction needs whitespace"},
		{"minus in an argument list", "let f(a) = a\nb: f(1 -2)\n", "a subtraction needs whitespace"},
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
