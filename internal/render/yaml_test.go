package render_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/render"
)

func object(pairs ...any) *eval.Object {
	o := eval.NewObject()
	for i := 0; i+1 < len(pairs); i += 2 {
		o.Set(pairs[i].(string), false, eval.Done(pairs[i+1].(eval.Value)))
	}
	return o
}

func renderDocs(t *testing.T, docs ...eval.Value) string {
	t.Helper()
	var buf bytes.Buffer
	if err := render.Documents(&buf, docs, render.DefaultOptions()); err != nil {
		t.Fatalf("Documents returned error: %v", err)
	}
	return buf.String()
}

func TestRenderScalars(t *testing.T) {
	tests := []struct {
		name string
		val  eval.Value
		want string
	}{
		{"null", eval.Null{}, "v: null\n"},
		{"true", eval.Bool(true), "v: true\n"},
		{"false", eval.Bool(false), "v: false\n"},
		{"integer", eval.Int(42), "v: 42\n"},
		{"negative integer", eval.Int(-7), "v: -7\n"},
		{"float", eval.Float(1.5), "v: 1.5\n"},
		{"whole float keeps a decimal point", eval.Float(2), "v: 2.0\n"},
		{"string", eval.String("plain"), "v: plain\n"},
		{"string that looks like a number", eval.String("42"), "v: \"42\"\n"},
		{"string that looks like a boolean", eval.String("true"), "v: \"true\"\n"},
		{"string that looks like null", eval.String("null"), "v: \"null\"\n"},
		{"empty string", eval.String(""), "v: \"\"\n"},
		{"string needing quotes", eval.String("a: b"), "v: 'a: b'\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := renderDocs(t, object("v", tc.val))
			if got != tc.want {
				t.Errorf("rendered %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRenderMultilineStringUsesLiteralBlock(t *testing.T) {
	got := renderDocs(t, object("v", eval.String("line one\nline two\n")))
	want := "v: |\n  line one\n  line two\n"
	if got != want {
		t.Errorf("rendered:\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderMultilineWithTrailingSpaceStaysQuoted(t *testing.T) {
	got := renderDocs(t, object("v", eval.String("one \ntwo")))
	if strings.Contains(got, "|") {
		t.Errorf("rendered %q as a block scalar, which would lose the trailing space", got)
	}
}

func TestRenderPreservesKeyOrder(t *testing.T) {
	got := renderDocs(t, object("z", eval.Int(1), "a", eval.Int(2), "m", eval.Int(3)))
	want := "z: 1\na: 2\nm: 3\n"
	if got != want {
		t.Errorf("rendered %q, want %q", got, want)
	}
}

func TestRenderSkipsHiddenFields(t *testing.T) {
	o := eval.NewObject()
	o.Set("shown", false, eval.Done(eval.Int(1)))
	o.Set("hidden", true, eval.Done(eval.Int(2)))
	got := renderDocs(t, o)
	want := "shown: 1\n"
	if got != want {
		t.Errorf("rendered %q, want %q", got, want)
	}
}

func TestRenderSequences(t *testing.T) {
	arr := eval.NewArray([]*eval.Thunk{
		eval.Done(eval.Int(1)),
		eval.Done(eval.String("two")),
	})
	got := renderDocs(t, object("list", arr))
	want := "list:\n  - 1\n  - two\n"
	if got != want {
		t.Errorf("rendered %q, want %q", got, want)
	}
}

func TestRenderEmptyCollections(t *testing.T) {
	got := renderDocs(t, object("m", eval.NewObject(), "s", eval.NewArray(nil)))
	want := "m: {}\ns: []\n"
	if got != want {
		t.Errorf("rendered %q, want %q", got, want)
	}
}

func TestRenderSeparatesDocuments(t *testing.T) {
	got := renderDocs(t, object("a", eval.Int(1)), object("b", eval.Int(2)))
	want := "a: 1\n---\nb: 2\n"
	if got != want {
		t.Errorf("rendered %q, want %q", got, want)
	}
}
