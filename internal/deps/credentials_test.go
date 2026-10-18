package deps

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

// authFile writes an auth file at path holding a password for every host.
func authFile(t *testing.T, path string, passwords map[string]string) {
	t.Helper()
	src := `{"auths": {`
	sep := ""
	for host, password := range passwords {
		src += sep + `"` + host + `": {"auth": "` + base64.StdEncoding.EncodeToString([]byte("yak:"+password)) + `"}`
		sep = ","
	}
	src += "}}"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCredentialsComeFromPodmanThenDocker(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(dir, "run"))
	t.Setenv("REGISTRY_AUTH_FILE", filepath.Join(dir, "chosen.json"))
	t.Setenv("DOCKER_CONFIG", filepath.Join(dir, "docker"))

	authFile(t, filepath.Join(dir, "chosen.json"), map[string]string{"a.example": "chosen"})
	authFile(t, filepath.Join(dir, "run", "containers", "auth.json"), map[string]string{"a.example": "runtime", "b.example": "runtime"})
	authFile(t, filepath.Join(dir, ".config", "containers", "auth.json"), map[string]string{"c.example": "config"})
	authFile(t, filepath.Join(dir, "docker", "config.json"), map[string]string{"c.example": "docker", "d.example": "docker"})

	store, err := credentialStore()
	if err != nil {
		t.Fatal(err)
	}
	for host, want := range map[string]string{
		"a.example": "chosen",
		"b.example": "runtime",
		"c.example": "config",
		"d.example": "docker",
		"e.example": "",
	} {
		cred, err := store.Get(context.Background(), host)
		if err != nil {
			t.Fatal(err)
		}
		if cred.Password != want {
			t.Errorf("password for %s = %q, want %q", host, cred.Password, want)
		}
	}
}
