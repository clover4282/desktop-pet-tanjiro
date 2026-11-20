#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BUMP="${1:-patch}"
NOTES="${2:-}"

case "$BUMP" in
  patch|minor|major) ;;
  *)
    echo "Unknown bump type: $BUMP (use patch|minor|major)" >&2
    exit 1
    ;;
esac

# Preflight: verify git branch, clean working tree, and required tools before mutating any files
CUR_BRANCH="$(git rev-parse --abbrev-ref HEAD)"
if [[ "$CUR_BRANCH" != "main" ]]; then
  echo "Error: releases must be run from 'main' (currently on '${CUR_BRANCH}')" >&2
  exit 1
fi
if [[ -n "$(git status --porcelain --untracked-files=no)" ]]; then
  echo "Error: working tree has uncommitted changes; commit or stash them before releasing." >&2
  exit 1
fi

for cmd in git go perl gh; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Error: required tool '$cmd' not found in PATH" >&2
    exit 1
  fi
done

WINDRES="${WINDRES:-}"
if [[ -z "$WINDRES" ]]; then
  if command -v x86_64-w64-mingw32-windres >/dev/null 2>&1; then
    WINDRES="x86_64-w64-mingw32-windres"
  elif command -v windres >/dev/null 2>&1; then
    WINDRES="windres"
  else
    echo "Error: neither 'x86_64-w64-mingw32-windres' nor 'windres' found in PATH (set WINDRES to override)" >&2
    exit 1
  fi
elif ! command -v "$WINDRES" >/dev/null 2>&1; then
  echo "Error: WINDRES='$WINDRES' not found in PATH" >&2
  exit 1
fi

if ! gh auth status >/dev/null 2>&1; then
  echo "Error: gh CLI is not authenticated (run 'gh auth login')" >&2
  exit 1
fi

CUR_VER="1.0.0"
if [[ -f VERSION ]]; then
  CUR_VER="$(tr -d '[:space:]' < VERSION)"
fi

if [[ ! "$CUR_VER" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
  echo "Error: invalid VERSION format '${CUR_VER}' (expected MAJOR.MINOR.PATCH)" >&2
  exit 1
fi
MAJ="${BASH_REMATCH[1]}"
MIN="${BASH_REMATCH[2]}"
PAT="${BASH_REMATCH[3]}"

tag_exists() {
  local t="$1"
  git rev-parse -q --verify "refs/tags/${t}" >/dev/null 2>&1 \
    || [[ -n "$(git ls-remote --tags origin "refs/tags/${t}")" ]]
}

# If tag v<CUR_VER> already exists locally or on origin, auto-increment according to BUMP (default: patch).
if tag_exists "v${CUR_VER}"; then
  case "$BUMP" in
    major) MAJ=$((MAJ + 1)); MIN=0; PAT=0 ;;
    minor) MIN=$((MIN + 1)); PAT=0 ;;
    patch) PAT=$((PAT + 1)) ;;
  esac
fi

NEW_VER="${MAJ}.${MIN}.${PAT}"
TAG="v${NEW_VER}"
if tag_exists "${TAG}"; then
  echo "Error: tag ${TAG} already exists locally or on origin" >&2
  exit 1
fi

WIN_VER_COMMA="${MAJ},${MIN},${PAT},0"
WIN_VER_DOT="${MAJ}.${MIN}.${PAT}.0"

echo "==> Releasing desktop-pet-tanjiro ${TAG}"
echo "$NEW_VER" > VERSION

# Sync version into Windows manifest & PE resource script (portable across macOS & Linux)
perl -pi -e "s/version=\"[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+\"/version=\"${WIN_VER_DOT}\"/" cmd/chibipet/app.manifest
perl -pi -e "
  s/(FILEVERSION\s+)[0-9]+,[0-9]+,[0-9]+,[0-9]+/\${1}${WIN_VER_COMMA}/;
  s/(PRODUCTVERSION\s+)[0-9]+,[0-9]+,[0-9]+,[0-9]+/\${1}${WIN_VER_COMMA}/;
  s/(\"FileVersion\",\s+)\"[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+\"/\${1}\"${WIN_VER_DOT}\"/;
  s/(\"ProductVersion\",\s+)\"[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+\"/\${1}\"${WIN_VER_DOT}\"/;
" cmd/chibipet/app.rc

# Recompile Windows PE resource (.syso) with embedded icon, manifest & updated version info
(cd cmd/chibipet && "$WINDRES" -F pe-x86-64 -i app.rc -o rsrc_windows_amd64.syso)

# Run uncached tests & Windows static checks
go test -count=1 -v ./...
GOOS=windows GOARCH=amd64 go vet ./...

# Build standalone Windows 64-bit GUI executable
mkdir -p dist
OUT_EXE="dist/desktop-pet-tanjiro.exe"
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-H=windowsgui -s -w" -o "$OUT_EXE" ./cmd/chibipet

# Stage only release metadata files
git add VERSION cmd/chibipet/app.manifest cmd/chibipet/app.rc cmd/chibipet/rsrc_windows_amd64.syso
if ! git diff --cached --quiet; then
  git commit -m "Release ${TAG}"
fi
git tag -a "${TAG}" -m "Release ${TAG}"
git push origin main
git push origin "${TAG}"

# Publish GitHub Release with desktop-pet-tanjiro.exe attached
if [[ -n "$NOTES" ]]; then
  gh release create "${TAG}" "$OUT_EXE" \
    --title "desktop-pet-tanjiro ${TAG}" \
    --notes "$NOTES"
else
  gh release create "${TAG}" "$OUT_EXE" \
    --title "desktop-pet-tanjiro ${TAG}" \
    --generate-notes
fi

echo "==> Done! Published ${TAG} (${OUT_EXE})"
