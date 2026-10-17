package deps_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daluz/yak/internal/deps"
	"github.com/daluz/yak/internal/module"
)

// write writes files beneath dir.
func write(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, src := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	args = append([]string{"-c", "user.name=yak", "-c", "user.email=yak@example.com",
		"-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", "-c", "init.defaultBranch=main"}, args...)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// repo creates a git repository holding files and answers its directory.
func repo(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	git(t, dir, "init", "--quiet")
	commit(t, dir, files)
	return dir
}

// commit commits files to the repository at dir and answers the commit.
func commit(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	write(t, dir, files)
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "--quiet", "-m", "change")
	return git(t, dir, "rev-parse", "HEAD")
}

func source(dir, rest string) string {
	return "git+file://" + filepath.ToSlash(dir) + rest
}

func key(dir string) string {
	return strings.TrimPrefix(filepath.ToSlash(dir), "/")
}

// project writes a package named app with the given yak.pkg and reads it.
func project(t *testing.T, pkgfile string) *module.Package {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, map[string]string{module.PkgFile: pkgfile})
	return readPackage(t, dir)
}

func readPackage(t *testing.T, dir string) *module.Package {
	t.Helper()
	pkg, err := module.FindPackage(dir)
	if err != nil || pkg == nil {
		t.Fatalf("FindPackage(%s) = %v, %v", dir, pkg, err)
	}
	return pkg
}

func read(t *testing.T, path string) string {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(src)
}

func TestSyncVendorsAndPins(t *testing.T) {
	lib := repo(t, map[string]string{"yak.pkg": "package:\n  name: lib\n", "labels.yakl": "team:: \"a\"\n"})
	git(t, lib, "tag", "v1")
	v1 := git(t, lib, "rev-parse", "HEAD")
	pkg := project(t, "package:\n  name: app\ndependencies:\n  lib: "+source(lib, "@v1")+"\n")

	if err := deps.Sync(pkg, deps.Options{}); err != nil {
		t.Fatal(err)
	}
	got, err := module.Resolve(filepath.Join(pkg.Root(), "app.yak"), []string{"lib", "labels"})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(pkg.Root(), "vendor", filepath.FromSlash(key(lib)), "labels.yakl"); got != want {
		t.Errorf("Resolve(lib.labels) = %s, want %s", got, want)
	}
	lock := read(t, filepath.Join(pkg.Root(), deps.LockFile))
	for _, want := range []string{key(lib) + ":", "ref: v1", "commit: " + v1, "hash: sha256:"} {
		if !strings.Contains(lock, want) {
			t.Errorf("yak.lock does not hold %q:\n%s", want, lock)
		}
	}
	if got := read(t, filepath.Join(pkg.Root(), "vendor", ".gitignore")); got != "*\n" {
		t.Errorf("vendor/.gitignore = %q, want everything ignored", got)
	}
	if _, err := os.Stat(filepath.Join(pkg.Root(), "vendor", filepath.FromSlash(key(lib)), ".git")); err == nil {
		t.Error("the vendored package holds .git, want it left out")
	}
}

func TestSyncHonoursTheLock(t *testing.T) {
	lib := repo(t, map[string]string{"yak.pkg": "package:\n  name: lib\n", "x.yak": "v: 1\n"})
	first := git(t, lib, "rev-parse", "HEAD")
	pkg := project(t, "package:\n  name: app\ndependencies:\n  lib: "+source(lib, "@main")+"\n")
	vendored := filepath.Join(pkg.Root(), "vendor", filepath.FromSlash(key(lib)), "x.yak")
	lockPath := filepath.Join(pkg.Root(), deps.LockFile)

	if err := deps.Sync(pkg, deps.Options{}); err != nil {
		t.Fatal(err)
	}
	second := commit(t, lib, map[string]string{"x.yak": "v: 2\n"})

	// A vendored copy that was changed or removed comes back as pinned.
	write(t, filepath.Dir(vendored), map[string]string{"x.yak": "tampered\n"})
	if err := deps.Sync(pkg, deps.Options{}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, vendored); got != "v: 1\n" {
		t.Errorf("after sync, x.yak = %q, want the pinned commit's", got)
	}
	if lock := read(t, lockPath); !strings.Contains(lock, first) {
		t.Errorf("yak.lock moved off %s:\n%s", first, lock)
	}

	if err := deps.Sync(pkg, deps.Options{Update: []string{"lib"}}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, vendored); got != "v: 2\n" {
		t.Errorf("after sync --update, x.yak = %q, want the branch's new commit", got)
	}
	if lock := read(t, lockPath); !strings.Contains(lock, second) {
		t.Errorf("yak.lock does not pin %s:\n%s", second, lock)
	}
}

func TestSyncRefusesALockThatDoesNotMatch(t *testing.T) {
	lib := repo(t, map[string]string{"yak.pkg": "package:\n  name: lib\n"})
	pkg := project(t, "package:\n  name: app\ndependencies:\n  lib: "+source(lib, "")+"\n")
	if err := deps.Sync(pkg, deps.Options{}); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(pkg.Root(), deps.LockFile)
	lock := read(t, lockPath)
	i := strings.Index(lock, "hash: ")
	j := i + strings.Index(lock[i:], "\n")
	write(t, pkg.Root(), map[string]string{deps.LockFile: lock[:i] + "hash: sha256:00" + lock[j:]})

	err := deps.Sync(pkg, deps.Options{})
	if err == nil || !strings.Contains(err.Error(), "does not hold what yak.lock pinned") {
		t.Errorf("error = %v, want the content to be refused", err)
	}
}

func TestSyncVendorsDependenciesOfDependencies(t *testing.T) {
	util := repo(t, map[string]string{"yak.pkg": "package:\n  name: util\n", "y.yakl": "v:: 1\n"})
	mono := repo(t, map[string]string{
		"pkgs/lib/yak.pkg": "package:\n  name: lib\ndependencies:\n  util: " + source(util, "") + "\n",
		"pkgs/lib/x.yakl":  "import util.y\nv:: y.v\n",
	})
	pkg := project(t, "package:\n  name: app\ndependencies:\n  lib: "+source(mono, "//pkgs/lib")+"\n")

	if err := deps.Sync(pkg, deps.Options{}); err != nil {
		t.Fatal(err)
	}
	x, err := module.Resolve(filepath.Join(pkg.Root(), "app.yak"), []string{"lib", "x"})
	if err != nil {
		t.Fatal(err)
	}
	y, err := module.Resolve(x, []string{"util", "y"})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(pkg.Root(), "vendor", filepath.FromSlash(key(util)), "y.yakl"); y != want {
		t.Errorf("Resolve(util.y) from lib = %s, want %s", y, want)
	}
}

func TestSyncRefusesTwoRefsOfOneRepository(t *testing.T) {
	util := repo(t, map[string]string{"yak.pkg": "package:\n  name: util\n"})
	git(t, util, "tag", "v1")
	commit(t, util, map[string]string{"more.yak": "a: 1\n"})
	git(t, util, "tag", "v2")
	lib := repo(t, map[string]string{"yak.pkg": "package:\n  name: lib\ndependencies:\n  util: " + source(util, "@v2") + "\n"})
	pkg := project(t, "package:\n  name: app\ndependencies:\n  lib: "+source(lib, "")+"\n  util: "+source(util, "@v1")+"\n")

	err := deps.Sync(pkg, deps.Options{})
	if err == nil || !strings.Contains(err.Error(), "depend on one ref of a repository") {
		t.Errorf("error = %v, want the two refs refused", err)
	}
}

func TestAddAndRemove(t *testing.T) {
	lib := repo(t, map[string]string{"yak.pkg": "package:\n  name: lib\n"})
	pkg := project(t, "# The app.\npackage:\n  name: app # its name\n")
	write(t, pkg.Root(), map[string]string{
		"../shared/yak.pkg": "package:\n  name: shared\n",
		"../one.yakl":       "",
	})

	if err := deps.Add(pkg, source(lib, ""), "", deps.Options{}); err != nil {
		t.Fatal(err)
	}
	pkg = readPackage(t, pkg.Root())
	if err := deps.Add(pkg, filepath.Join(pkg.Root(), "..", "shared"), "", deps.Options{}); err != nil {
		t.Fatal(err)
	}
	pkg = readPackage(t, pkg.Root())
	if err := deps.Add(pkg, filepath.Join(pkg.Root(), "..", "one.yakl"), "first", deps.Options{}); err != nil {
		t.Fatal(err)
	}
	want := "# The app.\npackage:\n  name: app # its name\ndependencies:\n" +
		"  lib: " + source(lib, "") + "\n  shared: ../shared\n  first: ../one.yakl\n"
	if got := read(t, pkg.Path); got != want {
		t.Errorf("yak.pkg after add:\n%s\nwant:\n%s", got, want)
	}
	if _, err := os.Stat(filepath.Join(pkg.Root(), deps.LockFile)); err != nil {
		t.Errorf("adding a git package did not write yak.lock: %v", err)
	}

	pkg = readPackage(t, pkg.Root())
	if err := deps.Add(pkg, filepath.Join(pkg.Root(), "..", "shared"), "", deps.Options{}); err == nil ||
		!strings.Contains(err.Error(), `already has a dependency "shared"`) {
		t.Errorf("adding shared twice: error = %v, want a complaint", err)
	}

	if err := deps.Remove(pkg, "lib", deps.Options{}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{deps.LockFile, module.VendorDir} {
		if _, err := os.Stat(filepath.Join(pkg.Root(), name)); err == nil {
			t.Errorf("%s is still there after the last git package was removed", name)
		}
	}
	pkg = readPackage(t, pkg.Root())
	if err := deps.Remove(pkg, "nope", deps.Options{}); err == nil || !strings.Contains(err.Error(), `no dependency "nope"`) {
		t.Errorf("removing nope: error = %v, want a complaint", err)
	}
}

func TestAddLeavesYakPkgAloneOnFailure(t *testing.T) {
	plain := repo(t, map[string]string{"README": "not a package\n"})
	const pkgfile = "package:\n  name: app\n"
	pkg := project(t, pkgfile)

	err := deps.Add(pkg, source(plain, ""), "plain", deps.Options{})
	if err == nil || !strings.Contains(err.Error(), "is not a package") {
		t.Errorf("error = %v, want the repository refused", err)
	}
	if got := read(t, pkg.Path); got != pkgfile {
		t.Errorf("yak.pkg = %q, want it unchanged", got)
	}
}

func TestFetchSyncsThePackage(t *testing.T) {
	util := repo(t, map[string]string{"yak.pkg": "package:\n  name: util\n"})
	app := repo(t, map[string]string{"web/yak.pkg": "package:\n  name: web\ndependencies:\n  util: " + source(util, "") + "\n"})
	src, err := module.ParseSource(source(app, "//web@main"))
	if err != nil {
		t.Fatal(err)
	}
	dir, err := deps.Fetch(src, t.TempDir(), deps.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor", filepath.FromSlash(key(util)), module.PkgFile)); err != nil {
		t.Errorf("the fetched package's dependencies were not vendored: %v", err)
	}
}
