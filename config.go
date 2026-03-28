package clitest

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// FileConfig is the on-disk YAML shape for the standalone clitest CLI.
type FileConfig struct {
	// Root is optional; if set, it is resolved relative to the config file's directory and used as the project root.
	Root string `yaml:"root"`
	// BinaryName is copied from the project root into each case temp dir (glob); may be empty if not needed.
	BinaryName string `yaml:"binary_name"`
	// BuildCommand is a shell string run once from the project root (empty skips build).
	BuildCommand string `yaml:"build_command"`
	// ProjectRootMarker is used with FindProjectRoot when Root is empty (default go.mod).
	ProjectRootMarker string `yaml:"project_root_marker"`
	// TestDirs are glob patterns or literal paths relative to project root, or suffix `/**` for one recursive base.
	TestDirs []string `yaml:"test_dirs"`
	// Patterns are extra {{name}} regex fragments merged over BuiltinPatterns.
	Patterns map[string]string `yaml:"patterns"`
	// Environment is passed to each act.sh.
	Environment map[string]string `yaml:"environment"`
	// CopyGlobs are extra artifacts to copy from project root into each case temp dir.
	CopyGlobs []string `yaml:"copy_globs"`
	// Recursive, when false, only each resolved test directory itself is checked (no subdirectory walk).
	Recursive *bool `yaml:"recursive"`
}

// LoadConfigFile reads and parses a YAML config file.
func LoadConfigFile(path string) (*FileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg FileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	return &cfg, nil
}
