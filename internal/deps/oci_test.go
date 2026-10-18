package deps_test

import (
	"context"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"oras.land/oras-go/v2/content/oci"

	"github.com/daluz/yak/internal/deps"
	"github.com/daluz/yak/internal/module"
)

// registryHost serves an empty OCI registry over HTTP and answers its host.
func registryHost(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

// library writes a package named lib, version 1.0.0, with a hidden file and
// a vendor directory that a build leaves out.
func library(t *testing.T) *module.Package {
	t.Helper()
	pkg := project(t, "package:\n  name: lib\n  version: 1.0.0\n")
	write(t, pkg.Root(), map[string]string{
		"labels.yakl":         "team:: \"a\"\n",
		"files/motd.txt":      "hello\n",
		".hidden/secret":      "no\n",
		"vendor/other/x.yakl": "",
	})
	return pkg
}

func TestBuildPushAndAddFromARegistry(t *testing.T) {
	host := registryHost(t)
	lib := library(t)
	if err := deps.Build(lib, deps.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := deps.Push(lib, "oci://"+host+"/acme/lib", deps.Options{PlainHTTP: true}); err != nil {
		t.Fatal(err)
	}

	app := project(t, "package:\n  name: app\n")
	ref := "oci://" + host + "/acme/lib:1.0.0"
	if err := deps.Add(app, ref, "", deps.Options{PlainHTTP: true}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, app.Path); !strings.Contains(got, "lib: "+ref) {
		t.Errorf("yak.pkg does not depend on lib:\n%s", got)
	}
	vendored := filepath.Join(app.Root(), "vendor", "127.0.0.1", "acme", "lib")
	for name, want := range map[string]bool{"labels.yakl": true, "files/motd.txt": true, "yak.pkg": true, ".hidden": false, "vendor": false} {
		if _, err := os.Stat(filepath.Join(vendored, name)); (err == nil) != want {
			t.Errorf("vendored %s: present = %v, want %v", name, err == nil, want)
		}
	}
	if lock := read(t, filepath.Join(app.Root(), module.LockFile)); !strings.Contains(lock, "digest: sha256:") {
		t.Errorf("yak.lock pins no digest:\n%s", lock)
	}
	if _, err := module.Resolve(filepath.Join(app.Root(), "app.yak"), []string{"lib", "labels"}); err != nil {
		t.Error(err)
	}

	src, err := module.ParseSource(ref)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := deps.Fetch(src, t.TempDir(), deps.Options{PlainHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "labels.yakl")); err != nil {
		t.Errorf("Fetch did not unpack the package: %v", err)
	}
}

func TestBuildNeedsATag(t *testing.T) {
	pkg := project(t, "package:\n  name: lib\n")
	if err := deps.Build(pkg, deps.BuildOptions{}); err == nil || !strings.Contains(err.Error(), "give --tag") {
		t.Errorf("error = %v, want a tag asked for", err)
	}
}

func TestBuildToAnArchive(t *testing.T) {
	out := filepath.Join(t.TempDir(), "lib.tar")
	if err := deps.Build(library(t), deps.BuildOptions{Tag: "dev", Output: out}); err != nil {
		t.Fatal(err)
	}
	store, err := oci.NewFromTar(context.Background(), out)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve(context.Background(), "dev"); err != nil {
		t.Errorf("the archive has no build tagged dev: %v", err)
	}
}

func TestPushNeedsABuild(t *testing.T) {
	err := deps.Push(library(t), "oci://"+registryHost(t)+"/acme/lib", deps.Options{PlainHTTP: true})
	if err == nil || !strings.Contains(err.Error(), "run yak package build first") {
		t.Errorf("error = %v, want a build asked for", err)
	}
}

func TestInit(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my_app")
	write(t, dir, map[string]string{".gitignore": "/build"})

	if err := deps.Init(dir, ""); err != nil {
		t.Fatal(err)
	}
	if got, want := read(t, filepath.Join(dir, module.PkgFile)), "package:\n  name: my_app\n  version: 0.1.0\ndependencies: {}\n"; got != want {
		t.Errorf("yak.pkg = %q, want %q", got, want)
	}
	if got, want := read(t, filepath.Join(dir, ".gitignore")), "/build\n/vendor/\n/.yak/\n"; got != want {
		t.Errorf(".gitignore = %q, want %q", got, want)
	}
	if got := read(t, filepath.Join(dir, "app.yak")); !strings.Contains(got, `app: "my_app"`) {
		t.Errorf("app.yak = %q, want a first template", got)
	}
	if err := deps.Init(dir, ""); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("second Init: error = %v, want the yak.pkg kept", err)
	}
	if err := deps.Init(filepath.Join(t.TempDir(), "my-app"), ""); err == nil || !strings.Contains(err.Error(), "--name") {
		t.Errorf("Init of my-app: error = %v, want a name asked for", err)
	}
}
