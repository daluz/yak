package lexer_test

import (
	"strings"
	"testing"

	"github.com/daluz/yak/internal/lexer"
	"github.com/daluz/yak/internal/token"
)

func lex(t *testing.T, src string) []token.Token {
	t.Helper()
	toks, err := lexer.Lex("test.yak", []byte(src))
	if err != nil {
		t.Fatalf("Lex(%q) returned error: %v", src, err)
	}
	return toks
}

// firstString lexes a source that is expected to start with a string token.
func firstString(t *testing.T, src string) token.Token {
	t.Helper()
	toks := lex(t, src)
	for _, tok := range toks {
		if tok.Kind == token.String {
			return tok
		}
	}
	t.Fatalf("Lex(%q) produced no string token", src)
	return token.Token{}
}

func TestQuotedStrings(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"double quoted", `"hello"`, "hello"},
		{"single quoted", `'hello'`, "hello"},
		{"empty", `""`, ""},
		{"escaped quote", `"say \"hi\""`, `say "hi"`},
		{"escaped backslash", `"a\\b"`, `a\b`},
		{"newline escape", `"a\nb"`, "a\nb"},
		{"tab escape", `"a\tb"`, "a\tb"},
		{"unicode escape", `"\u00e9"`, "é"},
		{"hex escape", `"\x41"`, "A"},
		{"single quote doubling", `'it''s'`, "it's"},
		{"raw double quoted", `r"a\nb"`, `a\nb`},
		{"raw keeps interpolation literal", `r"${.x}"`, "${.x}"},
		{"escaped dollar", `"\${.x}"`, "${.x}"},
		{"doubled dollar escape", `"$${.x}"`, "${.x}"},
		{"lone dollar", `"cost: $5"`, "cost: $5"},
		{"doubled dollar without brace", `"$$"`, "$$"},
		{"line folding", "\"a\nb\"", "a b"},
		{"blank line folding", "\"a\n\nb\"", "a\nb"},
		{"line continuation", "\"a\\\n  b\"", "ab"},
		{"no escapes in single quotes", `'a\nb'`, `a\nb`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := firstString(t, tc.src)
			if got.Chunks != nil {
				t.Fatalf("Lex(%q) produced interpolation chunks, want a plain literal", tc.src)
			}
			if got.Lit != tc.want {
				t.Errorf("Lex(%q) = %q, want %q", tc.src, got.Lit, tc.want)
			}
		})
	}
}

func TestInterpolationChunks(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{"only expression", `"${.a}"`, []string{"expr(.a)"}},
		{"prefix and suffix", `"a${.b}c"`, []string{`text(a)`, `expr(.b)`, `text(c)`}},
		{"two expressions", `"${.a}${.b}"`, []string{"expr(.a)", "expr(.b)"}},
		{"nested string in expression", `"${$$.m["k"]}"`, []string{`expr($$.m["k"])`}},
		{"brace inside expression", `"${ {a: 1} }"`, []string{"expr( {a: 1} )"}},
		{"escape then expression", `"$${x}${.y}"`, []string{"text(${x})", "expr(.y)"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tok := firstString(t, tc.src)
			var got []string
			for _, c := range tok.Chunks {
				kind := "text"
				if c.IsExpr {
					kind = "expr"
				}
				got = append(got, kind+"("+c.Text+")")
			}
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Errorf("Lex(%q) chunks = %v, want %v", tc.src, got, tc.want)
			}
		})
	}
}

func TestInterpolationPositions(t *testing.T) {
	tok := firstString(t, "key: \"a${.b}\"\n")
	if len(tok.Chunks) != 2 {
		t.Fatalf("got %d chunks, want 2", len(tok.Chunks))
	}
	expr := tok.Chunks[1]
	// Source columns:  k=1 ... "=6, a=7, $=8, {=9, .=10
	if expr.Pos.Line != 1 || expr.Pos.Col != 10 {
		t.Errorf("interpolation position = %d:%d, want 1:10", expr.Pos.Line, expr.Pos.Col)
	}
}

func TestBlockScalars(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"literal", "k: |\n  a\n  b\n", "a\nb\n"},
		{"literal strip", "k: |-\n  a\n  b\n", "a\nb"},
		{"literal keep", "k: |+\n  a\n\n", "a\n\n"},
		{"literal preserves indent", "k: |\n  a\n    b\n", "a\n  b\n"},
		{"folded", "k: >\n  a\n  b\n", "a b\n"},
		{"folded blank line", "k: >\n  a\n\n  b\n", "a\nb\n"},
		{"folded keeps more indented", "k: >\n  a\n   b\n", "a\n b\n"},
		{"folded strip", "k: >-\n  a\n  b\n", "a b"},
		{"explicit indent", "k: |2\n    a\n", "  a\n"},
		{"nested indentation", "a:\n  k: |\n    x\n", "x\n"},
		{"ends at dedent", "a:\n  k: |\n    x\nb: 1\n", "x\n"},
		{"raw literal", "k: r|\n  ${.a}\n", "${.a}\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := firstString(t, tc.src)
			if got.Chunks != nil {
				t.Fatalf("Lex(%q) produced chunks, want a plain literal", tc.src)
			}
			if got.Lit != tc.want {
				t.Errorf("Lex(%q) = %q, want %q", tc.src, got.Lit, tc.want)
			}
		})
	}
}

func TestBlockScalarInterpolation(t *testing.T) {
	tok := firstString(t, "k: |\n  hello ${.name}\n")
	if len(tok.Chunks) != 3 {
		t.Fatalf("got %d chunks, want 3: %+v", len(tok.Chunks), tok.Chunks)
	}
	if tok.Chunks[0].Text != "hello " || !tok.Chunks[1].IsExpr || tok.Chunks[1].Text != ".name" {
		t.Errorf("unexpected chunks: %+v", tok.Chunks)
	}
	if tok.Chunks[1].Pos.Line != 2 || tok.Chunks[1].Pos.Col != 11 {
		t.Errorf("interpolation position = %d:%d, want 2:11", tok.Chunks[1].Pos.Line, tok.Chunks[1].Pos.Col)
	}
}

func TestTokenKinds(t *testing.T) {
	toks := lex(t, "a-b: $.c[0]\n")
	want := []token.Kind{
		token.Ident, token.Colon, token.Dollar, token.Dots, token.Ident,
		token.LBracket, token.Int, token.RBracket, token.EOF,
	}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d: %v", len(toks), len(want), toks)
	}
	for i, k := range want {
		if toks[i].Kind != k {
			t.Errorf("token %d = %v, want %v", i, toks[i].Kind, k)
		}
	}
	if toks[0].Lit != "a-b" {
		t.Errorf("identifier = %q, want %q", toks[0].Lit, "a-b")
	}
}

func TestDotRuns(t *testing.T) {
	toks := lex(t, "x: ...name\n")
	if toks[2].Kind != token.Dots || toks[2].Lit != "..." {
		t.Fatalf("got %v %q, want a run of three dots", toks[2].Kind, toks[2].Lit)
	}
	if toks[3].Kind != token.Ident || toks[3].Lit != "name" {
		t.Errorf("got %v %q, want identifier name", toks[3].Kind, toks[3].Lit)
	}
}

func TestDocumentMarkers(t *testing.T) {
	toks := lex(t, "a: 1\n---\nb: 2\n...\n")
	var kinds []token.Kind
	for _, tok := range toks {
		if tok.Kind == token.DocStart || tok.Kind == token.DocEnd {
			kinds = append(kinds, tok.Kind)
		}
	}
	if len(kinds) != 2 || kinds[0] != token.DocStart || kinds[1] != token.DocEnd {
		t.Errorf("markers = %v, want [DocStart DocEnd]", kinds)
	}
}

func TestComments(t *testing.T) {
	toks := lex(t, "# leading\na: 1 # trailing\n")
	if len(toks) != 4 {
		t.Fatalf("got %d tokens, want 4: %v", len(toks), toks)
	}
}

func TestNumbers(t *testing.T) {
	tests := []struct {
		src  string
		kind token.Kind
		lit  string
	}{
		{"1", token.Int, "1"},
		{"-42", token.Int, "-42"},
		{"1.5", token.Float, "1.5"},
		{"1e3", token.Float, "1e3"},
		{"-2.5e-3", token.Float, "-2.5e-3"},
	}
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			toks := lex(t, "k: "+tc.src+"\n")
			got := toks[2]
			if got.Kind != tc.kind || got.Lit != tc.lit {
				t.Errorf("lexed %q as %v %q, want %v %q", tc.src, got.Kind, got.Lit, tc.kind, tc.lit)
			}
		})
	}
}

func TestLexErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"anchor", "a: &x 1\n", "anchors and aliases are not supported"},
		{"alias", "a: *x\n", "anchors and aliases are not supported"},
		{"tag", "a: !!str 1\n", "tags are not supported"},
		{"explicit key", "? a\n", "explicit key indicators"},
		{"directive", "%YAML 1.2\n", "directives"},
		{"unterminated string", `a: "oops`, "unterminated string literal"},
		{"unknown escape", `a: "\q"`, "unknown escape sequence"},
		{"tab indentation", "a:\n\tb: 1\n", "tabs may not be used for indentation"},
		{"unterminated interpolation", `a: "${.x`, "unterminated string interpolation"},
		{"unterminated nested string", `a: "${.x"`, "unterminated string literal inside an interpolation"},
		{"content after block header", "a: | junk\n  x\n", "unexpected content after block scalar header"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := lexer.Lex("test.yak", []byte(tc.src))
			if err == nil {
				t.Fatalf("Lex(%q) succeeded, want an error containing %q", tc.src, tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Lex(%q) error = %q, want it to contain %q", tc.src, err, tc.want)
			}
		})
	}
}
