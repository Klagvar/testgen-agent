package benchmark

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// Cloner encapsulates git operations used by the benchmark harness. It
// exists as a struct (rather than free functions) so tests can swap in a
// fake GitBin without touching the global PATH.
type Cloner struct {
	GitBin string    // defaults to "git" when empty
	Stdout io.Writer // where to mirror git stdout (nil = discard)
	Stderr io.Writer // where to mirror git stderr (nil = discard)
}

// NewCloner returns a Cloner with sensible defaults: the git binary is
// resolved from PATH, and stdout/stderr from the current process.
func NewCloner() *Cloner {
	return &Cloner{GitBin: "git", Stdout: os.Stdout, Stderr: os.Stderr}
}

// Ensure makes sure that `<workDir>/<repo.Name>` is a working clone of
// repo.URL, checked out at repo.Head, and that the base revision
// `repo.Base` exists locally (needed for `git diff <base>` inside the
// agent). Idempotent: existing clones are re-used.
//
// Returns the absolute path to the working copy.
func (c *Cloner) Ensure(repo Repo, workDir string) (string, error) {
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return "", fmt.Errorf("create workdir: %w", err)
	}
	dst := filepath.Join(workDir, repo.Name)

	absDst, err := filepath.Abs(dst)
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(filepath.Join(dst, ".git")); err == nil {
		// Existing clone: fetch new objects for base and head, then check
		// out head. `git fetch --tags` covers both tag and branch refs
		// without tripping over shallow refspecs.
		if err := c.run(dst, "fetch", "--tags", "--force", "origin"); err != nil {
			return "", fmt.Errorf("fetch %s: %w", repo.Name, err)
		}
	} else {
		// Fresh clone. Blobless clone keeps the object graph small while
		// still giving us full access to commit metadata for diffing.
		if err := c.run("", "clone", "--filter=blob:none", repo.URL, absDst); err != nil {
			return "", fmt.Errorf("clone %s: %w", repo.Name, err)
		}
	}
	if err := c.run(absDst, "checkout", repo.Head); err != nil {
		return "", fmt.Errorf("checkout head %s in %s: %w", repo.Head, repo.Name, err)
	}
	// Make sure the base rev is reachable. Some tags may not be fetched
	// by default on existing clones; explicit fetch of the ref fixes that.
	if err := c.run(absDst, "fetch", "origin", repo.Base); err != nil {
		// Non-fatal: the base might already be local (branch, SHA).
		// We only bail out later if `git diff <base>` actually fails.
		if c.Stderr != nil {
			fmt.Fprintf(c.Stderr, "⚠️  fetch base %s: %v\n", repo.Base, err)
		}
	}
	return absDst, nil
}

// run executes `git <args…>` with cwd set to dir (or inherited when empty).
func (c *Cloner) run(dir string, args ...string) error {
	bin := c.GitBin
	if bin == "" {
		bin = "git"
	}
	cmd := exec.Command(bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if c.Stdout != nil {
		cmd.Stdout = c.Stdout
	}
	if c.Stderr != nil {
		cmd.Stderr = c.Stderr
	}
	return cmd.Run()
}
