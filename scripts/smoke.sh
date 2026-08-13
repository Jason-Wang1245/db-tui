#!/bin/sh
set -eu

binary=${1:-./db-tui}
expected_os=${2:-$(go env GOOS)}
expected_arch=${3:-$(go env GOARCH)}

actual_os=$(go version -m "$binary" | sed -n 's/^[[:space:]]*build[[:space:]]*GOOS=//p')
actual_arch=$(go version -m "$binary" | sed -n 's/^[[:space:]]*build[[:space:]]*GOARCH=//p')

if [ "$actual_os" != "$expected_os" ] || [ "$actual_arch" != "$expected_arch" ]; then
  echo "binary target is $actual_os/$actual_arch, want $expected_os/$expected_arch" >&2
  exit 1
fi

"$binary" --version
"$binary" --help >/dev/null
