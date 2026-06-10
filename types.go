package clitest

// Options controls how the CLI harness runs.
type Options struct {
	// BaseDirs lists roots to scan recursively for directories containing act.sh and assert.txt.
	BaseDirs []string
	// EnvOverrideVar, if set in the process environment, replaces BaseDirs with that single directory.
	EnvOverrideVar string
	// BinaryName is the built artifact basename copied into each test temp dir (also used as default build output).
	BinaryName string
	// BuildCommand is run once from the project root before any case (e.g. {"go","build","-o","mybin"}).
	BuildCommand []string
	// ProjectRootMarker is a filename that must exist in the project root (e.g. "go.mod").
	ProjectRootMarker string
	// RootDir is the absolute project root (directory containing ProjectRootMarker).
	// If empty, the root is found by walking upward from this package's source file (only valid when tests live in the clitest module).
	RootDir string
	// DefaultPatterns maps {{name}} placeholders to regex fragments; merged over BuiltinPatterns (user wins).
	DefaultPatterns map[string]string
	// Environment is extra key=value pairs passed to act.sh.
	Environment map[string]string
	// CopyGlobs are glob patterns relative to project root; matched files are copied into each test temp dir.
	CopyGlobs []string
	// NonRecursive, when true, only considers act.sh/assert.txt directly in each BaseDir (no subdirectory walk).
	// When false (default), walks subdirectories but does not descend past directories that are test cases.
	NonRecursive bool
}

// Result holds aggregate counts from RunSuite.
type Result struct {
	Total, Passed, Failed int32
	FailedDetails         []string
}

// CLITestCase is one discovered directory with act.sh + assert.txt.
type CLITestCase struct {
	Name       string
	Path       string
	ActScript  string
	AssertFile string
}
