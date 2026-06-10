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
		RootDir:           repoRoot,
		BaseDirs:          []string{dog},
		EnvOverrideVar:    "CLITEST_SELFTEST_DIR",
		BinaryName:        "clitest",
		BuildCommand:      []string{"go", "build", "-o", "clitest", "./cmd/clitest"},
		ProjectRootMarker: "go.mod",
		Environment:       map[string]string{"CLITEST_VERIFY_ENVVARS": "i am set"},
	}

	clitest.RunSuite(t, opts)
}
