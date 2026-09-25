package engine_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	"github.com/daluz/yak/internal/engine"
	"github.com/daluz/yak/internal/render"
)

// TestFormatsAgree renders every fixture in every format and checks that
// they all describe the same documents.
//
// The golden files pin down how each format looks; this pins down what it
// means. An escape written wrongly, a fold that loses a newline or a TOML
// table nested under the wrong header all show up here as a difference from
// the YAML rendering, which the parser tests already trust.
func TestFormatsAgree(t *testing.T) {
	dir := testdataDir("template")
	templates, err := filepath.Glob(filepath.Join(dir, "*.yak"))
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range templates {
		name := strings.TrimSuffix(filepath.Base(path), engine.ExtTemplate)
		t.Run(name, func(t *testing.T) {
			contexts, err := filepath.Glob(filepath.Join(dir, name+".ctx*"))
			if err != nil {
				t.Fatal(err)
			}
			sort.Strings(contexts)

			render1 := func(f render.Format) ([]byte, error) {
				var buf bytes.Buffer
				err := engine.Template(engine.TemplateRequest{
					Path:         path,
					ContextPaths: contexts,
					Out:          &buf,
					Options:      render.Options{Format: f},
				})
				return buf.Bytes(), err
			}

			out, err := render1(render.FormatYAML)
			if err != nil {
				t.Fatalf("rendering as yaml: %v", err)
			}
			want := decode(t, render.FormatYAML, out)

			for _, f := range render.Formats {
				if f == render.FormatYAML {
					continue
				}
				t.Run(f.String(), func(t *testing.T) {
					out, err := render1(f)
					if err != nil {
						// A format that cannot hold this document at all
						// is the business of the golden files.
						t.Skipf("not representable as %s: %v", f, err)
					}
					got := decode(t, f, out)
					if !reflect.DeepEqual(got, want) {
						t.Errorf("%s describes different documents\n--- %[1]s ---\n%s\n--- got ---\n%#v\n--- want ---\n%#v", f, out, got, want)
					}
				})
			}
		})
	}
}

// requireValid reads a rendering back with an off-the-shelf parser for its
// format. Comments are written into the output, and one placed badly would
// change the shape of a document rather than just look wrong.
func requireValid(t *testing.T, f render.Format, out []byte) {
	t.Helper()
	decode(t, f, out)
}

// decode parses a rendering into plain Go values, normalised so that
// renderings of the same documents in different formats compare equal.
func decode(t *testing.T, f render.Format, out []byte) []any {
	t.Helper()
	docs, err := parseFormat(f, out)
	if err != nil {
		t.Fatalf("rendered output is not valid %s: %v\n%s", f, err, out)
	}
	for i, doc := range docs {
		docs[i] = canonical(doc)
	}
	return docs
}

func parseFormat(f render.Format, out []byte) ([]any, error) {
	switch f {
	case render.FormatYAML, render.FormatKYAML:
		var docs []any
		dec := yaml.NewDecoder(bytes.NewReader(out))
		for {
			var doc any
			err := dec.Decode(&doc)
			if errors.Is(err, io.EOF) {
				return docs, nil
			}
			if err != nil {
				return nil, err
			}
			docs = append(docs, doc)
		}

	case render.FormatJSON:
		return unmarshalJSON(out)

	case render.FormatJSONC:
		return unmarshalJSON(stripComments(out))

	case render.FormatJWCC:
		return unmarshalJSON(stripTrailingCommas(stripComments(out)))

	case render.FormatJSONL:
		var docs []any
		for _, line := range bytes.Split(bytes.TrimRight(out, "\n"), []byte("\n")) {
			parsed, err := unmarshalJSON(line)
			if err != nil {
				return nil, err
			}
			docs = append(docs, parsed...)
		}
		return docs, nil

	case render.FormatTOML:
		var doc map[string]any
		if err := toml.Unmarshal(out, &doc); err != nil {
			return nil, err
		}
		return []any{doc}, nil
	}
	return nil, errors.New("no parser for this format")
}

func unmarshalJSON(out []byte) ([]any, error) {
	var doc any
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, err
	}
	return []any{doc}, nil
}

// canonical rewrites decoded documents into a shape that does not depend on
// the format they came from. Only the numbers really differ: JSON has one
// number type, TOML and YAML tell integers from floats, and YAML hands back
// mappings keyed by any.
func canonical(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = canonical(val)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k.(string)] = canonical(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = canonical(val)
		}
		return out
	case []map[string]any:
		// What a TOML decoder hands back for an array of tables.
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = canonical(val)
		}
		return out
	case int:
		return number(float64(t))
	case int64:
		return number(float64(t))
	case float64:
		return number(t)
	default:
		return v
	}
}

// number spells every number the same way, since JSON cannot say whether 3
// was written as an integer.
func number(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }

// stripComments removes "//" comments so that a JSON parser can read a
// rendering that keeps them.
func stripComments(src []byte) []byte {
	out := make([]byte, 0, len(src))
	for i := 0; i < len(src); i++ {
		if c := src[i]; c == '"' {
			end := endOfString(src, i)
			out = append(out, src[i:end]...)
			i = end - 1
			continue
		}
		if src[i] == '/' && i+1 < len(src) && src[i+1] == '/' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
		}
		if i < len(src) {
			out = append(out, src[i])
		}
	}
	return out
}

// stripTrailingCommas removes the comma before a closing bracket. It runs
// after stripComments, so only whitespace can stand between the two.
func stripTrailingCommas(src []byte) []byte {
	out := make([]byte, 0, len(src))
	for i := 0; i < len(src); i++ {
		if src[i] == '"' {
			end := endOfString(src, i)
			out = append(out, src[i:end]...)
			i = end - 1
			continue
		}
		if src[i] == ',' {
			j := i + 1
			for j < len(src) && (src[j] == ' ' || src[j] == '\t' || src[j] == '\n' || src[j] == '\r') {
				j++
			}
			if j < len(src) && (src[j] == '}' || src[j] == ']') {
				continue
			}
		}
		out = append(out, src[i])
	}
	return out
}

// endOfString returns the index just past the string that starts at the
// quote at src[i].
func endOfString(src []byte, i int) int {
	for j := i + 1; j < len(src); j++ {
		switch src[j] {
		case '\\':
			j++
		case '"':
			return j + 1
		}
	}
	return len(src)
}
