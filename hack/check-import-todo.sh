#!/usr/bin/env bash
# Fails while code from codegen/cmd/import still needs porting by hand:
# files quarantined behind the importtodo build tag, or TODO(import) markers.
#
# Usage: hack/check-import-todo.sh [root]   (root defaults to the git top level)
set -euo pipefail

root="${1:-$(git rev-parse --show-toplevel)}"
if ! git -C "${root}" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "check-import-todo: ${root} is not a git checkout" >&2
  exit 2
fi
cd "${root}"

# The importer and its tests contain the marker on purpose, so codegen/ is
# skipped. Untracked files count too: a fresh import is not committed yet.
files=()
while IFS= read -r -d '' f; do
  [[ -f "${f}" ]] && files+=("${f}")
done < <(git ls-files -z -co --exclude-standard -- '*.go' ':(exclude)codegen/**')

found=0
for f in "${files[@]+"${files[@]}"}"; do
  case "${f}" in
    *_importtodo.go | *_importtodo_test.go)
      echo "${f}: quarantined by codegen/cmd/import (build tag importtodo)"
      found=1
      ;;
  esac
done
if ((${#files[@]} > 0)); then
  rc=0
  hits="$(grep -HnF -- 'TODO(import)' "${files[@]}")" || rc=$?
  # grep exits 1 when nothing matches, and 2 when it could not read a file.
  if ((rc > 1)); then
    echo "check-import-todo: cannot read every Go file" >&2
    exit 2
  fi
  while IFS= read -r hit; do
    [[ -n "${hit}" ]] || continue
    file="${hit%%:*}"
    rest="${hit#*:}"
    line="${rest%%:*}"
    text="${rest#*:}"
    text="${text#"${text%%[![:space:]]*}"}"
    echo "${file}:${line}: ${text}"
    found=1
  done <<<"${hits}"
fi

if ((found)); then
  echo "check-import-todo: code left by codegen/cmd/import still needs porting, see .agents/skills/import-kind/SKILL.md step 3" >&2
  exit 1
fi
