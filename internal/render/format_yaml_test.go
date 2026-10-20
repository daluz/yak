package render_test

import (
	"strings"
	"testing"

	"github.com/daluz/yak/internal/eval"
)

func TestRenderStringNeedingQuotes(t *testing.T) {
	got := renderDocs(t, object("v", eval.String("a: b")))
	if want := "v: 'a: b'\n"; got != want {
		t.Errorf("rendered %q, want %q", got, want)
	}
}

func TestRenderMultilineWithTrailingSpaceStaysQuoted(t *testing.T) {
	got := renderDocs(t, object("v", eval.String("one \ntwo")))
	if strings.Contains(got, "|") {
		t.Errorf("rendered %q as a block scalar, which would lose the trailing space", got)
	}
}

func TestRenderSequenceComments(t *testing.T) {
	arr := eval.NewArray([]*eval.Elem{eval.Item(eval.Int(1)), eval.Item(eval.Int(2))})
	arr.Items()[0].Head = []string{"# the first"}
	arr.Items()[0].Line = "# beside the first"

	got := renderDocs(t, object("list", arr))
	want := "list:\n  # the first\n  - 1 # beside the first\n  - 2\n"
	if got != want {
		t.Errorf("rendered:\n%s\nwant:\n%s", got, want)
	}
}

// A line or a foot comment written on a collection inside a sequence comes
// out attached to the item after it unless it is moved onto a scalar first.
func TestRenderCommentsOnCollectionItems(t *testing.T) {
	arr := eval.NewArray([]*eval.Elem{
		eval.Item(object("name", eval.String("queue"))),
		eval.Item(object("name", eval.String("cron"))),
	})
	arr.Items()[0].Line = "# beside the first"
	arr.Items()[1].Foot = []string{"# after the last"}

	got := renderDocs(t, object("workers", arr))
	want := "workers:\n  - name: queue # beside the first\n  - name: cron\n    # after the last\n"
	if got != want {
		t.Errorf("rendered:\n%s\nwant:\n%s", got, want)
	}
}
