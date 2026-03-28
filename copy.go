package clitest

import (
	"fmt"
	"os"
	"path/filepath"
)

// CopyFile copies a file by path.
func CopyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// CopyDir recursively copies a directory tree.
func CopyDir(src, dst string) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)
	si, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !si.IsDir() {
		return fmt.Errorf("source is not a directory")
	}
	if err := os.MkdirAll(dst, si.Mode()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := CopyDir(s, d); err != nil {
				return err
			}
		} else {
			if err := CopyFile(s, d); err != nil {
				return err
			}
		}
	}
	return nil
}

// CopyTestDirectoryContents copies test fixture files into tempDir, excluding act.sh and assert.txt.
func CopyTestDirectoryContents(testDir, tempDir string) error {
	entries, err := os.ReadDir(testDir)
	if err != nil {
		return fmt.Errorf("read test directory: %w", err)
	}
	skipFiles := map[string]bool{
		"act.sh":     true,
		"assert.txt": true,
	}
	for _, entry := range entries {
		if skipFiles[entry.Name()] {
			continue
		}
		src := filepath.Join(testDir, entry.Name())
		dst := filepath.Join(tempDir, entry.Name())
		if entry.IsDir() {
			if err := CopyDir(src, dst); err != nil {
				return fmt.Errorf("copy directory %s: %w", entry.Name(), err)
			}
		} else {
			if err := CopyFile(src, dst); err != nil {
				return fmt.Errorf("copy file %s: %w", entry.Name(), err)
			}
		}
	}
	return nil
}
