#!/bin/sh
set -e
here=$(dirname "$0")
clitest -config "$here/files/glob.yml"
