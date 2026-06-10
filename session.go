package clitest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// StartTestSession builds the binary, prepares a temp directory mirroring RunCase,
// and starts an interactive shell with the same environment act.sh would receive.
func StartTestSession(projectRoot, testDir, binaryName, buildCommand string, copyGlobs []string, env map[string]string, shell string) error {
	testDir = filepath.Clean(testDir)
	if st, err := os.Stat(testDir); err != nil || !st.IsDir() {
		return fmt.Errorf("test directory %q not found", testDir)
	}
	actScript := filepath.Join(testDir, "act.sh")
	if !exists(actScript) {
		return fmt.Errorf("act.sh not found in %s", testDir)
	}

	if err := BuildInDir(projectRoot, buildCommand); err != nil {
		return err
	}

	sessionDir, err := os.MkdirTemp("", "clitest-session-*")
	if err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}

	if err := PrepareSessionDir(testDir, sessionDir, projectRoot, binaryName, copyGlobs); err != nil {
		os.RemoveAll(sessionDir)
		return err
	}

	fmt.Printf("Setting up test session in: %s\n", sessionDir)
	fmt.Println("Test session environment created.")
	fmt.Printf("Binary %q is on PATH. Run ./act.sh to reproduce the test act.\n", binaryName)
	fmt.Println("Type 'exit' to close and clean up.")

	interactiveShell := resolveShell(shell)
	script := fmt.Sprintf(
		`SESSION_DIR=%q
trap 'echo "Cleaning up test session..."; rm -rf "$SESSION_DIR"' EXIT
cd "$SESSION_DIR"
%s -i --no-rcs
`,
		sessionDir,
		shellQuote(interactiveShell),
	)

	cmd := exec.Command("sh", "-c", script)
	cmd.Env = ActScriptEnv(sessionDir, env)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("test session: %w", err)
	}
	return nil
}

func resolveShell(preferred string) string {
	if s := strings.TrimSpace(preferred); s != "" {
		return s
	}
	if s := strings.TrimSpace(os.Getenv("SHELL")); s != "" {
		return s
	}
	for _, candidate := range []string{"/bin/zsh", "/bin/bash", "/bin/sh"} {
		if exists(candidate) {
			return candidate
		}
	}
	return "/bin/sh"
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
