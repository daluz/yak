package render_test

import (
	"testing"

	"github.com/daluz/yak/internal/render"
)

func TestParseFormat(t *testing.T) {
	for _, tc := range []struct {
		name string
		want render.Format
	}{
		{"yaml", render.FormatYAML},
		{"YAML", render.FormatYAML},
		{"yml", render.FormatYAML},
		{"kyaml", render.FormatKYAML},
		{"json", render.FormatJSON},
		{"jsonc", render.FormatJSONC},
		{"jwcc", render.FormatJWCC},
		{"jsoncc", render.FormatJWCC},
		{"hujson", render.FormatJWCC},
		{"json5", render.FormatJWCC},
		{"jsonl", render.FormatJSONL},
		{"ndjson", render.FormatJSONL},
		{"toml", render.FormatTOML},
	} {
		got, err := render.ParseFormat(tc.name)
		if err != nil {
			t.Errorf("ParseFormat(%q) returned %v", tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseFormat(%q) = %s, want %s", tc.name, got, tc.want)
		}
	}

	if _, err := render.ParseFormat("xml"); err == nil {
		t.Error("ParseFormat(\"xml\") succeeded, want an error")
	}
}

// TestFormatNamesParse checks that every format can be named back, so that a
// diagnostic or a golden file suffix always round-trips.
func TestFormatNamesParse(t *testing.T) {
	for _, f := range render.Formats {
		got, err := render.ParseFormat(f.String())
		if err != nil {
			t.Errorf("ParseFormat(%q) returned %v", f, err)
			continue
		}
		if got != f {
			t.Errorf("ParseFormat(%q) = %s, want the format it was named for", f, got)
		}
	}
}

func TestFormatForFile(t *testing.T) {
	for _, tc := range []struct {
		path string
		want render.Format
		ok   bool
	}{
		{"out.yaml", render.FormatYAML, true},
		{"out.yml", render.FormatYAML, true},
		{"out.kyaml", render.FormatKYAML, true},
		{"out.json", render.FormatJSON, true},
		{"out.jsonc", render.FormatJSONC, true},
		{"out.jwcc", render.FormatJWCC, true},
		{"out.json5", render.FormatJWCC, true},
		{"out.toml", render.FormatTOML, true},
		{"dir/out.JSON", render.FormatJSON, true},
		{"out.txt", render.FormatYAML, false},
		{"out", render.FormatYAML, false},
	} {
		got, ok := render.FormatForFile(tc.path)
		if ok != tc.ok {
			t.Errorf("FormatForFile(%q) recognised = %v, want %v", tc.path, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("FormatForFile(%q) = %s, want %s", tc.path, got, tc.want)
		}
	}
}
