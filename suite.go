package clitest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// BuiltinPatterns are always available in assert.txt {{name}} expansion.
// Options.DefaultPatterns are merged on top (user patterns win).
var BuiltinPatterns = map[string]string{
	"hash8":        `[a-f0-9]{8}`,
	"hash64":       `[a-f0-9]{64}`,
	"timestamp":    `\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}[+-]\d{2}:\d{2}`,
	"timestamp_ms": `\d{13,}`,
	"number":       `\d+`,
	"path":         `/[^\s]+`,
	"any":          `.+`,
	"content":      `[A-Za-z0-9+/=]+`,
}

var (
	projectRoot string
	builtOnce   bool
)

// RunSuite discovers and runs CLI tests using act.sh + assert.txt.
func RunSuite(t *testing.T, opts Options) Result {
	if len(opts.BaseDirs) == 0 {
		opts.BaseDirs = []string{"cmd_samples", "integration"}
	}
	if opts.EnvOverrideVar == "" {
		opts.EnvOverrideVar = "CLI_TEST_SUITE_DIR"
	}
	if opts.BinaryName == "" {
		opts.BinaryName = "app"
	}
	if len(opts.BuildCommand) == 0 {
		opts.BuildCommand = []string{"go", "build", "-o", opts.BinaryName}
	}
	if opts.ProjectRootMarker == "" {
		opts.ProjectRootMarker = "go.mod"
	}
	patterns := MergePatterns(opts.DefaultPatterns)

	buildOnce(t, opts)

	if override := os.Getenv(opts.EnvOverrideVar); override != "" {
		opts.BaseDirs = []string{override}
	}

	testCases, err := DiscoverTestCases(opts.BaseDirs, projectRoot, !opts.NonRecursive)
	if err != nil {
		t.Fatalf("discover test cases: %v", err)
	}
	if len(testCases) == 0 {
		t.Logf("No test cases found under: %s", strings.Join(opts.BaseDirs, ", "))
		return Result{}
	}
	t.Logf("Discovered %d test cases", len(testCases))

	var total, passed, failed int32
	var failedDetails []string

	progressEnabled := clitestProgressEnabled()

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			atomic.AddInt32(&total, 1)
			var start time.Time
			if progressEnabled {
				start = time.Now()
			}
			defer func() {
				if t.Failed() {
					atomic.AddInt32(&failed, 1)
					failedDetails = append(failedDetails, fmt.Sprintf("%s: %s", tc.Name, tc.AssertFile))
				} else {
					atomic.AddInt32(&passed, 1)
				}
				if progressEnabled && !start.IsZero() {
					status := "PASS"
					if t.Failed() {
						status = "FAIL"
					}
					t.Logf("clitest: %s %s (%.2fs)", tc.Name, status, time.Since(start).Seconds())
				}
			}()

			tempDir := t.TempDir()

			if err := CopyTestDirectoryContents(tc.Path, tempDir); err != nil {
				t.Fatalf("copy test directory contents: %v", err)
			}

			allGlobs := append([]string{opts.BinaryName}, opts.CopyGlobs...)
			for _, pattern := range allGlobs {
				absPattern := filepath.Join(projectRoot, pattern)
				matches, err := filepath.Glob(absPattern)
				if err != nil {
					t.Fatalf("invalid copy glob %q: %v", pattern, err)
				}
				for _, match := range matches {
					dst := filepath.Join(tempDir, filepath.Base(match))
					if err := CopyFile(match, dst); err != nil {
						t.Fatalf("copy %s: %v", match, err)
					}
					_ = os.Chmod(dst, 0755)
				}
			}

			stdout, stderr, _, actErr := RunActScript(tempDir, tc.ActScript, opts.Environment)
			if actErr != nil {
				if _, ok := actErr.(*exec.ExitError); !ok {
					t.Fatalf("act.sh execution failed: %v", actErr)
				}
			}

			if err := AssertResultsText(tc.AssertFile, stdout+stderr, patterns); err != nil {
				t.Errorf("assert failed: %v", err)
			}
		})
	}

	return Result{Total: total, Passed: passed, Failed: failed, FailedDetails: failedDetails}
}

func clitestProgressEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("CLITEST_PROGRESS")))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func buildOnce(t *testing.T, opts Options) {
	if projectRoot == "" {
		var root string
		var err error
		if dir := strings.TrimSpace(opts.RootDir); dir != "" {
			root, err = filepath.Abs(dir)
			if err != nil {
				t.Fatalf("resolve RootDir: %v", err)
			}
		} else {
			_, file, _, ok := runtime.Caller(0)
			if !ok {
				t.Fatalf("runtime.Caller failed")
			}
			root, err = FindProjectRoot(filepath.Dir(file), opts.ProjectRootMarker)
			if err != nil {
				t.Fatalf("find project root: %v", err)
			}
		}
		projectRoot = root
		t.Logf("Project root: %s", projectRoot)
	}
	if builtOnce {
		return
	}
	cmd := exec.Command(opts.BuildCommand[0], opts.BuildCommand[1:]...)
	cmd.Dir = projectRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build (cwd: %s): %v\nOutput:\n%s", projectRoot, err, string(out))
	}
	builtOnce = true
}
