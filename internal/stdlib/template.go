package stdlib

import (
	"fmt"
	"strconv"
	"strings"
	"text/template"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/module"
)

// The template module renders Go's text/template, for plain text that a
// yak string would be awkward to write, such as a configuration file of
// another format.
func init() {
	module.Register(module.BuiltinModule{Path: "std.template", Builtins: []*eval.Builtin{
		{Name: "render", Params: append(params("template"), optional("data", &ast.Null{})), Fn: renderTemplate},
	}})
}

// templateFuncs are the functions a template may call beside text/template's
// own, named and behaving as Helm's do.
var templateFuncs = template.FuncMap{
	"quote": func(v any) string {
		if v == nil {
			return `""`
		}
		return strconv.Quote(fmt.Sprint(v))
	},
	"indent": indentLines,
	"nindent": func(n int, s string) string {
		return "\n" + indentLines(n, s)
	},
}

// indentLines puts n spaces before every line of s.
func indentLines(n int, s string) string {
	pad := strings.Repeat(" ", n)
	return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
}

func renderTemplate(args []*eval.Thunk) (eval.Value, error) {
	text, err := stringArg("render", args[0])
	if err != nil {
		return nil, err
	}
	v, err := args[1].Value()
	if err != nil {
		return nil, err
	}
	data, err := goValue(v, args[1])
	if err != nil {
		return nil, err
	}
	t, err := template.New("template").Option("missingkey=error").Funcs(templateFuncs).Parse(text)
	if err != nil {
		return nil, templateError(args[0], err)
	}
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		return nil, templateError(args[0], err)
	}
	return eval.String(b.String()), nil
}

func templateError(arg *eval.Thunk, err error) error {
	return eval.Errorf(arg.Pos(), "%q cannot render its template: %s", "render",
		strings.TrimPrefix(err.Error(), "template: "))
}

// goValue converts a value into the Go data a template reads, leaving out
// what the output formats leave out. arg is the argument it came from, which
// a complaint points at.
func goValue(v eval.Value, arg *eval.Thunk) (any, error) {
	switch t := v.(type) {
	case eval.Null:
		return nil, nil
	case eval.Bool:
		return bool(t), nil
	case eval.Int:
		return int64(t), nil
	case eval.Float:
		return float64(t), nil
	case eval.String:
		return string(t), nil
	case *eval.Array:
		items := make([]any, t.Len())
		for i, item := range t.Items() {
			iv, err := item.Value.Value()
			if err != nil {
				return nil, err
			}
			if items[i], err = goValue(iv, arg); err != nil {
				return nil, err
			}
		}
		return items, nil
	case *eval.Object:
		m := make(map[string]any, t.Len())
		for _, f := range t.Fields() {
			omitted, err := f.Omitted()
			if err != nil {
				return nil, err
			}
			if omitted {
				continue
			}
			fv, err := f.Value.Value()
			if err != nil {
				return nil, err
			}
			if m[f.Name], err = goValue(fv, arg); err != nil {
				return nil, err
			}
		}
		return m, nil
	default:
		return nil, eval.Errorf(arg.Pos(), "%q cannot pass a %s to a template", "render", v.TypeName())
	}
}
