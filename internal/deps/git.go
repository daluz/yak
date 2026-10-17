package deps

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/daluz/yak/internal/module"
)

// git runs git in dir and answers what it wrote to standard output.
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if errors.Is(err, exec.ErrNotFound) {
		return "", errors.New("fetching a git package needs git, which is not installed")
	}
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return string(out), nil
}

// checkout clones the repository of src into dir and checks out rev, a
// branch, tag or commit, or the default branch when rev is empty. It
// answers the commit checked out.
func checkout(src module.Source, rev, dir string) (string, error) {
	if _, err := git("", "clone", "--quiet", "--", src.Repo, dir); err != nil {
		return "", err
	}
	commit := ""
	if rev == "" {
		rev = "HEAD"
	}
	for _, r := range []string{rev, "origin/" + rev} {
		if out, err := git(dir, "rev-parse", "--verify", "--quiet", r+"^{commit}"); err == nil {
			commit = strings.TrimSpace(out)
			break
		}
	}
	if commit == "" {
		return "", fmt.Errorf("%s has no branch, tag or commit %q", src.Repo, rev)
	}
	if _, err := git(dir, "checkout", "--quiet", "--detach", commit); err != nil {
		return "", err
	}
	return commit, nil
}
