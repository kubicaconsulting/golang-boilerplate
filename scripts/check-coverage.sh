#!/bin/sh
set -eu

profile=${1:-coverage/coverage.out}
minimum=$(cat .coverage-threshold)

# Use statement counts rather than rounded function percentages or package averages.
LC_ALL=C awk -v minimum="$minimum" '
BEGIN {
  if (minimum !~ /^[0-9]+([.][0-9]+)?$/ || minimum < 0 || minimum > 100) {
    invalid = 1
    print "Coverage threshold must be a number between 0 and 100." > "/dev/stderr"
    exit 1
  }
}
NR == 1 {
  if ($0 !~ /^mode: (set|count|atomic)$/) {
    print "Invalid coverage profile header." > "/dev/stderr"
    invalid = 1
    exit 1
  }
  next
}
{
  if (NF != 3 || $2 !~ /^[0-9]+$/ || $3 !~ /^[0-9]+$/) {
    print "Invalid coverage profile entry." > "/dev/stderr"
    invalid = 1
    exit 1
  }
  # Cross-package instrumentation can repeat a block for several test binaries.
  if ($1 in blocks && blocks[$1] != $2) {
    print "Conflicting statement counts for a coverage block." > "/dev/stderr"
    invalid = 1
    exit 1
  }
  blocks[$1] = $2
  if ($3 > 0) hits[$1] = 1
}
END {
  if (invalid) exit 1
  for (block in blocks) {
    statements += blocks[block]
    if (hits[block]) covered += blocks[block]
  }
  if (statements == 0) {
    print "Coverage profile contains no statements." > "/dev/stderr"
    exit 1
  }
  percentage = 100 * covered / statements
  printf "Statement coverage: %.2f%% (%d/%d); required: %.2f%%\n", percentage, covered, statements, minimum
  if (100 * covered < minimum * statements) {
    print "Coverage is below the required threshold." > "/dev/stderr"
    exit 1
  }
}
' "$profile"
