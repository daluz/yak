package eval_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/daluz/yak/internal/ctxfile"
	"github.com/daluz/yak/internal/engine"
	"github.com/daluz/yak/internal/render"
)

// renderSource evaluates a template against an optional YAML context and
// returns the rendered document.
func renderSource(t *testing.T, src, contextYAML string) (string, error) {
	t.Helper()
	context, err := ctxfile.Decode("context.yaml", []byte(contextYAML))
	if err != nil {
		t.Fatalf("decoding context: %v", err)
	}
	var buf bytes.Buffer
	err = engine.Render("test.yak", []byte(src), context, &buf, render.DefaultOptions())
	return buf.String(), err
}

func mustRender(t *testing.T, src, contextYAML string) string {
	t.Helper()
	out, err := renderSource(t, src, contextYAML)
	if err != nil {
		t.Fatalf("rendering %q: %v", src, err)
	}
	return out
}

func TestReferences(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		context string
		want    string
	}{
		{
			name: "sibling via self",
			src:  "name: \"web\"\nalias: .name\n",
			want: "name: web\nalias: web\n",
		},
		{
			name: "forward reference",
			src:  "alias: .name\nname: \"web\"\n",
			want: "alias: web\nname: web\n",
		},
		{
			name: "parent via double dot",
			src:  "name: \"web\"\nchild:\n  parent: ..name\n",
			want: "name: web\nchild:\n  parent: web\n",
		},
		{
			name: "grandparent via triple dot",
			src:  "name: \"web\"\na:\n  b:\n    c: ...name\n",
			want: "name: web\na:\n  b:\n    c: web\n",
		},
		{
			name: "parent skips sequences",
			src:  "name: \"web\"\nitems:\n  - label: ..name\n",
			want: "name: web\nitems:\n  - label: web\n",
		},
		{
			name: "root reference",
			src:  "name: \"web\"\na:\n  b:\n    c: $.name\n",
			want: "name: web\na:\n  b:\n    c: web\n",
		},
		{
			name:    "context long form",
			src:     "name: $context.app\n",
			context: "app: shop\n",
			want:    "name: shop\n",
		},
		{
			name:    "context shorthand",
			src:     "name: $$.app\n",
			context: "app: shop\n",
			want:    "name: shop\n",
		},
		{
			name:    "context nested and typed",
			src:     "replicas: $$.deploy.replicas\ndebug: $$.deploy.debug\n",
			context: "deploy:\n  replicas: 4\n  debug: true\n",
			want:    "replicas: 4\ndebug: true\n",
		},
		{
			name: "self refers to whole mapping",
			src:  "a:\n  x: 1\nb: $.a.x\n",
			want: "a:\n  x: 1\nb: 1\n",
		},
		{
			name: "index into sequence",
			src:  "list:\n  - \"a\"\n  - \"b\"\nfirst: $.list[0]\nlast: $.list[-1]\n",
			want: "list:\n  - a\n  - b\nfirst: a\nlast: b\n",
		},
		{
			name:    "index into mapping with string",
			src:     "v: $$[\"key with space\"]\n",
			context: "\"key with space\": ok\n",
			want:    "v: ok\n",
		},
		{
			name: "reference to a mapping copies it",
			src:  "base:\n  a: 1\ncopy: $.base\n",
			want: "base:\n  a: 1\ncopy:\n  a: 1\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := mustRender(t, tc.src, tc.context); got != tc.want {
				t.Errorf("rendered:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}

func TestInterpolation(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		context string
		want    string
	}{
		{"string", `a: "x"` + "\n" + `b: "${.a}y"` + "\n", "", "a: x\nb: xy\n"},
		{"integer", "n: 3\ns: \"n=${.n}\"\n", "", "n: 3\ns: n=3\n"},
		{"boolean", "b: true\ns: \"b=${.b}\"\n", "", "b: true\ns: b=true\n"},
		{"float", "f: 1.5\ns: \"f=${.f}\"\n", "", "f: 1.5\ns: f=1.5\n"},
		{"null", "n: null\ns: \"n=${.n}\"\n", "", "n: null\ns: n=null\n"},
		{"context", `s: "${$$.a}-${$$.b}"` + "\n", "a: one\nb: two\n", "s: one-two\n"},
		{"in block scalar", "name: \"web\"\ntext: |\n  hello ${.name}\n", "", "name: web\ntext: |\n  hello web\n"},
		{"in computed key", "k: \"dyn\"\n[\"${.k}-key\"]: 1\n", "", "k: dyn\ndyn-key: 1\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := mustRender(t, tc.src, tc.context); got != tc.want {
				t.Errorf("rendered:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}

func TestHiddenFields(t *testing.T) {
	got := mustRender(t, "secret:: \"s3cret\"\nvisible: \"${.secret}!\"\n", "")
	want := "visible: s3cret!\n"
	if got != want {
		t.Errorf("rendered %q, want %q", got, want)
	}
}

func TestHiddenMappingHidesSubtree(t *testing.T) {
	got := mustRender(t, "defaults::\n  port: 80\nservice:\n  port: $.defaults.port\n", "")
	want := "service:\n  port: 80\n"
	if got != want {
		t.Errorf("rendered %q, want %q", got, want)
	}
}

func TestComputedKeys(t *testing.T) {
	got := mustRender(t, "[$$.key]: \"v\"\n", "key: dynamic\n")
	want := "dynamic: v\n"
	if got != want {
		t.Errorf("rendered %q, want %q", got, want)
	}
}

func TestComputedKeyCanReadLiteralSibling(t *testing.T) {
	got := mustRender(t, "prefix: \"app\"\n[.prefix]: 1\n", "")
	want := "prefix: app\napp: 1\n"
	if got != want {
		t.Errorf("rendered %q, want %q", got, want)
	}
}

func TestMultipleDocuments(t *testing.T) {
	got := mustRender(t, "a: 1\nr: $.a\n---\nb: 2\nr: $.b\n", "")
	want := "a: 1\nr: 1\n---\nb: 2\nr: 2\n"
	if got != want {
		t.Errorf("rendered:\n%s\nwant:\n%s", got, want)
	}
}

func TestRootIsPerDocument(t *testing.T) {
	_, err := renderSource(t, "a: 1\n---\nb: $.a\n", "")
	if err == nil {
		t.Fatal("expected $ in the second document not to see the first document's fields")
	}
	if !strings.Contains(err.Error(), `no field "a"`) {
		t.Errorf("error = %q, want a missing field error", err)
	}
}

func TestEvalErrors(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		context string
		want    string
	}{
		{"missing field", "a: .missing\n", "", `no field "missing"`},
		{"lists available fields", "a: 1\nb: .missing\n", "", `available fields: "a", "b"`},
		{"missing context field", "a: $$.nope\n", "", `no field "nope"`},
		{"self past root", "a: ..b\n", "", "reaches past the outermost mapping"},
		{"direct cycle", "a: .a\n", "", "circular reference"},
		{"indirect cycle", "a: .b\nb: .a\n", "", "circular reference"},
		{"interpolation cycle", `a: "${.b}"` + "\n" + `b: "${.a}"` + "\n", "", "circular reference"},
		{"field of scalar", "a: 1\nb: .a.c\n", "", "cannot read field"},
		{"index out of range", "a:\n  - 1\nb: $.a[5]\n", "", "out of range"},
		{"index with wrong type", "a:\n  - 1\nb: $.a[\"x\"]\n", "", "index must be an integer"},
		{"interpolate a mapping", "a:\n  b: 1\ns: \"${.a}\"\n", "", "cannot interpolate a mapping"},
		{"unknown identifier", "a: hello\n", "", `unknown identifier "hello"`},
		{"duplicate key", "a: 1\na: 2\n", "", `duplicate key "a"`},
		{"duplicate computed key", "a: 1\n[\"a\"]: 2\n", "", `duplicate key "a"`},
		{"self referential mapping", "a: .\n", "", "contains itself"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := renderSource(t, tc.src, tc.context)
			if err == nil {
				t.Fatalf("rendering %q succeeded, want an error containing %q", tc.src, tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestCycleErrorShowsTrace(t *testing.T) {
	_, err := renderSource(t, "a: .b\nb: .a\n", "")
	if err == nil {
		t.Fatal("expected a cycle error")
	}
	if !strings.Contains(err.Error(), "the cycle passes through") {
		t.Errorf("error = %q, want it to include the cycle trace", err)
	}
	if !strings.HasSuffix(err.Error(), "test.yak:1:4") {
		t.Errorf("error = %q, want the trace to end where it started", err)
	}
}

func TestErrorsCarryPositions(t *testing.T) {
	_, err := renderSource(t, "a: 1\nb: .missing\n", "")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.HasPrefix(err.Error(), "test.yak:2:4:") {
		t.Errorf("error = %q, want it to start with test.yak:2:4:", err)
	}
}
