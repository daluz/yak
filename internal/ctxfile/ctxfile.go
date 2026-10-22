// Package ctxfile loads the context files that a template is rendered
// against and merges them into the single value exposed as $context.
package ctxfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/daluz/yak/internal/eval"
)

// extJSON selects the JSON parser for a context file.
const extJSON = ".json"

// Load reads and merges context files in order. Later files win: mappings are
// merged key by key, and every other value is replaced outright.
func Load(paths []string) (eval.Value, error) {
	merged := eval.NewObject()
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading context file: %w", err)
		}
		o, err := Decode(path, data)
		if err != nil {
			return nil, err
		}
		if merged, err = eval.Merge(merged, o); err != nil {
			return nil, err
		}
	}
	return merged, nil
}

// Decode parses one context file. Context files are plain YAML or JSON, not
// yak, so anchors and unquoted strings are allowed in them. A .json name is
// read by the JSON parser, which accepts the whole of JSON including escapes
// such as \/ that YAML rejects; any other name is read as YAML, which accepts
// most JSON too. A file holds a mapping, or nothing, which reads as an empty
// one.
func Decode(name string, data []byte) (*eval.Object, error) {
	src := source{name: name, what: "context file " + name, data: data}
	if strings.EqualFold(filepath.Ext(name), extJSON) {
		return decodeJSON(src)
	}
	return decodeYAML(src)
}

// source is what a decoder reads. name prefixes a position within data, and
// what says what was being parsed when the whole of it is at fault.
type source struct {
	name string
	what string
	data []byte
}

// ValueYAML reads the one YAML document data holds, which may be a value of
// any type; an empty one is null. name stands for data in diagnostics.
func ValueYAML(name string, data []byte) (eval.Value, error) {
	src := source{name: name, what: name, data: data}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); errors.Is(err, io.EOF) {
		return eval.Null{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", src.what, err)
	}
	var next yaml.Node
	if err := dec.Decode(&next); err == nil {
		return nil, fmt.Errorf("parsing %s: it holds more than one document", src.what)
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parsing %s: %w", src.what, err)
	}
	return convert(&doc, src)
}

// ValueJSON reads the one JSON value data holds, which may be of any type.
// name stands for data in diagnostics.
func ValueJSON(name string, data []byte) (eval.Value, error) {
	src := source{name: name, what: name, data: data}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return nil, jsonError(src, err)
	}
	v, err := jsonValue(dec, tok, src)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parsing %s: it holds more than one value", src.what)
	}
	return v, nil
}

// decodeYAML reads a YAML stream. Every document must hold a mapping, and the
// documents are merged in order so a stream behaves like separate files.
func decodeYAML(src source) (*eval.Object, error) {
	dec := yaml.NewDecoder(bytes.NewReader(src.data))
	merged := eval.NewObject()
	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", src.what, err)
		}
		v, err := convert(&doc, src)
		if err != nil {
			return nil, err
		}
		if _, empty := v.(eval.Null); empty {
			continue
		}
		if merged, err = mergeMapping(src, merged, v); err != nil {
			return nil, err
		}
	}
	return merged, nil
}

// decodeJSON reads a JSON file. Like the YAML path it accepts more than one
// top-level value, merging them in order, and it keeps object keys in the
// order they appear so the rendered output follows the source.
func decodeJSON(src source) (*eval.Object, error) {
	dec := json.NewDecoder(bytes.NewReader(src.data))
	dec.UseNumber()
	merged := eval.NewObject()
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, jsonError(src, err)
		}
		v, err := jsonValue(dec, tok, src)
		if err != nil {
			return nil, err
		}
		if merged, err = mergeMapping(src, merged, v); err != nil {
			return nil, err
		}
	}
	return merged, nil
}

// jsonValue converts the value that starts at tok, reading the rest of it from
// dec. Tokens are used instead of unmarshalling into a map so that key order
// survives.
func jsonValue(dec *json.Decoder, tok json.Token, src source) (eval.Value, error) {
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			return jsonObject(dec, src)
		case '[':
			return jsonArray(dec, src)
		}
		return nil, fmt.Errorf("parsing %s: unexpected %q", src.what, t)

	case nil:
		return eval.Null{}, nil
	case bool:
		return eval.Bool(t), nil
	case string:
		return eval.String(t), nil
	case json.Number:
		return jsonNumber(t, src, dec.InputOffset())

	default:
		return nil, fmt.Errorf("parsing %s: unexpected %v", src.what, tok)
	}
}

func jsonObject(dec *json.Decoder, src source) (eval.Value, error) {
	obj := eval.NewObject()
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, jsonError(src, err)
		}
		if d, ok := tok.(json.Delim); ok && d == '}' {
			return obj, nil
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("parsing %s: object keys must be strings", src.what)
		}
		tok, err = dec.Token()
		if err != nil {
			return nil, jsonError(src, err)
		}
		val, err := jsonValue(dec, tok, src)
		if err != nil {
			return nil, err
		}
		obj.Set(key, false, eval.Done(val))
	}
}

func jsonArray(dec *json.Decoder, src source) (eval.Value, error) {
	var items []*eval.Elem
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, jsonError(src, err)
		}
		if d, ok := tok.(json.Delim); ok && d == ']' {
			return eval.NewArray(items), nil
		}
		v, err := jsonValue(dec, tok, src)
		if err != nil {
			return nil, err
		}
		items = append(items, eval.Item(v))
	}
}

// jsonNumber keeps whole numbers integral and falls back to a float for
// everything else, including values too large for an int64.
func jsonNumber(n json.Number, src source, offset int64) (eval.Value, error) {
	if i, err := strconv.ParseInt(n.String(), 10, 64); err == nil {
		return eval.Int(i), nil
	}
	f, err := strconv.ParseFloat(n.String(), 64)
	if err != nil {
		line, col := position(src.data, offset)
		return nil, fmt.Errorf("%s:%d:%d: %s is out of range", src.name, line, col, n.String())
	}
	return eval.Float(f), nil
}

// mergeMapping merges the top-level value v of src over merged, which every
// top-level value has to be a mapping for.
func mergeMapping(src source, merged *eval.Object, v eval.Value) (*eval.Object, error) {
	o, ok := v.(*eval.Object)
	if !ok {
		return nil, fmt.Errorf("%s must contain a mapping at the top level, found %s", src.what, v.TypeName())
	}
	return eval.Merge(merged, o)
}

// jsonError reports a decoding failure, pointing at the offending byte when
// the decoder tells us where it was.
func jsonError(src source, err error) error {
	if syntax, ok := errors.AsType[*json.SyntaxError](err); ok {
		line, col := position(src.data, syntax.Offset)
		return fmt.Errorf("%s:%d:%d: %s", src.name, line, col, syntax.Error())
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return fmt.Errorf("parsing %s: unexpected end of file", src.what)
	}
	return fmt.Errorf("parsing %s: %w", src.what, err)
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

func convert(n *yaml.Node, src source) (eval.Value, error) {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return eval.Null{}, nil
		}
		return convert(n.Content[0], src)

	case yaml.AliasNode:
		return convert(n.Alias, src)

	case yaml.MappingNode:
		obj := eval.NewObject()
		for i := 0; i+1 < len(n.Content); i += 2 {
			keyNode, valNode := n.Content[i], n.Content[i+1]
			var key string
			if err := keyNode.Decode(&key); err != nil {
				return nil, fmt.Errorf("%s:%d:%d: mapping keys must be strings", src.name, keyNode.Line, keyNode.Column)
			}
			val, err := convert(valNode, src)
			if err != nil {
				return nil, err
			}
			obj.Set(key, false, eval.Done(val))
		}
		return obj, nil

	case yaml.SequenceNode:
		items := make([]*eval.Elem, 0, len(n.Content))
		for _, c := range n.Content {
			v, err := convert(c, src)
			if err != nil {
				return nil, err
			}
			items = append(items, eval.Item(v))
		}
		return eval.NewArray(items), nil

	case yaml.ScalarNode:
		return convertScalar(n, src)

	default:
		return eval.Null{}, nil
	}
}

func convertScalar(n *yaml.Node, src source) (eval.Value, error) {
	var raw any
	if err := n.Decode(&raw); err != nil {
		return nil, fmt.Errorf("%s:%d:%d: %w", src.name, n.Line, n.Column, err)
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
		return eval.Float(v), nil
	case float64:
		if math.IsInf(v, 0) || math.IsNaN(v) {
			return nil, fmt.Errorf("%s:%d:%d: %s is not a number yak can hold", src.name, n.Line, n.Column, n.Value)
		}
		return eval.Float(v), nil
	case string:
		return eval.String(v), nil
	default:
		return eval.String(n.Value), nil
	}
}
