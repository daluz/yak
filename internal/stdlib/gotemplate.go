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

// The gotemplate module renders Go's text/template, for plain text that a
// yak string would be awkward to write, such as a configuration file of
// another format.
func init() {
	module.Register(module.BuiltinModule{Path: "std.gotemplate", Builtins: []*eval.Builtin{
		builtin("render", append(params("template"), optional("data", &ast.Null{})), renderTemplate),
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
	"nindent": func(n int64, s string) string {
		return "\n" + indentLines(n, s)
	},
}

// indentLines puts n spaces before every line of s.
func indentLines(n int64, s string) string {
	pad := strings.Repeat(" ", int(n))
	return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
}

func renderTemplate(c call) (eval.Value, error) {
	text, err := c.str(0)
	if err != nil {
		return nil, err
	}
	v, err := c.value(1)
	if err != nil {
		return nil, err
	}
	data, err := goValue(v, c)
	if err != nil {
		return nil, err
	}
	t, err := template.New("template").Option("missingkey=error").Funcs(templateFuncs).Parse(text)
	if err != nil {
		return nil, templateError(c, err)
	}
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		return nil, templateError(c, err)
	}
	return eval.String(b.String()), nil
}

func templateError(c call, err error) error {
	return c.errorf(0, "cannot render its template: %s", strings.TrimPrefix(err.Error(), "template: "))
}

// goValue converts a value of the data argument into the Go data a template
// reads, leaving out what the output formats leave out.
func goValue(v eval.Value, c call) (any, error) {
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
			if items[i], err = goValue(iv, c); err != nil {
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
			if m[f.Name], err = goValue(fv, c); err != nil {
				return nil, err
			}
		}
		return m, nil
	default:
		return nil, c.errorf(1, "cannot pass a %s to a template", v.TypeName())
	}
}
