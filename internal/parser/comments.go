package parser

import (
	"strings"

	"github.com/daluz/yak/internal/token"
)

// noteMarker introduces a comment addressed to whoever reads the template
// rather than to whoever reads the rendered YAML.
const noteMarker = "##"

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
// left behind because it renders nothing.
func (c *commentSet) head(i int) []string {
	var out []string
	for n := range c.all {
		cm := &c.all[n]
		if c.claimed[n] || !cm.OwnLine || cm.Next > i {
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

// rendered reports the text a comment contributes to the output. A "## "
// comment contributes nothing: it is a note about the template.
func rendered(text string) (string, bool) {
	rest, marked := strings.CutPrefix(text, noteMarker)
	if !marked {
		return text, true
	}
	// A space has to follow the marker, so that "##note" and a "###" banner
	// stay comments. A bare "##" is the note with nothing written on it.
	if rest != "" && rest[0] != ' ' {
		return text, true
	}
	return "", false
}
