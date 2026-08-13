#!/bin/sh
set -eu

unformatted=$(find . -name '*.go' -not -path './dist/*' -exec gofmt -l {} +)
if [ -n "$unformatted" ]; then
  echo "The following Go files are not gofmt-formatted:" >&2
  echo "$unformatted" >&2
  exit 1
fi
