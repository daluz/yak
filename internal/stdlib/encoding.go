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
		{Name: "encode", Params: append(params("value"), optional("indent", &ast.Int{})), Fn: encodeJSON},
		{Name: "decode", Params: valueParam, Fn: decodeWith(ctxfile.ValueJSON, "JSON")},
	}})
	module.Register(module.BuiltinModule{Path: "std.yaml", Builtins: []*eval.Builtin{
		{Name: "encode", Params: valueParam, Fn: encodeYAML},
		{Name: "decode", Params: valueParam, Fn: decodeWith(ctxfile.ValueYAML, "YAML")},
	}})
	module.Register(module.BuiltinModule{Path: "std.base64", Builtins: []*eval.Builtin{
		{Name: "encode", Params: valueParam, Fn: mapString("encode", func(s string) string {
			return base64.StdEncoding.EncodeToString([]byte(s))
		})},
		{Name: "decode", Params: valueParam, Fn: decodeBase64},
	}})
}

// encodeJSON writes compact JSON on one line, which is what an annotation or
// an environment variable wants, or JSON indented by indent spaces a level
// when indent is more than 0.
func encodeJSON(args []*eval.Thunk) (eval.Value, error) {
	indent, err := countArg("encode", args[1])
	if err != nil {
		return nil, err
	}
	format := render.FormatJSONL
	if indent > 0 {
		format = render.FormatJSON
	}
	out, err := encode(args[0], render.Options{Format: format, Indent: indent})
	if err != nil {
		return nil, err
	}
	return eval.String(strings.TrimSuffix(out, "\n")), nil
}

// encodeYAML writes block YAML ending in a newline, as a file would.
func encodeYAML(args []*eval.Thunk) (eval.Value, error) {
	out, err := encode(args[0], render.Options{Format: render.FormatYAML})
	if err != nil {
		return nil, err
	}
	return eval.String(out), nil
}

func encode(arg *eval.Thunk, opts render.Options) (string, error) {
	v, err := arg.Value()
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	opts.KeepNullDocuments = true
	if err := render.Documents(&buf, []eval.Value{v}, opts); err != nil {
		var e *eval.Error
		if errors.As(err, &e) {
			return "", err
		}
		return "", eval.Errorf(arg.Pos(), "%q cannot encode its value: %s", "encode", err.Error())
	}
	return buf.String(), nil
}

// decodeWith makes the decode of a format, which reads a string holding one
// document of it. format names the string in a diagnostic.
func decodeWith(read func(string, []byte) (eval.Value, error), format string) func([]*eval.Thunk) (eval.Value, error) {
	return func(args []*eval.Thunk) (eval.Value, error) {
		s, err := stringArg("decode", args[0])
		if err != nil {
			return nil, err
		}
		v, err := read(format, []byte(s))
		if err != nil {
			return nil, eval.Errorf(args[0].Pos(), "%q cannot decode its value: %s", "decode", err.Error())
		}
		return v, nil
	}
}

// decodeBase64 answers the text that a base64 string holds. Bytes that are not
// text are an error, since a yak string can hold nothing else.
func decodeBase64(args []*eval.Thunk) (eval.Value, error) {
	s, err := stringArg("decode", args[0])
	if err != nil {
		return nil, err
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, eval.Errorf(args[0].Pos(), "%q cannot decode its value: %s", "decode", err.Error())
	}
	if !utf8.Valid(b) {
		return nil, eval.Errorf(args[0].Pos(), "%q decoded bytes that are not UTF-8 text", "decode")
	}
	return eval.String(b), nil
}
