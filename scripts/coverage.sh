#!/usr/bin/env bash
# coverage.sh — run Go tests with coverage and record a history snapshot.
#
# Outputs (under webui/coverage/):
#   coverage.out   - merged statement coverage profile (ignored by git)
#   coverage.html  - browsable per-line report (ignored by git)
#   history.csv    - append-only snapshot log (tracked in git)
#
# history.csv is long-format: one row per scope per run, where scope is
# either "total" or a package import path. This keeps the schema stable
# as packages come and go.
set -euo pipefail

cd "$(dirname "$0")/.."

COVDIR=coverage
PROFILE="$COVDIR/coverage.out"
HISTORY="$COVDIR/history.csv"
mkdir -p "$COVDIR"

commit=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
dirty=""
[ -z "$(git status --porcelain -- . 2>/dev/null)" ] || dirty="+dirty"
timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)

echo "== go test ./... (coverage) =="
go test ./... -count=1 -covermode=atomic -coverprofile="$PROFILE"

go tool cover -html="$PROFILE" -o "$COVDIR/coverage.html"

total=$(go tool cover -func="$PROFILE" | awk '/^total:/ {gsub(/%/,"",$3); print $3}')

[ -f "$HISTORY" ] || echo "timestamp,commit,branch,scope,statements_pct" > "$HISTORY"

# Statement-weighted per-package coverage straight from the raw profile
# (block format: file.go:start,end numstmts hitcount).
awk -v ts="$timestamp" -v c="$commit$dirty" -v b="$branch" '
  NR > 1 {
    split($1, a, ":")
    file = a[1]
    n = split(file, segs, "/")
    pkg = file
    sub("/" segs[n] "$", "", pkg)
    tot[pkg] += $2
    if ($3 > 0) cov[pkg] += $2
  }
  END {
    for (p in tot)
      printf "%s,%s,%s,%s,%.1f\n", ts, c, b, p, 100 * cov[p] / tot[p]
  }' "$PROFILE" | sort -t, -k4 >> "$HISTORY"

echo "$timestamp,$commit$dirty,$branch,total,$total" >> "$HISTORY"

echo
echo "== summary =="
echo "total statement coverage: ${total}%"
echo "profile:  $PROFILE"
echo "html:     $COVDIR/coverage.html"
echo "history:  $HISTORY ($(($(wc -l < "$HISTORY") - 1)) rows)"
