package eval

import (
	"fmt"
	"strings"

	"github.com/daluz/yak/internal/token"
)

// Error is an evaluation failure tied to a source position. Cycles carry the
// chain of references that produced them.
type Error struct {
	Pos   token.Pos
	Msg   string
	Trace []token.Pos
	// TraceTitle introduces the trace; it defaults to "referenced from".
	TraceTitle string
}

func (e *Error) Error() string {
	var sb strings.Builder
	sb.WriteString(e.Pos.String())
	sb.WriteString(": ")
	sb.WriteString(e.Msg)
	if len(e.Trace) > 0 {
		title := e.TraceTitle
		if title == "" {
			title = "referenced from"
		}
		sb.WriteString("\n\t")
		sb.WriteString(title)
		sb.WriteString(":")
		for _, p := range e.Trace {
			sb.WriteString("\n\t  ")
			sb.WriteString(p.String())
		}
	}
	return sb.String()
}

func errorf(pos token.Pos, format string, args ...any) *Error {
	return &Error{Pos: pos, Msg: fmt.Sprintf(format, args...)}
}
