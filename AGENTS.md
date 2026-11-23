# Project Rules & Context (`desktop-pet-tanjiro`)

## Overview
- Pure Go, zero external Go dependency Windows 64-bit desktop pet overlay (`desktop-pet-tanjiro.exe`).
- Repository: `https://github.com/clover4282/desktop-pet-tanjiro.git` (`main` branch).
- Always reply concisely to the user in **Korean**.

## Standing User Rules (MUST FOLLOW)
1. **Paid API Cost Guard**:
   - **NEVER** incur paid API charges beyond free quota (especially `image-generator` / `gemini-3.1-flash-image`).
   - If any call fails with `429 RESOURCE_EXHAUSTED` or requires paid billing, **STOP immediately and ask the user** before continuing (offer to wait for quota reset or confirm with the user first).
2. **Auto-Build & Auto-Release on Every Build**:
   - Whenever code or assets are modified and built, **automatically bump semver (`VERSION`) and publish a GitHub Release** via `./scripts/release.sh patch "<notes>"` (pass `minor` or `major` only when requested).
   - Keep the output binary filename `desktop-pet-tanjiro.exe` (and `InternalName` / `OriginalFilename` in `cmd/chibipet/app.rc`).
   - `./scripts/release.sh` requires a clean tracked tree on `main`, so **always commit code/asset changes first**, then run `./scripts/release.sh patch "<multiline notes>"`.

## Character Roster (9 Characters × 7 Poses)
Each character has 7 embedded `160x160` PNG sprites (`idle`, `grabbed`, `react`, `walk1`, `walk2`, `climb1`, `climb2`) in `internal/pet/sprites/`:
- `manggom` (망곰 — cream-yellow hand-drawn doodle baby bear in sky-blue patterned onesie; opening partner with Tanjiro; app/tray icon mascot)
- `tanjiro` (카마도 탄지로)
- `kyojuro` (렌고쿠 쿄쥬로)
- `zenitsu` (아가츠마 젠이츠)
- `inosuke` (하시비라 이노스케)
- `akaza` (아카자)
- `yakgwa_boy` (약과군 — blue bow tie) & `yakgwa_girl` (약과양 — red head bow)
- `mashimaro` (엽기토끼 / 마시마로 — deadpan slit eyes & red plunger)
*(Note: Original Chiikawa trio was intentionally removed by user request.)*

## Architecture & Key Features
- `cmd/chibipet/`: Win32 layered transparent overlay (`UpdateLayeredWindow`, `UpdateLayeredWindow`-backed `dibSurface` with auto-shrink), system tray & right-click menu (`50%/85%/100%/125%/150%` scales), Per-Monitor V2 DPI (`WM_DPICHANGED`), Z-order window detection (`EnumWindows` + `DWMWA_EXTENDED_FRAME_BOUNDS`, excluding `WS_EX_LAYERED` overlays), default-on Windows boot auto-start (`HKCU\Software\Microsoft\Windows\CurrentVersion\Run\DesktopPetTanjiro` + opt-out pref in `HKCU\Software\DesktopPetTanjiro\AutoStart`).
- `internal/pet/`:
  - `sprites.go`: `Catalog`, `//go:embed sprites/*.png`, `PremulSprite` pre-conversion on load + `IdleNRGBA` fallback icon.
  - `render.go`: `PremulSprite` software blitting (`angle == 0` fast-path + rotated path), `RenderBounds` (handles rotated fall bounds), `HitTest` (AABB pre-reject + alpha check), FX/particles.
  - `world.go`: Pet physics, intro sequence, click/hover rubbing, character-specific fall/landing FX, head-stacking (`isStackableHost`, auto hop-off after 7–15s), Z-order occluded window climbing & jumping (`StateClimbUp`, `StateMantle`, `StateJumpPrep`, `StateJumpArc`).
  - `world_test.go`: Regression tests covering stacking, Z-order occlusion, window maximize/move, rotated fall bounds, and all 9 characters.
- `cmd/spriteproc/` & `cmd/mkicon/`:
  - Sprite cutter: `go run ./cmd/spriteproc -in assets/raw/<sheet>.jpg -out internal/pet/sprites -prefix <id> -poses ... -bg green` (use `-no-interior-green` when keeping green details inside outlines like Mang-gom's pajama dots; `-clean-top-string` removes dangling top strings in pose 2).
  - Icon generator: `go run ./cmd/mkicon internal/pet/sprites/manggom_react.png assets/app_icon.png` then `magick assets/app_icon.png -define icon:auto-resize=256,128,64,48,32,16 assets/app_icon.ico`.

## Verification & Release Commands
```bash
go test -count=1 -v ./...
GOOS=windows GOARCH=amd64 go vet ./...
GOOS=windows GOARCH=amd64 go build -o /dev/null ./cmd/chibipet

# After committing tracked edits on main:
./scripts/release.sh patch "### Release Notes..."
```
