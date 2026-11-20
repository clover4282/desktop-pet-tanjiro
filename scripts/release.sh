#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BUMP="${1:-patch}"
NOTES="${2:-}"

CUR_VER="1.0.0"
if [[ -f VERSION ]]; then
  CUR_VER="$(tr -d '[:space:]' < VERSION)"
fi

IFS='.' read -r MAJ MIN PAT <<< "$CUR_VER"
MAJ="${MAJ:-1}"
MIN="${MIN:-0}"
PAT="${PAT:-0}"

# If tag v<CUR_VER> already exists, auto-increment according to BUMP (default: patch).
if git rev-parse -q --verify "refs/tags/v${CUR_VER}" >/dev/null 2>&1; then
  case "$BUMP" in
    major) MAJ=$((MAJ + 1)); MIN=0; PAT=0 ;;
    minor) MIN=$((MIN + 1)); PAT=0 ;;
    patch) PAT=$((PAT + 1)) ;;
    *)
      echo "Unknown bump type: $BUMP (use patch|minor|major)" >&2
      exit 1
      ;;
  esac
fi

NEW_VER="${MAJ}.${MIN}.${PAT}"
TAG="v${NEW_VER}"
WIN_VER_COMMA="${MAJ},${MIN},${PAT},0"
WIN_VER_DOT="${MAJ}.${MIN}.${PAT}.0"

echo "==> Releasing ChibiDesktopPet ${TAG}"
echo "$NEW_VER" > VERSION

# Sync version into Windows manifest & PE resource script
sed -i '' -E "s/version=\"[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+\"/version=\"${WIN_VER_DOT}\"/" cmd/chibipet/app.manifest
sed -i '' -E \
  -e "s/FILEVERSION     [0-9]+,[0-9]+,[0-9]+,[0-9]+/FILEVERSION     ${WIN_VER_COMMA}/" \
  -e "s/PRODUCTVERSION  [0-9]+,[0-9]+,[0-9]+,[0-9]+/PRODUCTVERSION  ${WIN_VER_COMMA}/" \
  -e "s/\"FileVersion\",      \"[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+\"/\"FileVersion\",      \"${WIN_VER_DOT}\"/" \
  -e "s/\"ProductVersion\",   \"[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+\"/\"ProductVersion\",   \"${WIN_VER_DOT}\"/" \
  cmd/chibipet/app.rc

# Recompile Windows PE resource (.syso) with embedded icon, manifest & updated version info
(cd cmd/chibipet && x86_64-w64-mingw32-windres -F pe-x86-64 -i app.rc -o rsrc_windows_amd64.syso)

# Run tests & Windows static checks
go test -v ./...
GOOS=windows GOARCH=amd64 go vet ./...

# Build standalone Windows 64-bit GUI executable
mkdir -p dist
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-H=windowsgui -s -w" -o dist/ChibiDesktopPet.exe ./cmd/chibipet

# Commit, tag, and push
git add -A
if ! git diff --cached --quiet; then
  git commit -m "Release ${TAG}"
fi
git tag -a "${TAG}" -m "Release ${TAG}"
git push origin main
git push origin "${TAG}"

# Publish GitHub Release with ChibiDesktopPet.exe attached
if [[ -n "$NOTES" ]]; then
  gh release create "${TAG}" dist/ChibiDesktopPet.exe \
    --title "Chibi Desktop Pet ${TAG}" \
    --notes "$NOTES"
else
  gh release create "${TAG}" dist/ChibiDesktopPet.exe \
    --title "Chibi Desktop Pet ${TAG}" \
    --generate-notes
fi

echo "==> Done! Published ${TAG}"
