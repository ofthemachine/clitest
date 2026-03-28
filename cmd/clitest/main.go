package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
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

			if err := clitest.CopyTestDirectoryContents(tc.Path, tempDir); err != nil {
				fmt.Fprintf(os.Stderr, "clitest: %s: copy fixtures: %v\n", tc.Name, err)
				atomic.AddInt32(&failCount, 1)
				return
			}

			allGlobs := append([]string{cfg.BinaryName}, cfg.CopyGlobs...)
			for _, pattern := range allGlobs {
				if strings.TrimSpace(pattern) == "" {
					continue
				}
				absPattern := filepath.Join(projectRoot, pattern)
				matches, err := filepath.Glob(absPattern)
				if err != nil {
					fmt.Fprintf(os.Stderr, "clitest: %s: glob %q: %v\n", tc.Name, pattern, err)
					atomic.AddInt32(&failCount, 1)
					return
				}
				for _, match := range matches {
					dst := filepath.Join(tempDir, filepath.Base(match))
					if err := clitest.CopyFile(match, dst); err != nil {
						fmt.Fprintf(os.Stderr, "clitest: %s: copy %s: %v\n", tc.Name, match, err)
						atomic.AddInt32(&failCount, 1)
						return
					}
					_ = os.Chmod(dst, 0755)
				}
			}

			stdout, stderr, _, actErr := clitest.RunActScript(tempDir, tc.ActScript, cfg.Environment)
			combined := stdout + stderr
			if actErr != nil {
				var exitErr *exec.ExitError
				if !errors.As(actErr, &exitErr) {
					fmt.Fprintf(os.Stderr, "clitest: %s: act.sh: %v\n", tc.Name, actErr)
					atomic.AddInt32(&failCount, 1)
					return
				}
			}

			if err := clitest.AssertResultsText(tc.AssertFile, combined, patterns); err != nil {
				fmt.Fprintf(os.Stderr, "clitest: FAIL %s (%.2fs)\n%s\n", tc.Name, time.Since(start).Seconds(), err)
				fmt.Fprintf(os.Stderr, "--- output ---\n%s\n", combined)
				atomic.AddInt32(&failCount, 1)
				return
			}

			if *verbose {
				fmt.Fprintf(os.Stdout, "--- %s output ---\n%s\n", tc.Name, combined)
			}
			fmt.Printf("PASS %s (%.2fs)\n", tc.Name, time.Since(start).Seconds())
			atomic.AddInt32(&passCount, 1)
		}()
	}
	wg.Wait()

	fmt.Printf("SUMMARY pass=%d fail=%d\n", passCount, failCount)
	if failCount > 0 {
		os.Exit(1)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
