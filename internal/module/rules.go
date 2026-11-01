package module

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// RootDir is the key of a rule that covers every template of a package, and
// the value of one that sends what it covers to the root of the output.
const RootDir = "/"

// Rule is one entry of rules: a template, a directory ending in "/", or "/"
// for the whole package, and where what it covers goes.
type Rule struct {
	Key  string
	To   string
	Drop bool
	used bool
}

// NewRule checks a rule that sends what key covers to to, or leaves it out
// when drop is set. Either may begin with "/", which changes nothing but for
// "/" itself: "/deploy/" covers what "deploy/" does.
func NewRule(key, to string, drop bool) (*Rule, error) {
	key = trimRoot(key)
	if err := ruleKey(key); err != nil {
		return nil, err
	}
	r := &Rule{Key: key, Drop: drop}
	if drop {
		return r, nil
	}
	r.To = trimRoot(to)
	return r, ruleTarget(key, r.To)
}

func trimRoot(p string) string {
	if p == RootDir {
		return p
	}
	return strings.TrimPrefix(p, RootDir)
}

// ruleKey checks what a rule covers: "/", a directory beneath the package
// root ending in "/", or a template.
func ruleKey(key string) error {
	switch {
	case key == RootDir:
		return nil
	case key == "./":
		return fmt.Errorf("names no path; write %q for the whole package", RootDir)
	case !isLocalPath(strings.TrimSuffix(key, "/")):
		return errors.New("is not a path beneath the package root")
	case strings.HasSuffix(key, "/"):
		return nil
	case strings.HasSuffix(key, ExtLibrary):
		return errors.New("is a library, which can be imported but not rendered")
	case !strings.HasSuffix(key, Ext):
		return fmt.Errorf("names no template; a rule covers a %s template, or a directory written with a trailing /", Ext)
	}
	return nil
}

// ruleTarget checks where a rule sends what it covers: a file, or a
// directory ending in "/", beneath the output. A directory can only be sent
// to a directory.
func ruleTarget(key, to string) error {
	if to == "./" {
		return fmt.Errorf("sends it to %q; write %q for the root of the output", to, RootDir)
	}
	if to != RootDir && !isLocalPath(strings.TrimSuffix(to, "/")) {
		return fmt.Errorf("sends it to %q, which is not a path beneath the output", to)
	}
	if strings.HasSuffix(key, "/") && !strings.HasSuffix(to, "/") {
		return errors.New("names a directory, so it must be sent to one, written with a trailing /")
	}
	return nil
}

// isLocalPath reports whether p is a clean slash path beneath the directory
// it is relative to, and not that directory itself.
func isLocalPath(p string) bool {
	return p != "." && path.Clean(p) == p && filepath.IsLocal(filepath.FromSlash(p))
}

// Used reports whether the rule has covered a template that Place placed.
func (r *Rule) Used() bool { return r.used }

// covers reports whether the rule covers the template at rel, a slash path
// relative to the package root.
func (r *Rule) covers(rel string) bool {
	if strings.HasSuffix(r.Key, "/") {
		return strings.HasPrefix(rel, r.prefix())
	}
	return rel == r.Key
}

// prefix is the part of a template's path the rule covers.
func (r *Rule) prefix() string {
	if r.Key == RootDir {
		return ""
	}
	return r.Key
}

// Target is where one template's output goes.
type Target struct {
	// Drop leaves the template out.
	Drop bool
	// Path is the output file, a slash path relative to the output, when
	// File is set; otherwise the directory the output goes beneath, at Rest.
	Path string
	File bool
	// Rest is the template's path beneath Path, which its output is named
	// after as a directory names it.
	Rest string
}

// Place answers where the template at rel, a slash path relative to its
// package root, goes: the longest of rules that covers it decides. It
// answers false when none does.
func Place(rules []*Rule, rel string) (Target, bool) {
	var best *Rule
	for _, r := range rules {
		if r.covers(rel) && (best == nil || len(r.prefix()) > len(best.prefix())) {
			best = r
		}
	}
	if best == nil {
		return Target{}, false
	}
	best.used = true
	switch {
	case best.Drop:
		return Target{Drop: true}, true
	case !strings.HasSuffix(best.To, "/"):
		return Target{Path: best.To, File: true}, true
	}
	rest := strings.TrimPrefix(rel, best.prefix())
	if !strings.HasSuffix(best.Key, "/") {
		rest = path.Base(rel)
	}
	return Target{Path: strings.TrimSuffix(best.To, "/"), Rest: rest}, true
}
