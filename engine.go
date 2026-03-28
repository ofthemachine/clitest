package clitest

import (
	"bytes"
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
// The caller is responsible for creating and cleaning up tempDir.
func RunCase(tc CLITestCase, tempDir string, projectRoot string, binaryName string, copyGlobs []string, env map[string]string, patterns map[string]string) error {
	if err := CopyTestDirectoryContents(tc.Path, tempDir); err != nil {
		return fmt.Errorf("copy fixtures: %w", err)
	}
	allGlobs := append([]string{binaryName}, copyGlobs...)
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
