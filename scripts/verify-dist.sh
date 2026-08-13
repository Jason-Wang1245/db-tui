#!/bin/sh
set -eu

dist=${1:-dist}

archives=$(find "$dist" -maxdepth 1 -type f -name 'db-tui_*_*.tar.gz' | sort)
archive_count=$(printf '%s\n' "$archives" | sed '/^$/d' | wc -l | tr -d ' ')
if [ "$archive_count" -ne 4 ]; then
  echo "found $archive_count release archives, want 4" >&2
  exit 1
fi

for archive in $archives; do
  tar -tzf "$archive" | grep -qx 'db-tui'
  tar -tzf "$archive" | grep -qx 'README.md'
done

checksum_file="$dist/checksums.txt"
test -f "$checksum_file"
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$dist" && sha256sum --check checksums.txt)
else
  (cd "$dist" && shasum -a 256 -c checksums.txt)
fi

if [ "${SKIP_SBOM:-0}" != "1" ]; then
  sbom_count=$(find "$dist" -maxdepth 1 -type f -name '*.sbom.json' | wc -l | tr -d ' ')
  if [ "$sbom_count" -ne 4 ]; then
    echo "found $sbom_count SBOMs, want 4" >&2
    exit 1
  fi
fi
