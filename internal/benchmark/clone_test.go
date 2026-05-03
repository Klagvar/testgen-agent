package benchmark

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// TestCloner_Reset verifies that Reset returns the working tree to the
// commit identified by headSHA: tracked files modified since the run
// must be reverted, and untracked files (mimicking .testgen-cache.json
// or *_generated_test.go) must be removed. Without this guarantee the
// benchmark harness cannot run consecutive ablation configurations on
// the same clone.
func TestCloner_Reset(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	mustGit(t, "", "init", repo)
	mustGit(t, repo, "config", "user.email", "bench@test")
	mustGit(t, repo, "config", "user.name", "bench")
	mustGit(t, repo, "checkout", "-B", "main")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\nfunc main(){}")
	mustGit(t, repo, "add", ".")
	mustGit(t, repo, "commit", "-m", "init")

	headSHA := strings.TrimSpace(gitOut(t, repo, "rev-parse", "HEAD"))

	writeFile(t, filepath.Join(repo, "main.go"), "package main\nfunc main(){println(\"dirty\")}")
	writeFile(t, filepath.Join(repo, "main_generated_test.go"), "package main")
	writeFile(t, filepath.Join(repo, ".testgen-cache.json"), `{"version":"1","entries":{}}`)
	if err := os.MkdirAll(filepath.Join(repo, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, "subdir", "junk.txt"), "noise")

	cl := &Cloner{GitBin: "git"}
	if err := cl.Reset(repo, headSHA); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(repo, "main.go"))
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	// Normalise CRLF: on Windows git's autocrlf may rewrite line endings
	// during `reset --hard`. We care about content, not byte-for-byte
	// identity with the source string we wrote earlier.
	gotNorm := strings.ReplaceAll(string(got), "\r\n", "\n")
	if gotNorm != "package main\nfunc main(){}" {
		t.Fatalf("main.go not reverted, got: %q", gotNorm)
	}
	for _, p := range []string{
		"main_generated_test.go",
		".testgen-cache.json",
		"subdir/junk.txt",
		"subdir",
	} {
		if _, err := os.Stat(filepath.Join(repo, p)); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, err=%v", p, err)
		}
	}
}

// TestCloner_Reset_RejectsEmptyArgs guards the contract that callers
// never accidentally pass an empty headSHA (which on older git versions
// would silently reset to whatever HEAD currently points at).
func TestCloner_Reset_RejectsEmptyArgs(t *testing.T) {
	cl := &Cloner{GitBin: "git"}
	if err := cl.Reset("", "deadbeef"); err == nil {
		t.Fatal("expected error for empty dir")
	}
	if err := cl.Reset(t.TempDir(), ""); err == nil {
		t.Fatal("expected error for empty headSHA")
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(out)
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
