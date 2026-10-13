#!/usr/bin/env bash
# Lists mechanical signals for the pre-release retrospective in LESSONS.md:
# large commits without a body, and fix/test/style/perf commits that touch
# code a feat changed shortly before. The signals are leads for a reviewer,
# not verdicts. Usage: scripts/retro-signals.sh [base-ref]; the default base
# is the reviewed-through commit recorded at the top of LESSONS.md.
set -euo pipefail

LARGE_COMMIT_LINES=400
FOLLOWUP_WINDOW_SECONDS=$((24 * 3600))
NEAREST_FEATS=3
CODE_PATHS=(':(glob)**/*.go' ':(glob)**/*.py' ':(glob)**/*.sh')

cd "$(git rev-parse --show-toplevel)"

base=${1:-}
if [[ -z $base ]]; then
	base=$(sed -n 's/^<!-- reviewed-through: \([0-9a-f]*\).*/\1/p' LESSONS.md | head -n 1)
fi
if [[ -z $base ]]; then
	echo "no base: pass a ref or record reviewed-through in LESSONS.md" >&2
	exit 1
fi

echo "## range $base..HEAD ($(git rev-list --count "$base..HEAD") commits)"

# body_lines counts the message body without trailers such as Co-Authored-By.
body_lines() {
	git log -1 --format=%b "$1" | grep -cvE '^$|^[A-Za-z-]+: ' || true
}

echo
echo "## commits over $LARGE_COMMIT_LINES changed lines without a body"
git log --format=%h "$base..HEAD" | while read -r c; do
	changed=$(git show --numstat --format= "$c" | awk '{s += $1 + $2} END {print s + 0}')
	if ((changed > LARGE_COMMIT_LINES)) && (($(body_lines "$c") == 0)); then
		echo "$c ±$changed $(git log -1 --format=%s "$c")"
	fi
done

# feat_before prints the nearest feat commits that touched any of the given
# commit's code files within the follow-up window before it.
feat_before() {
	local c=$1 when files
	when=$(git log -1 --format=%ct "$c")
	mapfile -t files < <(git show --name-only --format= "$c" -- "${CODE_PATHS[@]}")
	((${#files[@]} > 0)) || return 0
	git log --format='%h %ct %s' --since="@$((when - FOLLOWUP_WINDOW_SECONDS))" \
		--until="@$when" "$c~1" -- "${files[@]}" |
		awk -v now="$when" '$3 ~ /^feat/ {printf "  after %s (%.1fh): %s\n", $1, (now - $2) / 3600, substr($0, index($0, $3))}' |
		head -n "$NEAREST_FEATS"
}

echo
echo "## fix/test/style/perf commits following a feat on the same code within 24h"
git log --format='%h %s' "$base..HEAD" | while read -r c subject; do
	[[ $subject =~ ^(fix|test|style|perf) ]] || continue
	follows=$(feat_before "$c")
	if [[ -n $follows ]]; then
		printf '%s %s\n%s\n' "$c" "$subject" "$follows"
	fi
done
