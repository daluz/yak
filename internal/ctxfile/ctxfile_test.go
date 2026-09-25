package ctxfile_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daluz/yak/internal/ctxfile"
	"github.com/daluz/yak/internal/eval"
)

func decode(t *testing.T, src string) *eval.Object {
	t.Helper()
	return decodeAs(t, "context.yaml", src)
}

// decodeJSON decodes through the JSON parser, which the .json name selects.
func decodeJSON(t *testing.T, src string) *eval.Object {
	t.Helper()
	return decodeAs(t, "context.json", src)
}

func decodeAs(t *testing.T, name, src string) *eval.Object {
	t.Helper()
	v, err := ctxfile.Decode(name, []byte(src))
	if err != nil {
		t.Fatalf("Decode(%q) returned error: %v", src, err)
	}
	obj, ok := v.(*eval.Object)
	if !ok {
		t.Fatalf("Decode(%q) returned a %s, want a mapping", src, v.TypeName())
	}
	return obj
}

// lookup walks a dotted path through the decoded context.
func lookup(t *testing.T, obj *eval.Object, path string) eval.Value {
	t.Helper()
	var current eval.Value = obj
	for _, part := range strings.Split(path, ".") {
		o, ok := current.(*eval.Object)
		if !ok {
			t.Fatalf("path %q: %s is not a mapping", path, current.TypeName())
		}
		f, ok := o.Lookup(part)
		if !ok {
			t.Fatalf("path %q: no field %q", path, part)
		}
		v, err := f.Value.Value()
		if err != nil {
			t.Fatalf("path %q: %v", path, err)
		}
		current = v
	}
	return current
}

func TestDecodeScalarTypes(t *testing.T) {
	obj := decode(t, "s: text\nq: \"quoted\"\ni: 42\nf: 1.5\nb: true\nn: null\nyes: yes\n")

	tests := []struct {
		path string
		want eval.Value
	}{
		{"s", eval.String("text")},
		{"q", eval.String("quoted")},
		{"i", eval.Int(42)},
		{"f", eval.Float(1.5)},
		{"b", eval.Bool(true)},
		{"n", eval.Null{}},
		// The YAML 1.2 core schema treats "yes" as a string, which matches
		// yak's own rule that only true and false are booleans.
		{"yes", eval.String("yes")},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			if got := lookup(t, obj, tc.path); got != tc.want {
				t.Errorf("%s = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestDecodePreservesKeyOrder(t *testing.T) {
	obj := decode(t, "z: 1\na: 2\nm: 3\n")
	got := strings.Join(obj.Names(), ",")
	if got != "z,a,m" {
		t.Errorf("key order = %q, want %q", got, "z,a,m")
	}
}

func TestDecodeNested(t *testing.T) {
	obj := decode(t, "a:\n  b:\n    c: deep\nlist:\n  - one\n  - two\n")
	if got := lookup(t, obj, "a.b.c"); got != eval.String("deep") {
		t.Errorf("a.b.c = %v, want deep", got)
	}
	arr, ok := lookup(t, obj, "list").(*eval.Array)
	if !ok {
		t.Fatal("list is not a sequence")
	}
	if arr.Len() != 2 {
		t.Errorf("list has %d items, want 2", arr.Len())
	}
}

// A YAML file may hold JSON, since YAML is very nearly a superset of it.
func TestDecodeJSONInYAMLFile(t *testing.T) {
	obj := decode(t, `{"a": {"b": 1}, "c": [1, 2]}`)
	if got := lookup(t, obj, "a.b"); got != eval.Int(1) {
		t.Errorf("a.b = %v, want 1", got)
	}
}

func TestDecodeJSONScalarTypes(t *testing.T) {
	obj := decodeJSON(t, `{"s": "text", "i": 42, "f": 1.5, "e": 1e3, "b": true, "n": null}`)

	tests := []struct {
		path string
		want eval.Value
	}{
		{"s", eval.String("text")},
		{"i", eval.Int(42)},
		{"f", eval.Float(1.5)},
		{"e", eval.Float(1000)},
		{"b", eval.Bool(true)},
		{"n", eval.Null{}},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			if got := lookup(t, obj, tc.path); got != tc.want {
				t.Errorf("%s = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestDecodeJSONNested(t *testing.T) {
	obj := decodeJSON(t, "{\n\t\"a\": {\"b\": {\"c\": \"deep\"}},\n\t\"list\": [\"one\", \"two\"]\n}\n")
	if got := lookup(t, obj, "a.b.c"); got != eval.String("deep") {
		t.Errorf("a.b.c = %v, want deep", got)
	}
	arr, ok := lookup(t, obj, "list").(*eval.Array)
	if !ok {
		t.Fatal("list is not a sequence")
	}
	if arr.Len() != 2 {
		t.Errorf("list has %d items, want 2", arr.Len())
	}
}

func TestDecodeJSONPreservesKeyOrder(t *testing.T) {
	obj := decodeJSON(t, `{"z": 1, "a": 2, "m": 3}`)
	if got := strings.Join(obj.Names(), ","); got != "z,a,m" {
		t.Errorf("key order = %q, want %q", got, "z,a,m")
	}
}

// The escaped slash is valid JSON but not valid YAML, so it only decodes on
// the JSON path.
func TestDecodeJSONEscapedSlash(t *testing.T) {
	obj := decodeJSON(t, `{"path": "\/usr\/bin"}`)
	if got := lookup(t, obj, "path"); got != eval.String("/usr/bin") {
		t.Errorf("path = %v, want /usr/bin", got)
	}
	if _, err := ctxfile.Decode("context.yaml", []byte(`{"path": "\/usr"}`)); err == nil {
		t.Error("YAML accepted an escaped slash, the JSON path is no longer needed")
	}
}

// Numbers that do not fit an int64 become floats rather than wrapping around.
func TestDecodeJSONLargeNumber(t *testing.T) {
	obj := decodeJSON(t, `{"big": 12345678901234567890}`)
	got, ok := lookup(t, obj, "big").(eval.Float)
	if !ok {
		t.Fatalf("big = %v, want a float", lookup(t, obj, "big"))
	}
	if float64(got) != 12345678901234567890.0 {
		t.Errorf("big = %v, want 12345678901234567890", got)
	}
}

// Concatenated top-level objects merge like successive YAML documents do.
func TestDecodeJSONMergesTopLevelValues(t *testing.T) {
	obj := decodeJSON(t, `{"a": 1, "b": 1} {"b": 2}`)
	if got := lookup(t, obj, "a"); got != eval.Int(1) {
		t.Errorf("a = %v, want 1", got)
	}
	if got := lookup(t, obj, "b"); got != eval.Int(2) {
		t.Errorf("b = %v, want 2 (from the later value)", got)
	}
}

func TestDecodeJSONEmpty(t *testing.T) {
	obj := decodeJSON(t, "")
	if obj.Len() != 0 {
		t.Errorf("empty file decoded to %d fields, want 0", obj.Len())
	}
}

func TestDecodeJSONRejectsNonMapping(t *testing.T) {
	_, err := ctxfile.Decode("context.json", []byte("[1, 2]\n"))
	if err == nil || !strings.Contains(err.Error(), "must contain a mapping") {
		t.Errorf("error = %v, want a top-level mapping error", err)
	}
}

func TestDecodeJSONReportsSyntaxPosition(t *testing.T) {
	_, err := ctxfile.Decode("context.json", []byte("{\n  \"a\": 1,\n  \"b\": oops\n}\n"))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.HasPrefix(err.Error(), "context.json:3:8:") {
		t.Errorf("error = %q, want it to start with context.json:3:8:", err)
	}
}

func TestDecodeJSONReportsTruncatedFile(t *testing.T) {
	_, err := ctxfile.Decode("context.json", []byte(`{"a": [1, 2`))
	if err == nil || !strings.Contains(err.Error(), "unexpected end of file") {
		t.Errorf("error = %v, want an unexpected end of file", err)
	}
}

func TestDecodeAnchors(t *testing.T) {
	obj := decode(t, "base: &b\n  a: 1\nuse: *b\n")
	if got := lookup(t, obj, "use.a"); got != eval.Int(1) {
		t.Errorf("use.a = %v, want 1", got)
	}
}

func TestDecodeEmpty(t *testing.T) {
	obj := decode(t, "")
	if obj.Len() != 0 {
		t.Errorf("empty file decoded to %d fields, want 0", obj.Len())
	}
}

func TestDecodeRejectsNonMapping(t *testing.T) {
	_, err := ctxfile.Decode("context.yaml", []byte("- 1\n- 2\n"))
	if err == nil || !strings.Contains(err.Error(), "must contain a mapping") {
		t.Errorf("error = %v, want a top-level mapping error", err)
	}
}

func TestMergeIsDeepAndLastWins(t *testing.T) {
	base := decode(t, "a:\n  x: 1\n  y: 2\nkeep: base\nlist:\n  - 1\n")
	over := decode(t, "a:\n  y: 20\n  z: 30\nlist:\n  - 9\n  - 9\n")
	merged, ok := ctxfile.Merge(base, over).(*eval.Object)
	if !ok {
		t.Fatal("merge did not produce a mapping")
	}

	if got := lookup(t, merged, "a.x"); got != eval.Int(1) {
		t.Errorf("a.x = %v, want 1 (kept from the base)", got)
	}
	if got := lookup(t, merged, "a.y"); got != eval.Int(20) {
		t.Errorf("a.y = %v, want 20 (overridden)", got)
	}
	if got := lookup(t, merged, "a.z"); got != eval.Int(30) {
		t.Errorf("a.z = %v, want 30 (added)", got)
	}
	if got := lookup(t, merged, "keep"); got != eval.String("base") {
		t.Errorf("keep = %v, want base", got)
	}
	arr := lookup(t, merged, "list").(*eval.Array)
	if arr.Len() != 2 {
		t.Errorf("sequences should be replaced, not merged; got %d items, want 2", arr.Len())
	}
}

func TestMergeKeepsBaseOrdering(t *testing.T) {
	base := decode(t, "a: 1\nb: 2\n")
	over := decode(t, "b: 20\nc: 30\n")
	merged := ctxfile.Merge(base, over).(*eval.Object)
	if got := strings.Join(merged.Names(), ","); got != "a,b,c" {
		t.Errorf("key order = %q, want %q", got, "a,b,c")
	}
}

func TestLoadMergesFilesInOrder(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.yaml")
	second := filepath.Join(dir, "second.yaml")
	if err := os.WriteFile(first, []byte("a: 1\nb: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("b: 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	v, err := ctxfile.Load([]string{first, second})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	obj := v.(*eval.Object)
	if got := lookup(t, obj, "a"); got != eval.Int(1) {
		t.Errorf("a = %v, want 1", got)
	}
	if got := lookup(t, obj, "b"); got != eval.Int(2) {
		t.Errorf("b = %v, want 2 (from the later file)", got)
	}
}

func TestLoadMixesYAMLAndJSON(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "first.yaml")
	jsonPath := filepath.Join(dir, "second.json")
	if err := os.WriteFile(yamlPath, []byte("a: 1\nb: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jsonPath, []byte(`{"b": 2, "c": 3}`), 0o600); err != nil {
		t.Fatal(err)
	}

	v, err := ctxfile.Load([]string{yamlPath, jsonPath})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	obj := v.(*eval.Object)
	if got := lookup(t, obj, "a"); got != eval.Int(1) {
		t.Errorf("a = %v, want 1 (from the YAML file)", got)
	}
	if got := lookup(t, obj, "b"); got != eval.Int(2) {
		t.Errorf("b = %v, want 2 (from the JSON file)", got)
	}
	if got := lookup(t, obj, "c"); got != eval.Int(3) {
		t.Errorf("c = %v, want 3 (from the JSON file)", got)
	}
}

func TestLoadWithoutFilesIsEmpty(t *testing.T) {
	v, err := ctxfile.Load(nil)
	if err != nil {
		t.Fatalf("Load(nil) returned error: %v", err)
	}
	obj, ok := v.(*eval.Object)
	if !ok || obj.Len() != 0 {
		t.Errorf("Load(nil) = %v, want an empty mapping", v)
	}
}

func TestLoadReportsMissingFile(t *testing.T) {
	_, err := ctxfile.Load([]string{filepath.Join(t.TempDir(), "absent.yaml")})
	if err == nil || !strings.Contains(err.Error(), "reading context file") {
		t.Errorf("error = %v, want a read failure", err)
	}
}
