package render

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/daluz/yak/internal/eval"
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

// formatSpec is everything the package knows about one format.
type formatSpec struct {
	// name is what the format is written and reported as.
	name string
	// aliases are the other spellings accepted for the format, including
	// the other names it goes by and the file extensions it is written to.
	aliases []string
	// single marks a format that holds exactly one document.
	single bool
	// floats names the non-finite floats. The zero value refuses them, as
	// JSON has no way to write them.
	floats floatSpelling
	encode func(docs []eval.Value, opts Options, floats floatSpelling) ([]byte, error)
}

// formatSpecs is indexed by Format, in the order formats are offered to the
// user. Each spec is defined beside its encoder.
var formatSpecs = [...]formatSpec{
	FormatYAML:  yamlSpec,
	FormatKYAML: kyamlSpec,
	FormatJSON:  jsonSpec,
	FormatJSONC: jsoncSpec,
	FormatJWCC:  jwccSpec,
	FormatJSONL: jsonlSpec,
	FormatTOML:  tomlSpec,
}

// Formats lists every format in the order they are offered to the user.
var Formats = func() []Format {
	fs := make([]Format, len(formatSpecs))
	for i := range formatSpecs {
		fs[i] = Format(i)
	}
	return fs
}()

// formatAliases maps every accepted spelling, names included, to its format.
var formatAliases = func() map[string]Format {
	m := make(map[string]Format)
	for i, s := range formatSpecs {
		m[s.name] = Format(i)
		for _, a := range s.aliases {
			m[a] = Format(i)
		}
	}
	return m
}()

// spec returns the format's spec, or false for a value no constant names.
func (f Format) spec() (formatSpec, bool) {
	if int(f) < 0 || int(f) >= len(formatSpecs) {
		return formatSpec{}, false
	}
	return formatSpecs[f], true
}

// String returns the format's name.
func (f Format) String() string {
	s, ok := f.spec()
	if !ok {
		return fmt.Sprintf("Format(%d)", int(f))
	}
	return s.name
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
