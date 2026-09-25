// Package render writes evaluated yak values as YAML.
package render

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/daluz/yak/internal/eval"
)

// Format selects the output encoding. Only YAML exists today; the enum keeps
// the seam open for JSON.
type Format int

// Supported output formats.
const (
	FormatYAML Format = iota
)

// Options controls how documents are written.
type Options struct {
	Format Format
	// Indent is the number of spaces per nesting level.
	Indent int
}

// DefaultOptions returns the standard rendering settings.
func DefaultOptions() Options {
	return Options{Format: FormatYAML, Indent: 2}
}

// Documents writes every evaluated document to w, separated by "---".
func Documents(w io.Writer, docs []eval.Value, opts Options) error {
	if opts.Indent <= 0 {
		opts.Indent = 2
	}
	if opts.Format != FormatYAML {
		return fmt.Errorf("unsupported output format")
	}

	// Resolve every document before writing anything, so that a failure in a
	// later document does not leave a partial stream behind.
	nodes := make([]*yaml.Node, 0, len(docs))
	for _, doc := range docs {
		if err := eval.Force(doc); err != nil {
			return err
		}
		node, err := toNode(doc)
		if err != nil {
			return err
		}
		nodes = append(nodes, node)
	}

	enc := yaml.NewEncoder(w)
	enc.SetIndent(opts.Indent)
	for _, node := range nodes {
		if err := enc.Encode(node); err != nil {
			return err
		}
	}
	return enc.Close()
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
		return scalar("!!float", formatFloat(float64(t))), nil

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
	if strings.Contains(s, "\n") && isBlockSafe(s) {
		n.Style = yaml.LiteralStyle
	}
	return n
}

// isBlockSafe reports whether a literal block scalar can round-trip the text.
// Trailing spaces and interior tabs at line starts defeat block scalars, so
// such strings fall back to quoted style.
func isBlockSafe(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		if strings.HasSuffix(line, " ") || strings.HasPrefix(line, "\t") {
			return false
		}
	}
	return true
}

func formatFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return ".nan"
	case math.IsInf(f, 1):
		return ".inf"
	case math.IsInf(f, -1):
		return "-.inf"
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	// A float that formats without a decimal point would read back as an
	// integer, so keep it unambiguous.
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}
