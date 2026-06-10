#!/bin/bash
# Interactive test session for clitest dogfood/integration cases.
# Usage: ./bin/test-session.sh dogfood/version

set -e

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [ -z "$1" ]; then
	echo "Usage: make test-session DIR=dogfood/version"
	echo ""
	echo "Available test directories:"
	find tests -name act.sh | while read -r f; do
		dir=$(dirname "$f")
		echo "  ${dir#tests/}"
	done | sort -u
	exit 1
fi

TEST_DIR="tests/$1"
if [ ! -d "$TEST_DIR" ]; then
	echo "Error: Test directory '$TEST_DIR' does not exist"
	exit 1
fi

if [ ! -x "./clitest" ]; then
	echo "Error: ./clitest not found; run 'make build' first"
	exit 1
fi

exec ./clitest -session -dir "$TEST_DIR"
