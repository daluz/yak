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

func TestRenderDropsAStreamOfNullDocuments(t *testing.T) {
	if got := renderDocs(t, eval.Null{}); got != "" {
		t.Errorf("rendered %q for a stream of nothing but nulls, want nothing", got)
	}
}
