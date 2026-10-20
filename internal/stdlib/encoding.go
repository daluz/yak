package stdlib

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/ctxfile"
	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/module"
	"github.com/daluz/yak/internal/render"
)

// The json and yaml modules write a value as the output format of the same
// name would, and read one back. The base64 module encodes strings.
func init() {
	module.Register(module.BuiltinModule{Path: "std.json", Builtins: []*eval.Builtin{
		builtin("encode", append(params("value"), optional("indent", &ast.Int{})), encodeJSON),
		builtin("decode", valueParam, decodeWith(ctxfile.ValueJSON, "JSON")),
	}})
	module.Register(module.BuiltinModule{Path: "std.yaml", Builtins: []*eval.Builtin{
		builtin("encode", valueParam, encodeYAML),
		builtin("decode", valueParam, decodeWith(ctxfile.ValueYAML, "YAML")),
	}})
	module.Register(module.BuiltinModule{Path: "std.base64", Builtins: []*eval.Builtin{
		builtin("encode", valueParam, mapString(func(s string) string {
			return base64.StdEncoding.EncodeToString([]byte(s))
		})),
		builtin("decode", valueParam, decodeBase64),
	}})
}

// encodeJSON writes compact JSON on one line, which is what an annotation or
// an environment variable wants, or JSON indented by indent spaces a level
// when indent is more than 0.
func encodeJSON(c call) (eval.Value, error) {
	indent, err := c.count(1)
	if err != nil {
		return nil, err
	}
	format := render.FormatJSONL
	if indent > 0 {
		format = render.FormatJSON
	}
	out, err := encode(c, render.Options{Format: format, Indent: indent})
	if err != nil {
		return nil, err
	}
	return eval.String(strings.TrimSuffix(out, "\n")), nil
}

// encodeYAML writes block YAML ending in a newline, as a file would.
func encodeYAML(c call) (eval.Value, error) {
	out, err := encode(c, render.Options{Format: render.FormatYAML})
	if err != nil {
		return nil, err
	}
	return eval.String(out), nil
}

// renderValue writes a value exactly as the output format named by its
// second argument would write it as a file, or as YAML when that is null.
func renderValue(c call) (eval.Value, error) {
	name, given, err := c.optionalStr(1)
	if err != nil {
		return nil, err
	}
	format := render.FormatYAML
	if given {
		if format, err = render.ParseFormat(name); err != nil {
			return nil, c.errorf(1, "needs one of the formats %s, found %q", render.FormatList(), name)
		}
	}
	out, err := encode(c, render.Options{Format: format})
	if err != nil {
		return nil, err
	}
	return eval.String(out), nil
}

// encode writes the value of the first argument in the format opts asks for.
func encode(c call, opts render.Options) (string, error) {
	v, err := c.value(0)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	opts.KeepNullDocuments = true
	if err := render.Documents(&buf, []eval.Value{v}, opts); err != nil {
		if _, ok := errors.AsType[*eval.Error](err); ok {
			return "", err
		}
		return "", c.errorf(0, "cannot encode its value: %s", err.Error())
	}
	return buf.String(), nil
}

// decodeWith makes the decode of a format, which reads a string holding one
// document of it. format names the string in a diagnostic.
func decodeWith(read func(string, []byte) (eval.Value, error), format string) func(call) (eval.Value, error) {
	return func(c call) (eval.Value, error) {
		s, err := c.str(0)
		if err != nil {
			return nil, err
		}
		v, err := read(format, []byte(s))
		if err != nil {
			return nil, c.errorf(0, "cannot decode its value: %s", err.Error())
		}
		return v, nil
	}
}

// decodeBase64 answers the text that a base64 string holds. Bytes that are not
// text are an error, since a yak string can hold nothing else.
func decodeBase64(c call) (eval.Value, error) {
	s, err := c.str(0)
	if err != nil {
		return nil, err
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, c.errorf(0, "cannot decode its value: %s", err.Error())
	}
	if !utf8.Valid(b) {
		return nil, c.errorf(0, "decoded bytes that are not UTF-8 text")
	}
	return eval.String(b), nil
}
