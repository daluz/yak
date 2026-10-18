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
// finds the packages that other one vendored beside it. It answers that
// vendor directory and the repository's directory in it.
func vendored(root, key string) (string, string, bool) {
	dir, err := filepath.Abs(root)
	if err != nil {
		return "", "", false
	}
	for {
		vendor := filepath.Join(dir, VendorDir)
		v := filepath.Join(vendor, filepath.FromSlash(key))
		if info, err := os.Stat(v); err == nil && info.IsDir() {
			return vendor, v, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", false
		}
		dir = parent
	}
}

// A Source is a remote package, written as
// git+SCHEME://HOST/REPOSITORY[//DIR][@REF] or
// oci://REGISTRY/REPOSITORY(:TAG|@DIGEST).
type Source struct {
	// OCI marks a package in an OCI registry rather than a git repository.
	OCI bool
	// Repo is the URL that git clones, or the registry and repository of an
	// OCI package.
	Repo string
	// Dir is the directory of the repository holding the package, or "" for
	// its root.
	Dir string
	// Ref is the branch, tag or commit to check out, or "" for the
	// repository's default branch; for an OCI package, the tag or the
	// digest.
	Ref string
	// Key names the repository beneath VendorDir: its host and path.
	Key string
}

// gitSchemes are the schemes a git source can be fetched over.
var gitSchemes = []string{"https", "ssh", "file"}

// ParseSource parses the target of a remote dependency.
func ParseSource(target string) (Source, error) {
	if rest, ok := strings.CutPrefix(target, "oci://"); ok {
		return parseOCI(target, rest)
	}
	rest, ok := strings.CutPrefix(target, "git+")
	if !ok {
		return Source{}, fmt.Errorf("%s: a remote package is written as git+https://, git+ssh://, git+file:// or oci://", target)
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

// parseOCI parses rest, which target writes after "oci://".
func parseOCI(target, rest string) (Source, error) {
	registry, repo, _ := strings.Cut(rest, "/")
	var ref string
	if r, digest, ok := strings.Cut(repo, "@"); ok {
		repo, ref = r, digest
	} else if i := strings.LastIndex(repo, ":"); i >= 0 {
		repo, ref = repo[:i], repo[i+1:]
	}
	if registry == "" || repo == "" || path.Clean(repo) != repo || strings.HasPrefix(repo, "..") {
		return Source{}, fmt.Errorf("%s: names no repository; write oci://REGISTRY/REPOSITORY:TAG", target)
	}
	if ref == "" {
		return Source{}, fmt.Errorf("%s: names no tag or digest; write oci://REGISTRY/REPOSITORY:TAG", target)
	}
	host, _, _ := strings.Cut(registry, ":")
	return Source{OCI: true, Repo: registry + "/" + repo, Ref: ref, Key: host + "/" + repo}, nil
}
