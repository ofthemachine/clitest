package clitest

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func FindProjectRoot(startPath string, marker string) (string, error) {
	current, err := filepath.Abs(startPath)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(current, marker)); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("marker %s not found", marker)
		}
		current = parent
	}
}

func MergePatterns(user map[string]string) map[string]string {
	out := make(map[string]string)
	for k, v := range BuiltinPatterns {
		out[k] = v
	}
	for k, v := range user {
		out[k] = v
	}
	return out
}

// DiscoverTestCases finds directories containing act.sh and assert.txt under each base.
// If recursive is false, only the base directory itself is checked (no walk into subdirs).
// When recursive is true, subdirectories are walked but descent stops at each case
// directory (fixture act.sh files nested under a case are not discovered separately).
func DiscoverTestCases(baseDirs []string, projectRoot string, recursive bool) ([]CLITestCase, error) {
	var out []CLITestCase
	for _, base := range baseDirs {
		if _, err := os.Stat(base); err != nil {
			continue
		}
		root := filepath.Clean(projectRoot)
		baseDir := filepath.Clean(base)
		if !recursive {
			if c, err := oneCaseAt(root, baseDir, base); err != nil {
				return nil, err
			} else if c != nil {
				out = append(out, *c)
			}
			continue
		}
		err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				act := filepath.Join(path, "act.sh")
				if exists(act) {
					assertPath := filepath.Join(path, "assert.txt")
					if !exists(assertPath) {
						return nil
					}
					caseDir := filepath.Clean(path)
					name, nameErr := cliTestCaseName(root, baseDir, caseDir)
					if nameErr != nil {
						return nameErr
					}
					out = append(out, CLITestCase{Name: name, Path: path, ActScript: act, AssertFile: assertPath})
					return filepath.SkipDir
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func oneCaseAt(projectRoot, baseDir, dir string) (*CLITestCase, error) {
	act := filepath.Join(dir, "act.sh")
	if !exists(act) {
		return nil, nil
	}
	assertPath := filepath.Join(dir, "assert.txt")
	if !exists(assertPath) {
		return nil, nil
	}
	caseDir := filepath.Clean(dir)
	name, err := cliTestCaseName(projectRoot, baseDir, caseDir)
	if err != nil {
		return nil, err
	}
	return &CLITestCase{Name: name, Path: dir, ActScript: act, AssertFile: assertPath}, nil
}

func cliTestCaseName(projectRoot, baseDir, caseDir string) (string, error) {
	if relToRoot, err := filepath.Rel(projectRoot, caseDir); err == nil && relToRoot != "." && !strings.HasPrefix(relToRoot, "..") {
		return filepath.ToSlash(relToRoot), nil
	}

	relToBase, err := filepath.Rel(baseDir, caseDir)
	if err != nil {
		return "", fmt.Errorf("build test case name from %q and %q: %w", baseDir, caseDir, err)
	}
	return filepath.ToSlash(filepath.Join(filepath.Base(baseDir), relToBase)), nil
}

// ActScriptEnv returns the environment slice used when running act.sh in tempDir.
func ActScriptEnv(tempDir string, additionalEnv map[string]string) []string {
	allow := []string{"HOME", "PATH", "SHELL", "LANG", "TZ"}
	env := []string{"TEST_TEMP_DIR=" + tempDir, "USER=test"}
	var existingPath string
	for _, k := range allow {
		if v, ok := os.LookupEnv(k); ok {
			if k == "PATH" {
				existingPath = v
				continue
			}
			env = append(env, k+"="+v)
		}
	}
	for k, v := range additionalEnv {
		env = append(env, k+"="+v)
	}
	if existingPath != "" {
		env = append(env, "PATH="+tempDir+":"+existingPath)
	} else {
		env = append(env, "PATH="+tempDir)
	}
	return env
}

// StageSentinel is placed in every staging directory created by StageBinary
// to verify ownership before recursive cleanup.
const StageSentinel = ".clitest-stage"

// StageBinary copies projectRoot/binaryName once into a new temp directory
// named for its content (clitest-bin-<sha8>-*), for cases to hard-link
// rather than copy. A freshly written executable is slow to start the first
// time (macOS scans it: about a second for a large binary), so copying it
// into every case directory costs that second per case; links share one
// file, scanned once. Linking a private copy rather than the project's own
// binary means a rebuild mid-run can't disturb running cases, and a case
// can't change the project's binary (cases do share the stage, so a case
// must not rewrite its binary in place). The stage sits in the OS temp
// directory beside the case directories, so links stay on one filesystem.
// CleanStageDir removes the directory once finished.
func StageBinary(projectRoot, binaryName string) (string, error) {
	data, err := os.ReadFile(filepath.Join(projectRoot, binaryName))
	if err != nil {
		return "", fmt.Errorf("stage %s: %w", binaryName, err)
	}
	sum := sha256.Sum256(data)
	dir, err := os.MkdirTemp("", fmt.Sprintf("clitest-bin-%x-*", sum[:4]))
	if err != nil {
		return "", fmt.Errorf("stage %s: %w", binaryName, err)
	}
	if err := os.WriteFile(filepath.Join(dir, StageSentinel), nil, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("stage %s: %w", binaryName, err)
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.Base(binaryName)), data, 0o755); err != nil {
		_ = CleanStageDir(dir)
		return "", fmt.Errorf("stage %s: %w", binaryName, err)
	}
	return dir, nil
}

// CleanStageDir safely removes a staging directory created by StageBinary.
// It verifies the directory contains StageSentinel before removing anything.
// An empty dir string is a no-op and returns nil.
func CleanStageDir(dir string) error {
	if dir == "" {
		return nil
	}
	sentinel := filepath.Join(dir, StageSentinel)
	if _, err := os.Stat(sentinel); err != nil {
		return fmt.Errorf("refusing to clean stage dir %q: missing sentinel %s", dir, StageSentinel)
	}
	return os.RemoveAll(dir)
}

// linkOrCopy hard-links src at dst, or copies it (mode 0755) where a link
// isn't possible (another filesystem, or one without links).
func linkOrCopy(src, dst string) error {
	if os.Link(src, dst) == nil {
		return nil
	}
	if err := CopyFile(src, dst); err != nil {
		return err
	}
	return os.Chmod(dst, 0o755)
}

// copyProjectArtifacts puts the binary and copyGlobs into tempDir: the
// binary linked from stageDir (see StageBinary), or copied from projectRoot
// when stageDir is "". copyGlobs are always copied, so a case can change
// its own without touching the project.
func copyProjectArtifacts(tempDir, projectRoot, stageDir, binaryName string, copyGlobs []string) error {
	allGlobs := append([]string{binaryName}, copyGlobs...)
	if stageDir != "" {
		name := filepath.Base(binaryName)
		if err := linkOrCopy(filepath.Join(stageDir, name), filepath.Join(tempDir, name)); err != nil {
			return fmt.Errorf("link %s: %w", name, err)
		}
		allGlobs = copyGlobs
	}
	for _, pattern := range allGlobs {
		if strings.TrimSpace(pattern) == "" {
			continue
		}
		matches, err := filepath.Glob(filepath.Join(projectRoot, pattern))
		if err != nil {
			return fmt.Errorf("glob %q: %w", pattern, err)
		}
		for _, match := range matches {
			dst := filepath.Join(tempDir, filepath.Base(match))
			if err := CopyFile(match, dst); err != nil {
				return fmt.Errorf("copy %s: %w", match, err)
			}
			_ = os.Chmod(dst, 0755)
		}
	}
	return nil
}

// PrepareCaseDir copies fixtures and project artifacts into tempDir for a
// test run, linking the binary from stageDir when it is set (StageBinary).
func PrepareCaseDir(testDir, tempDir, projectRoot, stageDir, binaryName string, copyGlobs []string) error {
	if err := CopyTestDirectoryContents(testDir, tempDir); err != nil {
		return fmt.Errorf("copy fixtures: %w", err)
	}
	return copyProjectArtifacts(tempDir, projectRoot, stageDir, binaryName, copyGlobs)
}

// PrepareSessionDir copies all test files (including act.sh and assert.txt) and project artifacts.
func PrepareSessionDir(testDir, tempDir, projectRoot, binaryName string, copyGlobs []string) error {
	if err := CopyAllDirectoryContents(testDir, tempDir); err != nil {
		return fmt.Errorf("copy test directory: %w", err)
	}
	if act := filepath.Join(tempDir, "act.sh"); exists(act) {
		_ = os.Chmod(act, 0755)
	}
	return copyProjectArtifacts(tempDir, projectRoot, "", binaryName, copyGlobs)
}

// RunActScript copies act.sh into tempDir, runs it, and returns stdout/stderr.
// A non-nil *exec.ExitError indicates a non-zero exit (often intentional).
func RunActScript(tempDir, actScriptPath string, additionalEnv map[string]string) (stdout, stderr string, exitCode int, err error) {
	if !exists(actScriptPath) {
		return "", "", -1, fmt.Errorf("act.sh not found: %s", actScriptPath)
	}
	local := filepath.Join(tempDir, "act.sh")
	if err := CopyFile(actScriptPath, local); err != nil {
		return "", "", -1, err
	}
	_ = os.Chmod(local, 0755)
	env := ActScriptEnv(tempDir, additionalEnv)
	const maxAttempts = 3
	var last error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(100 * time.Millisecond)
		}
		cmd := exec.Command("./act.sh")
		cmd.Dir = tempDir
		cmd.Env = env
		var outBuf, errBuf bytes.Buffer
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf
		execErr := cmd.Run()
		stdout, stderr = outBuf.String(), errBuf.String()
		if execErr != nil {
			last = execErr
			if isTransientError(execErr) && attempt < maxAttempts-1 {
				continue
			}
			if exitError, ok := execErr.(*exec.ExitError); ok {
				return stdout, stderr, exitError.ExitCode(), execErr
			}
			return stdout, stderr, -1, fmt.Errorf("running act.sh failed: %w", execErr)
		}
		return stdout, stderr, 0, nil
	}
	return "", "", -1, fmt.Errorf("max retries exceeded: %v", last)
}

// AssertResultsText reads assert.txt (comment lines and blanks skipped) and matches ORDERED_LINES.
func AssertResultsText(assertFile string, combinedOutput string, patterns map[string]string) error {
	data, err := os.ReadFile(assertFile)
	if err != nil {
		return fmt.Errorf("read assert.txt: %v", err)
	}
	var lines []string
	for _, l := range strings.Split(string(data), "\n") {
		s := strings.TrimRight(l, "\r")
		st := strings.TrimSpace(s)
		if st == "" {
			continue
		}
		if strings.HasPrefix(st, "#") {
			continue
		}
		lines = append(lines, s)
	}
	expected := strings.Join(lines, "\n")
	return MatchOutput(expected, combinedOutput, "ORDERED_LINES", patterns)
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "text file busy")
}

// ResolveTestRoots expands test_dirs entries (globs and trailing `/**`) relative to projectRoot.
func ResolveTestRoots(projectRoot string, entries []string) ([]string, error) {
	seen := make(map[string]bool)
	var out []string
	add := func(p string) {
		p = filepath.Clean(p)
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if strings.HasSuffix(e, "/**") {
			base := filepath.Join(projectRoot, strings.TrimSuffix(e, "/**"))
			st, err := os.Stat(base)
			if err != nil || !st.IsDir() {
				continue
			}
			add(base)
			continue
		}
		pat := filepath.Join(projectRoot, filepath.Clean(e))
		matches, err := filepath.Glob(pat)
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			if st, err := os.Stat(pat); err == nil && st.IsDir() {
				add(pat)
			}
			continue
		}
		for _, m := range matches {
			if st, err := os.Stat(m); err == nil && st.IsDir() {
				add(m)
			}
		}
	}
	return out, nil
}

// RunCase prepares a temp directory, runs act.sh, and asserts results for a single test case.
// The caller is responsible for creating and cleaning up tempDir. stageDir is
// a StageBinary directory to link the binary from ("" copies it instead).
func RunCase(tc CLITestCase, tempDir, projectRoot, stageDir, binaryName string, copyGlobs []string, env map[string]string, patterns map[string]string) error {
	if err := PrepareCaseDir(tc.Path, tempDir, projectRoot, stageDir, binaryName, copyGlobs); err != nil {
		return err
	}
	stdout, stderr, _, actErr := RunActScript(tempDir, tc.ActScript, env)
	if actErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(actErr, &exitErr) {
			return fmt.Errorf("act.sh: %w", actErr)
		}
	}
	return AssertResultsText(tc.AssertFile, stdout+stderr, patterns)
}

func BuildInDir(dir string, command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build in %s: %w\n%s", dir, err, string(out))
	}
	return nil
}
