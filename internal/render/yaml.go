package render

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/daluz/yak/internal/eval"
)

// encodeYAML writes the documents as block YAML, separated by "---".
func encodeYAML(docs []eval.Value, opts Options) ([]byte, error) {
	nodes := make([]*yaml.Node, 0, len(docs))
	for _, doc := range docs {
		node, err := toNode(doc)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(opts.Indent)
	for _, node := range nodes {
		if err := enc.Encode(node); err != nil {
			return nil, err
		}
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func toNode(v eval.Value) (*yaml.Node, error) {
	switch t := v.(type) {
	case eval.Null:
		return scalar("!!null", "null"), nil

	case eval.Bool:
		if t {
			return scalar("!!bool", "true"), nil
		}
		return scalar("!!bool", "false"), nil

	case eval.Int:
		return scalar("!!int", strconv.FormatInt(int64(t), 10)), nil

	case eval.Float:
		s, err := formatFloat(float64(t), yamlFloats)
		if err != nil {
			return nil, err
		}
		return scalar("!!float", s), nil

	case eval.String:
		return stringNode(string(t)), nil

	case *eval.Object:
		node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, f := range t.Fields() {
			if f.Hidden {
				continue
			}
			inner, err := f.Value.Value()
			if err != nil {
				return nil, err
			}
			valNode, err := toNode(inner)
			if err != nil {
				return nil, err
			}
			// An entry's comments go on its key, which is where they
			// were written and where they come out again.
			keyNode := stringNode(f.Name)
			keyNode.HeadComment = joinComments(f.Head)
			keyNode.LineComment = f.Line
			keyNode.FootComment = joinComments(f.Foot)
			node.Content = append(node.Content, keyNode, valNode)
		}
		return node, nil

	case *eval.Array:
		node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, item := range t.Items() {
			inner, err := item.Value.Value()
			if err != nil {
				return nil, err
			}
			itemNode, err := toNode(inner)
			if err != nil {
				return nil, err
			}
			setItemComments(itemNode, item.Comments)
			node.Content = append(node.Content, itemNode)
		}
		return node, nil

	default:
		return nil, fmt.Errorf("cannot render a value of type %s", v.TypeName())
	}
}

func scalar(tag, value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value}
}

func joinComments(lines []string) string { return strings.Join(lines, "\n") }

// setItemComments attaches a sequence item's comments to its node.
//
// The head comment goes on the item itself, but a line or foot comment
// written on a collection comes out attached to the item after it, so those
// are moved onto the scalar that opens or closes the item instead.
func setItemComments(n *yaml.Node, c eval.Comments) {
	n.HeadComment = joinComments(c.Head)
	if c.Line != "" {
		if target := opening(n); target.LineComment == "" {
			target.LineComment = c.Line
		}
	}
	if foot := joinComments(c.Foot); foot != "" {
		if target := closing(n); target.FootComment == "" {
			target.FootComment = foot
		}
	}
}

// opening returns the scalar a node begins with: itself, or the first key or
// item of a collection.
func opening(n *yaml.Node) *yaml.Node {
	if len(n.Content) == 0 {
		return n
	}
	return opening(n.Content[0])
}

// closing returns the scalar a node ends with. A mapping ends at its last
// key rather than its last value, because a comment written after a value
// that holds a block would come out inside that block.
func closing(n *yaml.Node) *yaml.Node {
	switch n.Kind {
	case yaml.MappingNode:
		if len(n.Content) >= 2 {
			return closing(n.Content[len(n.Content)-2])
		}
	case yaml.SequenceNode:
		if len(n.Content) > 0 {
			return closing(n.Content[len(n.Content)-1])
		}
	}
	return n
}

// stringNode emits a string, choosing a literal block for multi-line text so
// that rendered output stays readable.
func stringNode(s string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
	if !strings.Contains(s, "\n") {
		return n
	}
	if isBlockSafe(s) {
		n.Style = yaml.LiteralStyle
		return n
	}
	// Left to itself the encoder reaches for a block scalar here too, and
	// writes one that cannot be read back.
	n.Style = yaml.DoubleQuotedStyle
	return n
}

// isBlockSafe reports whether a literal block scalar can round-trip the text.
// Trailing spaces and interior tabs at line starts defeat block scalars, and
// so does an empty first line, which leaves the block with no indentation to
// measure itself against.
func isBlockSafe(s string) bool {
	lines := strings.Split(s, "\n")
	if lines[0] == "" {
		return false
	}
	for _, line := range lines {
		if strings.HasSuffix(line, " ") || strings.HasPrefix(line, "\t") {
			return false
		}
	}
	return true
}
