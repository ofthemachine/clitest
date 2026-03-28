# clitest

Integration test harness for CLIs using **`act.sh`** (run the subject) and **`assert.txt`** (ordered line expectations, with `{{pattern}}` placeholders).

## Install

```bash
go install github.com/ofthemachine/clitest/cmd/clitest@latest
```

## Standalone CLI

Create `clitest.yml` in your project root (or pass `-config`):

```yaml
binary_name: mytool
build_command: go build -o mytool .
project_root_marker: go.mod
test_dirs:
  - "integration/**"
  - "tests/cli/*"
patterns:
  custom_id: "[a-z]{3}[0-9]+"
environment:
  MY_ENV: test
copy_globs:
  - extra-binary
recursive: true
```

- **`build_command`**: optional shell string (`sh -c`). Empty skips the build step.
- **`recursive`**: when `false`, only each entry in `test_dirs` is checked for `act.sh`/`assert.txt` (no walk into subdirectories). Default is `true`.
- **`test_dirs`**: globs relative to **project root**, or a trailing `/**` on one segment (e.g. `cli_test/**` → scan that directory recursively).
- **`root`**: optional; if set, resolved relative to the config file’s directory and used as the project root instead of walking upward for `project_root_marker`.

Run:

```bash
clitest                      # uses ./clitest.yml
clitest -config path.yml     # explicit config
clitest -dir tests/one_case  # single directory (overrides test_dirs)
clitest -parallel 4
clitest -v                   # print act output on success
clitest -version
```

Exit code `1` if any case fails. Final line: `SUMMARY pass=N fail=M`.

## Go library

Use from `go test` with the `integration` build tag (typical pattern):

```go
//go:build integration

package main_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ofthemachine/clitest"
)

func TestCLI(t *testing.T) {
    _, f, _, _ := runtime.Caller(0)
    dir := filepath.Dir(f)
    root := filepath.Clean(filepath.Join(dir, "..")) // adjust to your layout
    clitest.RunSuite(t, clitest.Options{
        RootDir:           root,
        BaseDirs:          []string{dir},
        EnvOverrideVar:    "CLI_TEST_SUITE_DIR",
        BinaryName:        "mytool",
        BuildCommand:      []string{"go", "build", "-o", "mytool", "."},
        ProjectRootMarker: "go.mod",
    })
}
```

- **`RootDir`**: absolute or relative path to the project root (directory containing `ProjectRootMarker`). Required when the test package is not inside the clitest module (e.g. `filepath.Clean(filepath.Join(testDir, ".."))`).
- **`NonRecursive: true`**: only each `BaseDir` itself is checked for `act.sh`/`assert.txt` (no walk into subdirectories).

## assert.txt

- Non-empty, non-`#` lines are expectations (comments use `#`).
- Matching mode is **ordered lines** with a lookahead window on the combined stdout+stderr.
- Placeholders: `{{hash8}}`, `{{path}}`, `{{any}}`, `{{timestamp_ms}}`, … see `BuiltinPatterns` in code.
- Custom regex segment: `{{any:yourRegexHere}}` (only this form; no `re:` / `# REGEX:` / `/.../` line syntax).

`act.sh` runs with `TEST_TEMP_DIR` set and the built binary (and `copy_globs` matches) on `PATH` ahead of the rest of the environment.

## Development

```bash
make build
make test
make test-integration
```

## License

MIT
