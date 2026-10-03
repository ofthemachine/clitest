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
- **`root`**: optional; if set, resolved relative to the config file’s directory and used as the project root. When `root` is set, `project_root_marker` is ignored — use one or the other, not both. Nested fixture configs (see `tests/dogfood/*/files/*.yml`) typically use `root: .` only.
- **`project_root_marker`**: when `root` is omitted, walk upward from the config file to find this filename (default `go.mod`).

Run:

```bash
clitest                      # uses ./clitest.yml
clitest -config path.yml     # explicit config
clitest -dir tests/one_case  # single directory (overrides test_dirs)
clitest -parallel 4
clitest -v                   # print act output on success
clitest -version
clitest -session -dir tests/one_case  # interactive shell in a prepared temp dir
```

Exit code `1` if any case fails. Final line: `SUMMARY pass=N fail=M`.

`-session` builds the binary, copies fixtures and artifacts into a temp directory (same layout as a test run), sets `TEST_TEMP_DIR` and `PATH`, then starts an interactive shell. Requires `-dir`. Temp dir is removed when the shell exits.

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
- **Recursive discovery (default)**: walks subdirectories of each `BaseDir` for `act.sh`/`assert.txt` pairs, but does not descend into a directory once it is registered as a case (nested fixture cases stay internal to their parent test).
- **`NonRecursive: true`**: only each `BaseDir` itself is checked (no walk into subdirectories).

## assert.txt

- Non-empty, non-`#` lines are expectations (comments use `#`).
- Matching mode is **ordered lines** with a lookahead window on the combined stdout+stderr.
- Placeholders: `{{hash8}}`, `{{path}}`, `{{any}}`, `{{timestamp_ms}}`, … see `BuiltinPatterns` in code.
- Custom regex inline: `{{name:yourRegexHere}}` overrides the named pattern with a one-off regex fragment.

`act.sh` runs with `TEST_TEMP_DIR` set and the built binary (and `copy_globs` matches) on `PATH` ahead of the rest of the environment. The binary is copied once per run into a staging directory and hard-linked into each case (a copy where a link isn't possible): a freshly written executable is slow on its first run, about a second on macOS, so copying it per case cost that second every case. `copy_globs` matches are still copied, so a case can change its own; cases share the staged binary, so a case must not rewrite it in place.

## Development

```bash
make build
make test
make test-integration
```

### Debugging a test case

Open an interactive shell in the same temp-dir environment `RunCase` uses:

```bash
make test-session DIR=dogfood/version
# or
clitest -session -dir tests/dogfood/version
```

Run `./act.sh` inside the session to reproduce the test act. Type `exit` to clean up the temp directory.

## License

MIT
