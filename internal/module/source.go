package module

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// VendorDir is the directory beneath a package root that holds the remote
// packages it depends on, each under its Source's Key.
const VendorDir = "vendor"

// IsRemote reports whether a dependency's target names a remote package
// rather than a local path.
func IsRemote(target string) bool {
	return strings.Contains(target, "://")
}

// vendored finds the repository key names in the vendor directory of root
// or of a directory above it, which is where a package vendored by another
// finds the packages that other one vendored beside it.
func vendored(root, key string) (string, bool) {
	dir, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	for {
		v := filepath.Join(dir, VendorDir, filepath.FromSlash(key))
		if info, err := os.Stat(v); err == nil && info.IsDir() {
			return v, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// A Source is a remote package, written as
// git+SCHEME://HOST/REPOSITORY[//DIR][@REF].
type Source struct {
	// Repo is the URL that git clones.
	Repo string
	// Dir is the directory of the repository holding the package, or "" for
	// its root.
	Dir string
	// Ref is the branch, tag or commit to check out, or "" for the
	// repository's default branch.
	Ref string
	// Key names the repository beneath VendorDir: its host and path.
	Key string
}

// gitSchemes are the schemes a git source can be fetched over.
var gitSchemes = []string{"https", "ssh", "file"}

// ParseSource parses the target of a remote dependency.
func ParseSource(target string) (Source, error) {
	if strings.HasPrefix(target, "oci://") {
		return Source{}, fmt.Errorf("%s: OCI packages are not implemented yet", target)
	}
	rest, ok := strings.CutPrefix(target, "git+")
	if !ok {
		return Source{}, fmt.Errorf("%s: a remote package is written as git+https://, git+ssh:// or git+file://", target)
	}
	scheme, rest, _ := strings.Cut(rest, "://")
	known := false
	for _, s := range gitSchemes {
		known = known || s == scheme
	}
	if !known {
		return Source{}, fmt.Errorf("%s: git cannot fetch over %q; use one of %s", target, scheme, strings.Join(gitSchemes, ", "))
	}
	authority, repo, _ := strings.Cut(rest, "/")
	src := Source{}
	repo, src.Ref, ok = strings.Cut(repo, "@")
	if ok && src.Ref == "" {
		return Source{}, fmt.Errorf("%s: nothing follows %q; name a branch, tag or commit, or leave the %q out", target, "@", "@")
	}
	repo, src.Dir, ok = strings.Cut(repo, "//")
	if ok && (src.Dir == "" || path.Clean(src.Dir) != src.Dir || strings.HasPrefix(src.Dir, "..") || path.IsAbs(src.Dir)) {
		return Source{}, fmt.Errorf("%s: %q is not a directory inside the repository", target, src.Dir)
	}
	host := authority
	if _, after, ok := strings.Cut(host, "@"); ok {
		host = after
	}
	host, _, _ = strings.Cut(host, ":")
	key := path.Clean(path.Join(host, strings.TrimSuffix(repo, ".git")))
	if repo == "" || key == "." || strings.HasPrefix(key, "..") {
		return Source{}, fmt.Errorf("%s: names no repository", target)
	}
	src.Repo = scheme + "://" + authority + "/" + repo
	src.Key = key
	return src, nil
}
