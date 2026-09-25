package render

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Format selects the output encoding.
type Format int

// Supported output formats. The zero value is the default, so an empty
// Options renders YAML.
const (
	// FormatYAML is block YAML, restricted the same way yak source is.
	FormatYAML Format = iota
	// FormatKYAML is KYAML, the flow-style YAML dialect Kubernetes defines
	// in KEP-5295.
	FormatKYAML
	// FormatJSON is indented JSON.
	FormatJSON
	// FormatJSONC is indented JSON keeping comments.
	FormatJSONC
	// FormatJWCC is JSON with comments and commas: indented JSON keeping
	// comments and writing trailing commas. Also known as jsoncc and as
	// HuJSON, and a subset of JSON5.
	FormatJWCC
	// FormatJSONL is one compact JSON document per line.
	FormatJSONL
	// FormatTOML is TOML.
	FormatTOML
)

// formatNames gives each format the name it is written and reported with.
var formatNames = [...]string{
	FormatYAML:  "yaml",
	FormatKYAML: "kyaml",
	FormatJSON:  "json",
	FormatJSONC: "jsonc",
	FormatJWCC:  "jwcc",
	FormatJSONL: "jsonl",
	FormatTOML:  "toml",
}

// Formats lists every format in the order they are offered to the user.
var Formats = []Format{
	FormatYAML, FormatKYAML, FormatJSON, FormatJSONC, FormatJWCC,
	FormatJSONL, FormatTOML,
}

// formatAliases names every spelling accepted for a format, including the
// other names some of them go by and the file extensions they are written to.
var formatAliases = map[string]Format{
	"yaml":   FormatYAML,
	"yml":    FormatYAML,
	"kyaml":  FormatKYAML,
	"json":   FormatJSON,
	"jsonc":  FormatJSONC,
	"jwcc":   FormatJWCC,
	"jsoncc": FormatJWCC,
	"hujson": FormatJWCC,
	// Not all of JSON5, but everything yak writes for jwcc reads as JSON5.
	"json5":  FormatJWCC,
	"jsonl":  FormatJSONL,
	"ndjson": FormatJSONL,
	"toml":   FormatTOML,
}

// String returns the format's name.
func (f Format) String() string {
	if int(f) < 0 || int(f) >= len(formatNames) {
		return fmt.Sprintf("Format(%d)", int(f))
	}
	return formatNames[f]
}

// ParseFormat resolves a format name, accepting the other names some
// formats go by, such as jsoncc and hujson for jwcc.
func ParseFormat(name string) (Format, error) {
	if f, ok := formatAliases[strings.ToLower(strings.TrimSpace(name))]; ok {
		return f, nil
	}
	return 0, fmt.Errorf("unknown output format %q; want one of %s", name, FormatList())
}

// FormatForFile returns the format a filename's extension asks for.
func FormatForFile(path string) (Format, bool) {
	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	f, ok := formatAliases[strings.ToLower(ext)]
	return f, ok
}

// FormatList returns the format names for help text and diagnostics.
func FormatList() string {
	names := make([]string, len(Formats))
	for i, f := range Formats {
		names[i] = f.String()
	}
	return strings.Join(names, ", ")
}
