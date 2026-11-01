#!/bin/sh
set -eu

GO=${GO:-go}
: "${MOCKERY_VERSION:?Set MOCKERY_VERSION through make mocks-check}"
cd "$(git rev-parse --show-toplevel)"
temporary=$(mktemp -d)
trap 'rm -rf "$temporary"' EXIT HUP INT TERM

# Include tracked files and new source files, but keep generation away from the working tree.
git ls-files --cached --others --exclude-standard -z > "$temporary/files"
tar -cf "$temporary/source.tar" --null -T "$temporary/files"
mkdir "$temporary/source"
tar -xf "$temporary/source.tar" -C "$temporary/source"
(
  cd "$temporary/source"
  # Remove copied outputs so obsolete mock files are detected too.
  find internal -type f -name mocks_test.go -exec rm {} +
  GOWORK=off "$GO" run "github.com/vektra/mockery/v3@$MOCKERY_VERSION" --config .mockery.yml
  find internal -type f -name mocks_test.go | LC_ALL=C sort > "$temporary/expected"
)
find internal -type f -name mocks_test.go | LC_ALL=C sort > "$temporary/actual"
if ! diff -u "$temporary/actual" "$temporary/expected"; then
  printf '%s\n' 'Generated mock files differ. Run make mocks and review the result.' >&2
  exit 1
fi
while IFS= read -r file; do
  if ! diff -u "$file" "$temporary/source/$file"; then
    printf '%s\n' 'Generated mocks are stale. Run make mocks and review the result.' >&2
    exit 1
  fi
done < "$temporary/actual"
printf '%s\n' 'Generated mocks match their configured interfaces.'
