// Package ctxfile loads the context files that a template is rendered
// against and merges them into the single value exposed as $context.
package ctxfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/daluz/yak/internal/eval"
)

// ExtJSON selects the JSON parser for a context file.
const ExtJSON = ".json"

// Load reads and merges context files in order. Later files win: mappings are
// merged key by key, and every other value is replaced outright.
func Load(paths []string) (eval.Value, error) {
	merged := eval.Value(eval.NewObject())
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading context file: %w", err)
		}
		v, err := Decode(path, data)
		if err != nil {
			return nil, err
		}
		merged = Merge(merged, v)
	}
	return merged, nil
}

// Decode parses one context file. Context files are plain YAML or JSON, not
// yak, so anchors and unquoted strings are allowed in them. A .json name is
// read by the JSON parser, which accepts the whole of JSON including escapes
// such as \/ that YAML rejects; any other name is read as YAML, which accepts
// most JSON too.
func Decode(name string, data []byte) (eval.Value, error) {
	if strings.EqualFold(filepath.Ext(name), ExtJSON) {
		return decodeJSON(name, data)
	}
	return decodeYAML(name, data)
}

// decodeYAML reads a YAML stream. Every document must hold a mapping, and the
// documents are merged in order so a stream behaves like separate files.
func decodeYAML(name string, data []byte) (eval.Value, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	merged := eval.Value(eval.NewObject())
	seen := false
	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parsing context file %s: %w", name, err)
		}
		v, err := convert(&doc, name)
		if err != nil {
			return nil, err
		}
		if err := requireMapping(name, v); err != nil {
			return nil, err
		}
		merged = Merge(merged, v)
		seen = true
	}
	if !seen {
		return eval.NewObject(), nil
	}
	return merged, nil
}

// decodeJSON reads a JSON file. Like the YAML path it accepts more than one
// top-level value, merging them in order, and it keeps object keys in the
// order they appear so the rendered output follows the source.
func decodeJSON(name string, data []byte) (eval.Value, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	merged := eval.Value(eval.NewObject())
	seen := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, jsonError(name, data, err)
		}
		v, err := jsonValue(dec, tok, name, data)
		if err != nil {
			return nil, err
		}
		if err := requireMapping(name, v); err != nil {
			return nil, err
		}
		merged = Merge(merged, v)
		seen = true
	}
	if !seen {
		return eval.NewObject(), nil
	}
	return merged, nil
}

// jsonValue converts the value that starts at tok, reading the rest of it from
// dec. Tokens are used instead of unmarshalling into a map so that key order
// survives.
func jsonValue(dec *json.Decoder, tok json.Token, name string, data []byte) (eval.Value, error) {
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			return jsonObject(dec, name, data)
		case '[':
			return jsonArray(dec, name, data)
		}
		return nil, fmt.Errorf("parsing context file %s: unexpected %q", name, t)

	case nil:
		return eval.Null{}, nil
	case bool:
		return eval.Bool(t), nil
	case string:
		return eval.String(t), nil
	case json.Number:
		return jsonNumber(t, name, data, dec.InputOffset())

	default:
		return nil, fmt.Errorf("parsing context file %s: unexpected %v", name, tok)
	}
}

func jsonObject(dec *json.Decoder, name string, data []byte) (eval.Value, error) {
	obj := eval.NewObject()
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, jsonError(name, data, err)
		}
		if d, ok := tok.(json.Delim); ok && d == '}' {
			return obj, nil
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("parsing context file %s: object keys must be strings", name)
		}
		tok, err = dec.Token()
		if err != nil {
			return nil, jsonError(name, data, err)
		}
		val, err := jsonValue(dec, tok, name, data)
		if err != nil {
			return nil, err
		}
		obj.Set(key, false, eval.Done(val))
	}
}

func jsonArray(dec *json.Decoder, name string, data []byte) (eval.Value, error) {
	var items []*eval.Thunk
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, jsonError(name, data, err)
		}
		if d, ok := tok.(json.Delim); ok && d == ']' {
			return eval.NewArray(items), nil
		}
		v, err := jsonValue(dec, tok, name, data)
		if err != nil {
			return nil, err
		}
		items = append(items, eval.Done(v))
	}
}

// jsonNumber keeps whole numbers integral and falls back to a float for
// everything else, including values too large for an int64.
func jsonNumber(n json.Number, name string, data []byte, offset int64) (eval.Value, error) {
	if i, err := strconv.ParseInt(n.String(), 10, 64); err == nil {
		return eval.Int(i), nil
	}
	f, err := strconv.ParseFloat(n.String(), 64)
	if err != nil {
		line, col := position(data, offset)
		return nil, fmt.Errorf("%s:%d:%d: %s is out of range", name, line, col, n.String())
	}
	return eval.Float(f), nil
}

func requireMapping(name string, v eval.Value) error {
	if _, ok := v.(*eval.Object); ok {
		return nil
	}
	return fmt.Errorf("context file %s must contain a mapping at the top level, found %s", name, v.TypeName())
}

// jsonError reports a decoding failure, pointing at the offending byte when
// the decoder tells us where it was.
func jsonError(name string, data []byte, err error) error {
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) {
		line, col := position(data, syntax.Offset)
		return fmt.Errorf("%s:%d:%d: %s", name, line, col, syntax.Error())
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return fmt.Errorf("parsing context file %s: unexpected end of file", name)
	}
	return fmt.Errorf("parsing context file %s: %w", name, err)
}

// position turns a byte offset into the 1-based line and column used by the
// rest of the tool's diagnostics. Offsets point just past the byte the decoder
// was reading, so they are stepped back by one.
func position(data []byte, offset int64) (int, int) {
	if offset > int64(len(data)) {
		offset = int64(len(data))
	}
	if offset > 0 {
		offset--
	}
	line, col := 1, 1
	for _, b := range data[:offset] {
		if b == '\n' {
			line, col = line+1, 1
			continue
		}
		col++
	}
	return line, col
}

// Merge deep-merges over onto base. Mappings merge recursively; anything else
// replaces what came before.
func Merge(base, over eval.Value) eval.Value {
	b, okBase := base.(*eval.Object)
	o, okOver := over.(*eval.Object)
	if !okBase || !okOver {
		return over
	}
	out := eval.NewObject()
	for _, f := range b.Fields() {
		out.Set(f.Name, f.Hidden, f.Value)
	}
	for _, f := range o.Fields() {
		existing, ok := out.Lookup(f.Name)
		if !ok {
			out.Set(f.Name, f.Hidden, f.Value)
			continue
		}
		prev, err := existing.Value.Value()
		if err != nil {
			out.Set(f.Name, f.Hidden, f.Value)
			continue
		}
		next, err := f.Value.Value()
		if err != nil {
			out.Set(f.Name, f.Hidden, f.Value)
			continue
		}
		out.Set(f.Name, f.Hidden, eval.Done(Merge(prev, next)))
	}
	return out
}

func convert(n *yaml.Node, name string) (eval.Value, error) {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return eval.Null{}, nil
		}
		return convert(n.Content[0], name)

	case yaml.AliasNode:
		return convert(n.Alias, name)

	case yaml.MappingNode:
		obj := eval.NewObject()
		for i := 0; i+1 < len(n.Content); i += 2 {
			keyNode, valNode := n.Content[i], n.Content[i+1]
			var key string
			if err := keyNode.Decode(&key); err != nil {
				return nil, fmt.Errorf("%s:%d:%d: context mapping keys must be strings", name, keyNode.Line, keyNode.Column)
			}
			val, err := convert(valNode, name)
			if err != nil {
				return nil, err
			}
			obj.Set(key, false, eval.Done(val))
		}
		return obj, nil

	case yaml.SequenceNode:
		items := make([]*eval.Thunk, 0, len(n.Content))
		for _, c := range n.Content {
			v, err := convert(c, name)
			if err != nil {
				return nil, err
			}
			items = append(items, eval.Done(v))
		}
		return eval.NewArray(items), nil

	case yaml.ScalarNode:
		return convertScalar(n, name)

	default:
		return eval.Null{}, nil
	}
}

func convertScalar(n *yaml.Node, name string) (eval.Value, error) {
	var raw any
	if err := n.Decode(&raw); err != nil {
		return nil, fmt.Errorf("%s:%d:%d: %w", name, n.Line, n.Column, err)
	}
	switch v := raw.(type) {
	case nil:
		return eval.Null{}, nil
	case bool:
		return eval.Bool(v), nil
	case int:
		return eval.Int(v), nil
	case int64:
		return eval.Int(v), nil
	case uint64:
		return eval.Int(int64(v)), nil
	case float64:
		return eval.Float(v), nil
	case string:
		return eval.String(v), nil
	case time.Time:
		return eval.String(v.Format(time.RFC3339)), nil
	default:
		return eval.String(n.Value), nil
	}
}
