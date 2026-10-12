package render_test

import (
	"bytes"
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

func TestRenderDropsNullDocuments(t *testing.T) {
	got := renderDocs(t, eval.Null{}, object("a", eval.Int(1)), eval.Null{})
	want := "a: 1\n"
	if got != want {
		t.Errorf("rendered %q, want %q", got, want)
	}
	if got := renderDocs(t, eval.Null{}); got != "" {
		t.Errorf("rendered %q for a stream of nothing but nulls, want nothing", got)
	}
}

func TestRenderKeepsNullDocumentsOnRequest(t *testing.T) {
	opts := render.DefaultOptions()
	opts.KeepNullDocuments = true

	var buf bytes.Buffer
	if err := render.Documents(&buf, []eval.Value{eval.Null{}, object("a", eval.Int(1))}, opts); err != nil {
		t.Fatalf("Documents returned error: %v", err)
	}
	want := "null\n---\na: 1\n"
	if got := buf.String(); got != want {
		t.Errorf("rendered %q, want %q", got, want)
	}
}
