package benchmark

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestCloner_Ensure creates a bare local git repository in a temp dir,
// then uses the Cloner to materialise a working copy — the whole test
// runs offline. Skipped if `git` is not on PATH.
func TestCloner_Ensure(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	root := t.TempDir()
	// 1. Set up an upstream working repo with a tagged base and extra
	//    commit on top so head != base (so `git checkout head` is a real
	//    operation, not a no-op).
	upstream := filepath.Join(root, "upstream")
	mustGit(t, "", "init", upstream)
	mustGit(t, upstream, "config", "user.email", "bench@test")
	mustGit(t, upstream, "config", "user.name", "bench")
	// Ensure a stable default branch name across git versions.
	mustGit(t, upstream, "checkout", "-B", "main")
	writeFile(t, filepath.Join(upstream, "README.md"), "hello")
	mustGit(t, upstream, "add", ".")
	mustGit(t, upstream, "commit", "-m", "init")
	mustGit(t, upstream, "tag", "v1.0.0")
	writeFile(t, filepath.Join(upstream, "CHANGELOG.md"), "v2")
	mustGit(t, upstream, "add", ".")
	mustGit(t, upstream, "commit", "-m", "v2")

	// 2. Convert upstream into a bare clone so we can `clone` it from
	//    the test through a file:// URL.
	bare := filepath.Join(root, "bare.git")
	mustGit(t, "", "clone", "--bare", upstream, bare)

	workDir := filepath.Join(root, "checkouts")
	cl := &Cloner{GitBin: "git"}
	r := Repo{Name: "demo", URL: bare, Base: "v1.0.0", Head: "main"}
	dir, err := cl.Ensure(r, workDir)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "CHANGELOG.md")); err != nil {
		t.Fatalf("expected head commit files to be checked out: %v", err)
	}

	// 3. Second call must be idempotent (re-fetch + re-checkout,
	//    no re-clone).
	dir2, err := cl.Ensure(r, workDir)
	if err != nil {
		t.Fatalf("Ensure idempotent: %v", err)
	}
	if dir != dir2 {
		t.Fatalf("dir changed between calls: %q vs %q", dir, dir2)
	}
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var buf bytes.Buffer
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v: %v\nstderr: %s", args, err, buf.String())
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
