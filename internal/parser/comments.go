package parser

import (
	"strings"

	"github.com/daluz/yak/internal/token"
)

// localMarker introduces a comment addressed to whoever reads the template
// rather than to whoever reads the rendered YAML.
const localMarker = "#local"

// commentSet hands the comments of a file to the nodes they belong to.
//
// Each comment records the token that follows it, which places it exactly
// within the constructs around it: the comments above a node are the ones
// that point at its first token, and the comment trailing a node is the one
// that points into the middle of it. A comment is claimed at most once, and
// whatever is left over was written where nothing is rendered.
type commentSet struct {
	all     []token.Comment
	claimed []bool
}

func newCommentSet(comments []token.Comment) *commentSet {
	return &commentSet{all: comments, claimed: make([]bool, len(comments))}
}

// head claims the whole-line comments that have come due at the token at i:
// the ones written directly above it, and any that a construct in between
// left behind because it renders nothing. Comments at or before after are
// left alone, which is how a comment written above a binding stays available
// to whatever follows the binding.
func (c *commentSet) head(after, i int) []string {
	var out []string
	for n := range c.all {
		cm := &c.all[n]
		if c.claimed[n] || !cm.OwnLine || cm.Next <= after || cm.Next > i {
			continue
		}
		c.claimed[n] = true
		if text, ok := rendered(cm.Text); ok {
			out = append(out, text)
		}
	}
	return out
}

// line claims the comment trailing a node that spans the tokens from start
// to end. A comment trailing a line further down belongs to a node nested in
// this one and was claimed while that node was parsed, so what is left here
// is the node's own; the last of them wins if a flow collection left more
// than one behind.
func (c *commentSet) line(start, end int) string {
	out := ""
	for n := range c.all {
		cm := &c.all[n]
		if c.claimed[n] || cm.OwnLine || cm.Next <= start || cm.Next > end {
			continue
		}
		c.claimed[n] = true
		if text, ok := rendered(cm.Text); ok {
			out = text
		}
	}
	return out
}

// drop claims the comments written between the tokens from start to end
// without handing them to anyone, so that a comment written where nothing is
// rendered cannot drift onto a later node.
//
// A whole-line comment past the last of those tokens is left alone: it sits
// below the construct and so belongs to whatever comes next.
func (c *commentSet) drop(start, end int) {
	for n := range c.all {
		cm := &c.all[n]
		if c.claimed[n] || cm.Next <= start || cm.Next > end {
			continue
		}
		if cm.Next < end || !cm.OwnLine {
			c.claimed[n] = true
		}
	}
}

// rendered reports the text a comment contributes to the output. A "#local"
// comment contributes nothing: it is a note about the template.
func rendered(text string) (string, bool) {
	rest, marked := strings.CutPrefix(text, localMarker)
	if !marked {
		return text, true
	}
	// A word boundary is required, so that "#localhost" stays a comment.
	if rest != "" && isWordByte(rest[0]) {
		return text, true
	}
	return "", false
}

func isWordByte(c byte) bool {
	return c == '_' || c == '-' || (c >= '0' && c <= '9') ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
