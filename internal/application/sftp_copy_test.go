package application

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestCopyLocal covers the local→local paste backend: single file, nested
// tree, overwrite and the refuse-to-copy-into-itself guard.
func TestCopyLocal(t *testing.T) {
	svc := &SftpService{}
	root := t.TempDir()

	// Source tree: a.txt + nested dir/with b.txt
	write := func(rel, content string) {
		p := filepath.Join(root, "src", filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.txt", "alpha")
	write("dir/b.txt", "beta")
	write("dir/sub/c.txt", "gamma")

	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")

	var reported bool
	if err := svc.CopyLocal(context.Background(), src, dst, func(_, total int64) {
		reported = total > 0
	}); err != nil {
		t.Fatalf("copy tree: %v", err)
	}
	for rel, want := range map[string]string{
		"a.txt":         "alpha",
		"dir/b.txt":     "beta",
		"dir/sub/c.txt": "gamma",
	} {
		got, err := os.ReadFile(filepath.Join(dst, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("copied %s missing: %v", rel, err)
		}
		if string(got) != want {
			t.Fatalf("copied %s = %q, want %q", rel, got, want)
		}
	}
	if !reported {
		t.Fatal("progress callback never reported a non-zero total")
	}

	// Overwrite: copying again replaces file contents.
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("alpha2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.CopyLocal(context.Background(), src, dst, nil); err != nil {
		t.Fatalf("re-copy: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dst, "a.txt"))
	if string(got) != "alpha2" {
		t.Fatalf("overwrite copy = %q, want %q", got, "alpha2")
	}

	// A directory must not copy into its own subtree.
	if err := svc.CopyLocal(context.Background(), src, filepath.Join(src, "inside"), nil); err == nil {
		t.Fatal("copy into own subtree: want error, got nil")
	}

	// Cancellation mid-copy surfaces ctx.Err (single big file + pre-cancelled ctx).
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := svc.CopyLocal(ctx, src, filepath.Join(root, "dst2"), nil); err == nil {
		t.Fatal("cancelled copy: want error, got nil")
	}
}

func TestRemoteContains(t *testing.T) {
	cases := []struct {
		parent, child string
		want          bool
	}{
		{"/a", "/a", true},
		{"/a", "/a/b", true},
		{"/a", "/ab", false}, // prefix without separator boundary
		{"/a", "/b", false},
		{"/a/b", "/a", false},
	}
	for _, c := range cases {
		if got := remoteContains(c.parent, c.child); got != c.want {
			t.Errorf("remoteContains(%q, %q) = %v, want %v", c.parent, c.child, got, c.want)
		}
	}
}

func TestLocalContains(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "sub")
	if !localContains(parent, child) {
		t.Error("localContains(parent, parent/sub) = false, want true")
	}
	if !localContains(parent, parent) {
		t.Error("localContains(parent, parent) = false, want true")
	}
	if localContains(child, parent) {
		t.Error("localContains(parent/sub, parent) = true, want false")
	}
	// Prefix without separator boundary must not count.
	sibling := parent + "x"
	if localContains(parent, sibling) {
		t.Error("localContains(parent, parent+x) = true, want false")
	}
}

func TestRebaseDst(t *testing.T) {
	cases := []struct {
		root, planned, actual, want string
	}{
		{`F:\dl`, `F:\dl`, `F:\dl`, `F:\dl`},
		{`F:\dl`, `F:\dl/a`, `F:\dl`, `F:\dl\a`},
		{`F:\dl`, `F:\dl/a/b.txt`, `F:\dl`, filepath.Join(`F:\dl`, "a", "b.txt")},
	}
	for _, c := range cases {
		if got := rebaseDst(c.root, c.planned, c.actual); got != c.want {
			t.Errorf("rebaseDst(%q, %q, %q) = %q, want %q", c.root, c.planned, c.actual, got, c.want)
		}
	}
}

func TestJoinRemote(t *testing.T) {
	if got := joinRemote("/", "a"); got != "/a" {
		t.Errorf("joinRemote(root) = %q, want /a", got)
	}
	if got := joinRemote("/a/b", "c"); got != "/a/b/c" {
		t.Errorf("joinRemote = %q, want /a/b/c", got)
	}
}
