package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ofthemachine/clitest"
)

var version = "dev"

func main() {
	configPath := flag.String("config", "clitest.yml", "path to clitest.yml")
	dirOverride := flag.String("dir", "", "if set, run only this directory (overrides test_dirs)")
	parallel := flag.Int("parallel", runtime.NumCPU(), "max concurrent cases")
	verbose := flag.Bool("v", false, "print act output on success")
	showVersion := flag.Bool("version", false, "print version and exit")
	session := flag.Bool("session", false, "start an interactive shell in a prepared test temp directory (requires -dir)")
	shell := flag.String("shell", "", "shell for -session (default: $SHELL, then zsh, then sh)")
	flag.Parse()

	if *showVersion {
		fmt.Printf("clitest %s\n", version)
		os.Exit(0)
	}

	absConfig, err := filepath.Abs(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "clitest: config path: %v\n", err)
		os.Exit(2)
	}
	if _, err := os.Stat(absConfig); err != nil {
		fmt.Fprintf(os.Stderr, "clitest: config file %q: %v\n", absConfig, err)
		os.Exit(2)
	}

	cfg, err := clitest.LoadConfigFile(absConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "clitest: %v\n", err)
		os.Exit(2)
	}

	configDir := filepath.Dir(absConfig)
	marker := cfg.ProjectRootMarker
	if marker == "" {
		marker = "go.mod"
	}

	rootOpt := strings.TrimSpace(cfg.Root)
	var projectRoot string
	if rootOpt != "" {
		projectRoot = filepath.Clean(filepath.Join(configDir, rootOpt))
	} else {
		projectRoot, err = clitest.FindProjectRoot(configDir, marker)
		if err != nil {
			fmt.Fprintf(os.Stderr, "clitest: find project root: %v\n", err)
			os.Exit(2)
		}
	}

	if *session {
		if *dirOverride == "" {
			fmt.Fprintf(os.Stderr, "clitest: -session requires -dir\n")
			os.Exit(2)
		}
		testDir, err := filepath.Abs(*dirOverride)
		if err != nil {
			fmt.Fprintf(os.Stderr, "clitest: -dir: %v\n", err)
			os.Exit(2)
		}
		binaryName := cfg.BinaryName
		if binaryName == "" {
			binaryName = "app"
		}
		if err := clitest.StartTestSession(projectRoot, testDir, binaryName, cfg.BuildCommand, cfg.CopyGlobs, cfg.Environment, *shell); err != nil {
			fmt.Fprintf(os.Stderr, "clitest: %v\n", err)
			os.Exit(2)
		}
		return
	}

	var bases []string
	if *dirOverride != "" {
		absDir, err := filepath.Abs(*dirOverride)
		if err != nil {
			fmt.Fprintf(os.Stderr, "clitest: -dir: %v\n", err)
			os.Exit(2)
		}
		bases = []string{absDir}
	} else {
		dirs := cfg.TestDirs
		if len(dirs) == 0 {
			dirs = []string{"tests"}
		}
		bases, err = clitest.ResolveTestRoots(projectRoot, dirs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "clitest: resolve test_dirs: %v\n", err)
			os.Exit(2)
		}
	}
	if len(bases) == 0 {
		fmt.Fprintf(os.Stderr, "clitest: no test directories matched\n")
		os.Exit(2)
	}

	if err := clitest.BuildInDir(projectRoot, cfg.BuildCommand); err != nil {
		fmt.Fprintf(os.Stderr, "clitest: %v\n", err)
		os.Exit(2)
	}

	recursive := true
	if cfg.Recursive != nil {
		recursive = *cfg.Recursive
	}
	cases, err := clitest.DiscoverTestCases(bases, projectRoot, recursive)
	if err != nil {
		fmt.Fprintf(os.Stderr, "clitest: discover: %v\n", err)
		os.Exit(2)
	}
	if len(cases) == 0 {
		fmt.Fprintf(os.Stderr, "clitest: no cases (act.sh + assert.txt) found\n")
		os.Exit(2)
	}

	patterns := clitest.MergePatterns(cfg.Patterns)
	p := max(1, *parallel)

	var passCount, failCount int32
	var failedPaths []string
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, p)

	for _, tc := range cases {
		tc := tc
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			start := time.Now()
			tempDir, err := os.MkdirTemp("", "clitest-case-*")
			if err != nil {
				fmt.Fprintf(os.Stderr, "clitest: %s: temp dir: %v\n", tc.Name, err)
				atomic.AddInt32(&failCount, 1)
				return
			}
			defer os.RemoveAll(tempDir)

			if err := clitest.RunCase(tc, tempDir, projectRoot, cfg.BinaryName, cfg.CopyGlobs, cfg.Environment, patterns); err != nil {
				fmt.Fprintf(os.Stderr, "clitest: FAIL %s (%.2fs)\n%s\n", tc.Name, time.Since(start).Seconds(), err)
				atomic.AddInt32(&failCount, 1)
				mu.Lock()
				failedPaths = append(failedPaths, tc.Path)
				mu.Unlock()
				return
			}

			if *verbose {
				fmt.Fprintf(os.Stdout, "--- %s ---\n", tc.Name)
			}
			fmt.Printf("PASS %s (%.2fs)\n", tc.Name, time.Since(start).Seconds())
			atomic.AddInt32(&passCount, 1)
		}()
	}
	wg.Wait()

	fmt.Printf("\nSUMMARY pass=%d fail=%d\n", passCount, failCount)
	if failCount > 0 {
		fmt.Printf("FAILED TESTS:\n")
		for _, p := range failedPaths {
			fmt.Printf("  %s  \n", p)
		}
		os.Exit(1)
	}
}
