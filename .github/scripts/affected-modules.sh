#!/usr/bin/env bash
# Prints a compact JSON array of the Go module directories whose tests need to run.
#
# Usage: affected-modules.sh [<base-ref>]
#
#   With no base ref, every module is returned (push to main, manual runs).
#   With a base ref, only modules affected by `git diff origin/<base-ref>...HEAD` are returned:
#     - a module is affected if a file under its directory changed (nested modules own their files);
#     - a module that uses a local `replace` directive is also affected when the replaced module is,
#       since it builds against the working tree rather than a released version;
#     - changes to CI/lint config affect every module;
#     - markdown-only changes affect nothing.
#   An empty array ("[]") means no tests are needed.
#
# Kept compatible with bash 3.2 so it can also be run on macOS.
set -euo pipefail

base_ref="${1:-}"

all_modules=()
while IFS= read -r line; do all_modules+=("$line"); done < <(find . -name go.mod -exec dirname {} \; | sort)

emit() {
  if [ $# -eq 0 ]; then
    echo "[]"
  else
    jq --null-input --compact-output '$ARGS.positional' --args "$@"
  fi
}

if [ -z "$base_ref" ]; then
  emit "${all_modules[@]}"
  exit 0
fi

# Capture the diff in a checked assignment: a failure inside process substitution would be
# swallowed and look like "no changes", letting required checks pass without running tests.
# --no-renames reports a move as a delete plus an add so the source module is affected too.
if ! diff_output=$(git diff --name-only --no-renames "origin/${base_ref}...HEAD"); then
  echo "error: unable to diff against origin/${base_ref}" >&2
  exit 1
fi
changed=()
while IFS= read -r line; do
  if [ -n "$line" ]; then changed+=("$line"); fi
done <<< "$diff_output"

global_re='^(\.github/(workflows|scripts)/|\.golangci\.yml$|\.testcoverage\.yml$|go\.work)'
for f in ${changed[@]+"${changed[@]}"}; do
  if [[ "$f" =~ $global_re ]]; then
    echo "Global change detected ($f); testing all modules." >&2
    emit "${all_modules[@]}"
    exit 0
  fi
done

# owner prints the module directory that owns a file (longest matching prefix).
owner() {
  local file="./$1" best="" m
  for m in "${all_modules[@]}"; do
    if [ "$m" = "." ] || [[ "$file" == "$m/"* ]]; then
      if [ ${#m} -gt ${#best} ]; then best="$m"; fi
    fi
  done
  echo "$best"
}

# affected is a newline-delimited set of module directories.
affected=""
for f in ${changed[@]+"${changed[@]}"}; do
  case "$f" in *.md) continue ;; esac
  affected="${affected}$(owner "$f")"$'\n'
done

# abs prints the physical absolute path of a directory.
abs() { (cd "$1" && pwd -P); }

# A module with a local `replace` builds against that directory's working tree, so it is
# affected when the replaced module is. Single pass: chains of local replaces are not followed.
direct="$affected"
for m in "${all_modules[@]}"; do
  targets=$(grep -Eo '=>[[:space:]]*\.[^[:space:]]*' "$m/go.mod" | sed -E 's/=>[[:space:]]*//' || true)
  for t in $targets; do
    target_abs=$(abs "$m/$t" 2>/dev/null) || continue
    while IFS= read -r a; do
      if [ -n "$a" ] && [ "$(abs "$a")" = "$target_abs" ]; then
        affected="${affected}${m}"$'\n'
      fi
    done <<< "$direct"
  done
done

result=()
for m in "${all_modules[@]}"; do
  if printf '%s' "$affected" | grep -Fxq -- "$m"; then
    result+=("$m")
  fi
done

emit ${result[@]+"${result[@]}"}
