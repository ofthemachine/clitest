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

// Cases link one staged copy of the binary: no case writes a new executable
// (slow to start on first run), and nothing a case does reaches the
// project's binary.
func TestStagedBinaryIsLinkedNotCopied(t *testing.T) {
	project := t.TempDir()
	bin := filepath.Join(project, "app")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho v1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	stage, err := StageBinary(project, "app")
	if err != nil {
		t.Fatal(err)
	}
	defer CleanStageDir(stage)
	if _, err := os.Stat(filepath.Join(stage, StageSentinel)); err != nil {
		t.Fatalf("missing stage sentinel %s: %v", StageSentinel, err)
	}
	staged, err := os.Stat(filepath.Join(stage, "app"))
	if err != nil {
		t.Fatal(err)
	}
	if staged.Mode().Perm()&0100 == 0 {
		t.Errorf("staged binary mode %v, want executable", staged.Mode())
	}

	fixtures := t.TempDir()
	var caseBins []string
	for i := 0; i < 2; i++ {
		dir := t.TempDir()
		if err := PrepareCaseDir(fixtures, dir, project, stage, "app", nil); err != nil {
			t.Fatal(err)
		}
		caseBins = append(caseBins, filepath.Join(dir, "app"))
	}
	for _, b := range caseBins {
		fi, err := os.Stat(b)
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(fi, staged) {
			t.Errorf("%s is not the staged file: each case would write (and first-run) a new executable", b)
		}
	}

	// A case that rewrites its binary changes the stage, never the project.
	if err := os.WriteFile(caseBins[0], []byte("changed"), 0755); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(bin); string(got) != "#!/bin/sh\necho v1\n" {
		t.Errorf("project binary changed through a case: %q", got)
	}
}

// Without a stage the binary is copied, as before; and where a link can't
// be made, linkOrCopy copies (executable) instead.
func TestBinaryCopiedWithoutStageOrLink(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "app"), []byte("bin"), 0755); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := PrepareCaseDir(t.TempDir(), dir, project, "", "app", nil); err != nil {
		t.Fatal(err)
	}
	a, _ := os.Stat(filepath.Join(project, "app"))
	b, err := os.Stat(filepath.Join(dir, "app"))
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(a, b) {
		t.Error("no stage: the case must get a copy, not the project's own file")
	}

	// os.Link refuses an existing dst, which takes the copy path.
	dst := filepath.Join(t.TempDir(), "app")
	if err := os.WriteFile(dst, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := linkOrCopy(filepath.Join(project, "app"), dst); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "bin" || fi.Mode().Perm()&0100 == 0 {
		t.Errorf("fallback copy = %q mode %v, want the binary, executable", got, fi.Mode())
	}
}

func TestCleanStageDir(t *testing.T) {
	// Empty directory is a no-op
	if err := CleanStageDir(""); err != nil {
		t.Errorf("CleanStageDir(\"\") = %v, want nil", err)
	}

	// Refuses to delete directory without sentinel
	dir := t.TempDir()
	canary := filepath.Join(dir, "important.txt")
	if err := os.WriteFile(canary, []byte("preserve"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := CleanStageDir(dir); err == nil {
		t.Errorf("CleanStageDir on dir without sentinel succeeded; want error")
	}
	if _, err := os.Stat(canary); err != nil {
		t.Errorf("canary was deleted when sentinel was absent")
	}

	// Deletes directory when sentinel is present
	stageDir, err := os.MkdirTemp("", "clitest-stage-test-*")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stageDir, StageSentinel), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := CleanStageDir(stageDir); err != nil {
		t.Errorf("CleanStageDir failed on valid stage dir: %v", err)
	}
	if _, err := os.Stat(stageDir); !os.IsNotExist(err) {
		t.Errorf("stageDir still exists after CleanStageDir")
	}
}
