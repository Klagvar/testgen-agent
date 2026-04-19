package benchmark

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeYAML(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "dataset.yaml")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("write dataset: %v", err)
	}
	return path
}

func TestLoadDataset_HappyPath(t *testing.T) {
	dir := t.TempDir()
	p := writeYAML(t, dir, `
workdir: ./checkouts
repos:
  - name: foo
    url: https://example.com/foo.git
    base: v1.0.0
    head: HEAD
  - name: bar
    url: https://example.com/bar.git
    base: main
    head: feat
    subdir: internal/bar
    exclude: ["**/*_gen.go"]
`)
	ds, err := LoadDataset(p)
	if err != nil {
		t.Fatalf("LoadDataset: %v", err)
	}
	if !strings.HasSuffix(ds.WorkDir, "checkouts") {
		t.Fatalf("WorkDir not resolved: %s", ds.WorkDir)
	}
	if !filepath.IsAbs(ds.WorkDir) {
		t.Fatalf("WorkDir must be absolute, got %s", ds.WorkDir)
	}
	if len(ds.Repos) != 2 {
		t.Fatalf("repos=%d, want 2", len(ds.Repos))
	}
	if ds.Repos[1].Subdir != "internal/bar" {
		t.Fatalf("subdir=%q", ds.Repos[1].Subdir)
	}
}

func TestLoadDataset_MissingField(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		body, want string
	}{
		{"repos: []\n", "no repos"},
		{"repos:\n  - url: x\n    base: main\n    head: HEAD\n", "name is required"},
		{"repos:\n  - name: foo\n    base: main\n    head: HEAD\n", "url is required"},
		{"repos:\n  - name: foo\n    url: x\n    head: HEAD\n", "base is required"},
		{"repos:\n  - name: foo\n    url: x\n    base: main\n", "head is required"},
		{"repos:\n  - name: a\n    url: x\n    base: m\n    head: H\n  - name: a\n    url: y\n    base: m\n    head: H\n", "duplicate name"},
		{"repos:\n  - name: a/b\n    url: x\n    base: m\n    head: H\n", "path separators"},
	}
	for i, c := range cases {
		p := filepath.Join(dir, "d.yaml")
		if err := os.WriteFile(p, []byte(c.body), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
		_, err := LoadDataset(p)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("case %d: err=%v, want to contain %q", i, err, c.want)
		}
	}
}

func TestAgentDir(t *testing.T) {
	if got := AgentDir("/work/foo", Repo{}); got != "/work/foo" {
		t.Fatalf("empty subdir: %s", got)
	}
	got := AgentDir("/work/foo", Repo{Subdir: "pkg/internal"})
	want := filepath.Join("/work/foo", "pkg/internal")
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}
