package clitest

import (
	"os"
	"path/filepath"
	"testing"
)

// When recursive discovery finds act.sh+assert.txt in a directory, that directory
// is a case and its subdirectories are not scanned. A nested act.sh/assert.txt pair
// under a case is fixture material (see tests/dogfood/case_boundary).
func TestDiscoverTestCases_stopsAtCaseBoundary(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "suite")
	parent := filepath.Join(base, "parent")
	nested := filepath.Join(parent, "files", "nested_fixture")
	for _, dir := range []string{parent, nested} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	writeCase := func(dir string) {
		if err := os.WriteFile(filepath.Join(dir, "act.sh"), []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "assert.txt"), []byte("ok\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeCase(parent)
	writeCase(nested)

	cases, err := DiscoverTestCases([]string{base}, root, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1 {
		t.Fatalf("got %d cases, want 1: recursive walk must stop at parent, not pick up nested_fixture", len(cases))
	}
	if filepath.Clean(cases[0].Path) != filepath.Clean(parent) {
		t.Fatalf("got case path %q, want %q", cases[0].Path, parent)
	}
}
