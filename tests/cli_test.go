//go:build integration

package clitest_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ofthemachine/clitest"
)

func TestClitestDogfood(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	testDir := filepath.Dir(testFile)
	dog := filepath.Join(testDir, "dogfood")
	repoRoot := filepath.Clean(filepath.Join(testDir, ".."))
	opts := clitest.Options{
		RootDir: repoRoot,
		BaseDirs: []string{
			filepath.Join(dog, "version"),
			filepath.Join(dog, "help"),
			filepath.Join(dog, "config_run"),
			filepath.Join(dog, "glob_run"),
		},
		NonRecursive:      true,
		EnvOverrideVar:    "CLITEST_SELFTEST_DIR",
		BinaryName:        "clitest",
		BuildCommand:      []string{"go", "build", "-o", "clitest", "./cmd/clitest"},
		ProjectRootMarker: "go.mod",
	}

	clitest.RunSuite(t, opts)
}
