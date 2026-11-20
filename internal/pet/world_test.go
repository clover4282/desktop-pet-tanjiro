package pet

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAndSimulateClipScenes(t *testing.T) {
	bank, err := LoadSpriteBank()
	if err != nil {
		t.Fatalf("LoadSpriteBank failed: %v", err)
	}
	if len(bank.List) != 6 {
		t.Fatalf("expected 6 characters (no Chiikawa), got %d", len(bank.List))
	}
	for _, removed := range []string{"chiikawa", "usagi", "rakko"} {
		if _, ok := bank.ByID[removed]; ok {
			t.Fatalf("expected %s to be removed from catalog", removed)
		}
	}

	screen := image.Rect(0, 0, 1280, 720)
	workArea := image.Rect(0, 0, 1280, 672)
	w := NewWorld(bank, workArea, screen, 1.0)
	w.FreeRoam = false

	// 1. Intro: Mang-gom sits on taskbar, propeller claw drops Tanjiro onto Mang-gom's head
	for i := 0; i < 65; i++ {
		w.Step(1.0 / 40.0)
	}
	if len(w.Pets) != 2 || w.Pets[0].CharID != "manggom" || w.Pets[1].CharID != "tanjiro" {
		t.Fatalf("expected manggom + tanjiro intro, got %d pets", len(w.Pets))
	}

	// 2. Step until Tanjiro lands & stacks on Mang-gom
	for i := 0; i < 80; i++ {
		w.Step(1.0 / 40.0)
	}
	tanjiro := w.Pets[1]
	manggom := w.Pets[0]
	if tanjiro.State != StateStacked || tanjiro.StackParentID != manggom.ID {
		t.Fatalf("expected Tanjiro stacked on Mang-gom, got state=%v parent=%d", tanjiro.State, tanjiro.StackParentID)
	}

	// 3. Click Tanjiro while stacked -> Water Breathing reaction!
	hitID := w.HitTest(tanjiro.X, tanjiro.Y-45)
	if hitID != tanjiro.ID {
		t.Fatalf("expected HitTest to hit Tanjiro (%d), got %d", tanjiro.ID, hitID)
	}
	w.MouseDown(tanjiro.X, tanjiro.Y-45)
	w.MouseUp(tanjiro.X, tanjiro.Y-45)
	w.TriggerReact(manggom.ID)
	for i := 0; i < 8; i++ {
		w.Step(1.0 / 40.0)
	}
	if tanjiro.ReactTimer <= 0 || manggom.ReactTimer <= 0 {
		t.Fatalf("expected both stacked pets reacting")
	}

	// 4. Duplicate summon prevention!
	dup := w.SpawnWithClaw("tanjiro", 0, 0)
	if len(w.Pets) != 2 || dup.ID != tanjiro.ID {
		t.Fatalf("expected duplicate tanjiro summon blocked, got len=%d", len(w.Pets))
	}

	// 5. Summon remaining characters -> total should be 6 (no duplicates)
	w.SpawnAll()
	if len(w.Pets) != 6 {
		t.Fatalf("expected 6 unique pets after SpawnAll, got %d", len(w.Pets))
	}
	if w.SpawnRandomWithClaw() != nil {
		t.Fatalf("expected SpawnRandomWithClaw to return nil when all 6 are already out")
	}

	// 6. Drag & stack Akaza onto Kyojuro, then dismiss Kyojuro -> Akaza lands safely
	for i := 0; i < 200; i++ {
		w.Step(1.0 / 40.0)
	}
	kyojuro := w.FindPetByChar("kyojuro")
	akaza := w.FindPetByChar("akaza")
	w.MouseDown(akaza.X, akaza.Y-45)
	w.MouseMove(kyojuro.X, w.HeadTopY(kyojuro)-20)
	w.MouseUp(kyojuro.X, w.HeadTopY(kyojuro)-20)
	if akaza.State != StateStacked || akaza.StackParentID != kyojuro.ID {
		t.Fatalf("expected Akaza stacked on Kyojuro, got state=%v", akaza.State)
	}
	w.DismissPet(kyojuro.ID)
	for i := 0; i < 160; i++ {
		w.Step(1.0 / 40.0)
	}
	if w.FindPet(kyojuro.ID) != nil {
		t.Fatalf("expected Kyojuro dismissed")
	}
	if akaza.State != StateIdle && akaza.State != StateWalk {
		t.Fatalf("expected Akaza to land safely, got state=%v", akaza.State)
	}
}

func TestCharacterSpecificFallAndRubbing(t *testing.T) {
	bank, err := LoadSpriteBank()
	if err != nil {
		t.Fatalf("LoadSpriteBank failed: %v", err)
	}
	screen := image.Rect(0, 0, 1280, 720)
	workArea := image.Rect(0, 0, 1280, 672)
	w := NewWorld(bank, workArea, screen, 1.0)
	w.Pets = nil
	w.introSpawnDone = true
	w.FreeRoam = false

	for i, def := range Catalog {
		x := 140.0 + float64(i)*190.0
		p := w.newPet(def.ID, x, 190.0)
		w.Pets = append(w.Pets, p)
		w.startFall(p, 0, 0)
	}
	for i := 0; i < 110; i++ {
		w.Step(1.0 / 40.0)
	}
	for _, p := range w.Pets {
		if p.State != StateIdle {
			t.Fatalf("expected %s to settle in StateIdle, got %v", p.CharID, p.State)
		}
	}

	// Test rubbing on all 6 characters
	for _, p := range w.Pets {
		cy := p.Y - 68
		w.MouseHover(p.X, cy)
		for stroke := 0; stroke < 14; stroke++ {
			dx := 24.0
			if stroke%2 == 1 {
				dx = -24.0
			}
			w.MouseHover(p.X+dx, cy)
			w.Step(1.0 / 40.0)
		}
		if p.RubMeter < 0.6 {
			t.Fatalf("expected %s RubMeter >= 0.6 after rubbing, got %.2f", p.CharID, p.RubMeter)
		}
	}
}

func TestWindowParkourLeftRightWallsAndNoClimbWithStack(t *testing.T) {
	bank, err := LoadSpriteBank()
	if err != nil {
		t.Fatalf("LoadSpriteBank failed: %v", err)
	}
	screen := image.Rect(0, 0, 1280, 720)
	workArea := image.Rect(0, 0, 1280, 672)
	w := NewWorld(bank, workArea, screen, 1.0)
	w.Pets = nil
	w.introSpawnDone = true
	w.FreeRoam = true

	win := WindowRect{ID: 101, Rect: image.Rect(340, 220, 940, 620)}
	w.SetWindows([]WindowRect{win})

	// 1. Tanjiro climbs LEFT edge of window (wall on his RIGHT -> FacingLeft == false!)
	tanjiro := w.newPet("tanjiro", 300, 560)
	w.Pets = append(w.Pets, tanjiro)
	w.StartClimbWindow(tanjiro.ID, win.ID, true)
	if tanjiro.FacingLeft {
		t.Fatalf("climbing left edge of window (wall to right) must face RIGHT (FacingLeft=false)")
	}

	// 2. Inosuke climbs RIGHT edge of window (wall on his LEFT -> FacingLeft == true!)
	inosuke := w.newPet("inosuke", 980, 520)
	w.Pets = append(w.Pets, inosuke)
	w.StartClimbWindow(inosuke.ID, win.ID, false)
	if !inosuke.FacingLeft {
		t.Fatalf("climbing right edge of window (wall to left) must face LEFT (FacingLeft=true)")
	}

	// 3. Kyojuro walks in side-profile along top of window
	kyojuro := w.newPet("kyojuro", 520, float64(win.Rect.Min.Y))
	kyojuro.AttachedWinID = win.ID
	w.Pets = append(w.Pets, kyojuro)
	w.startWalk(kyojuro, 1.0)

	// 4. Zenitsu jumps OFF the top-right edge of the window!
	zenitsu := w.newPet("zenitsu", 890, float64(win.Rect.Min.Y))
	zenitsu.AttachedWinID = win.ID
	w.Pets = append(w.Pets, zenitsu)
	w.StartJumpOff(zenitsu.ID, 1.0)

	// 5. Mang-gom carries Akaza on his head -> must NOT climb walls or jump off!
	manggom := w.newPet("manggom", 150, float64(workArea.Max.Y))
	akaza := w.newPet("akaza", 150, float64(workArea.Max.Y)-100)
	w.Pets = append(w.Pets, manggom, akaza)
	w.attachStack(akaza, manggom)
	w.StartClimbWindow(manggom.ID, win.ID, true)
	if manggom.State == StateClimbUp {
		t.Fatalf("Mang-gom must NOT climb wall while Akaza is on his head")
	}

	for i := 0; i < 16; i++ {
		w.Step(1.0 / 40.0)
	}
	saveMockup(t, w, "scene_final_showcase.png")

	// Verify window movement & closing
	kyojuro.State = StateIdle
	kyojuro.AttachedWinID = win.ID
	kyojuro.X = 600
	kyojuro.Y = float64(win.Rect.Min.Y)
	w.SetWindows([]WindowRect{{ID: 101, Rect: image.Rect(390, 260, 990, 660)}})
	if kyojuro.X != 650 || kyojuro.Y != 260 {
		t.Fatalf("expected Kyojuro to ride moved window, got (%.1f,%.1f)", kyojuro.X, kyojuro.Y)
	}
	w.SetWindows(nil)
	if kyojuro.State != StateFall || kyojuro.AttachedWinID != 0 {
		t.Fatalf("expected Kyojuro to fall when window closed")
	}
}

func TestNoStackLoopOnClimbingPet(t *testing.T) {
	bank, err := LoadSpriteBank()
	if err != nil {
		t.Fatalf("LoadSpriteBank failed: %v", err)
	}
	screen := image.Rect(0, 0, 1280, 720)
	workArea := image.Rect(0, 0, 1280, 672)
	w := NewWorld(bank, workArea, screen, 1.0)
	w.Pets = nil
	w.introSpawnDone = true

	win := WindowRect{ID: 101, Rect: image.Rect(200, 140, 800, 620)}
	w.SetWindows([]WindowRect{win})

	// Reproduce exact user screenshot: Mang-gom climbing right window edge while Zenitsu drops onto his head!
	manggom := w.newPet("manggom", 830, 540)
	w.Pets = append(w.Pets, manggom)
	w.StartClimbWindow(manggom.ID, win.ID, false)

	zenitsu := w.newPet("zenitsu", manggom.X, manggom.Y-110)
	w.Pets = append(w.Pets, zenitsu)
	w.startFall(zenitsu, 0, 0)

	// Dropping directly onto climbing Mang-gom must NOT attach or loop-spam lightning particles!
	maxParticles := 0
	for i := 0; i < 60; i++ {
		w.Step(1.0 / 40.0)
		if zenitsu.State == StateStacked && zenitsu.StackParentID == manggom.ID && manggom.State == StateClimbUp {
			t.Fatalf("Zenitsu must NOT stack onto Mang-gom while Mang-gom is climbing!")
		}
		if len(w.Particles) > maxParticles {
			maxParticles = len(w.Particles)
		}
	}
	if maxParticles > 35 {
		t.Fatalf("unexpected particle explosion during fall past climbing pet: maxParticles=%d", maxParticles)
	}
}

func TestWindowZOrderOcclusion(t *testing.T) {
	bank, err := LoadSpriteBank()
	if err != nil {
		t.Fatalf("LoadSpriteBank failed: %v", err)
	}
	screen := image.Rect(0, 0, 1280, 720)
	workArea := image.Rect(0, 0, 1280, 672)
	w := NewWorld(bank, workArea, screen, 1.0)
	w.Pets = nil
	w.introSpawnDone = true

	// Case 1: Foreground Explorer (idx 0) covers the lower half of Background Edge (idx 1)
	frontExplorer := WindowRect{ID: 1, Rect: image.Rect(200, 320, 1100, 670)}
	backEdge := WindowRect{ID: 2, Rect: image.Rect(50, 140, 500, 660)}
	w.SetWindows([]WindowRect{frontExplorer, backEdge})

	akaza := w.newPet("akaza", 560, float64(workArea.Max.Y))
	akaza.VX = -80
	w.Pets = append(w.Pets, akaza)

	// Crossing backEdge's right wall (x=500 + 0.24*132 ≈ 532) at taskbar Y=672 MUST be blocked because frontExplorer covers it!
	if win, _, hit := w.findWindowWallHit(akaza, 560, 510); hit && win.ID == backEdge.ID {
		t.Fatalf("must NOT climb background Edge wall occluded by foreground Explorer")
	}

	// Case 1b: Standing ON TOP of frontExplorer (Y=320), backEdge's upper right wall (Y=140..320) IS visible above Explorer -> MUST be climbable!
	akazaOnExplorer := w.newPet("akaza", 560, float64(frontExplorer.Rect.Min.Y))
	akazaOnExplorer.AttachedWinID = frontExplorer.ID
	akazaOnExplorer.VX = -80
	if win, faceDir, hit := w.findWindowWallHit(akazaOnExplorer, 560, 510); !hit || win.ID != backEdge.ID || faceDir != -1.0 {
		t.Fatalf("expected Akaza on Explorer titlebar to climb visible upper right wall of backEdge, got hit=%v win=%d dir=%.1f", hit, win.ID, faceDir)
	}

	// Case 2: Maximized foreground window (Occluding=true) covers everything behind it
	maxChrome := WindowRect{ID: 10, Rect: workArea, Occluding: true}
	hiddenNotepad := WindowRect{ID: 11, Rect: image.Rect(300, 200, 900, 640)}
	w.SetWindows([]WindowRect{maxChrome, hiddenNotepad})

	if _, _, hit := w.findWindowWallNear(300, 400, 50); hit {
		t.Fatalf("must NOT grab wall of Notepad hidden behind maximized Chrome")
	}
	akaza.X = 500
	if _, ok := w.findLandingWindowTop(akaza, 180, 210); ok {
		t.Fatalf("must NOT land on Notepad titlebar hidden behind maximized Chrome")
	}
}

func TestStackedPetAutoHopsOff(t *testing.T) {
	bank, err := LoadSpriteBank()
	if err != nil {
		t.Fatalf("LoadSpriteBank failed: %v", err)
	}
	screen := image.Rect(0, 0, 1280, 720)
	workArea := image.Rect(0, 0, 1280, 672)
	w := NewWorld(bank, workArea, screen, 1.0)
	w.Pets = nil
	w.introSpawnDone = true
	w.FreeRoam = true

	manggom := w.newPet("manggom", 900, float64(workArea.Max.Y))
	tanjiro := w.newPet("tanjiro", 900, float64(workArea.Max.Y)-100)
	w.Pets = append(w.Pets, manggom, tanjiro)
	w.attachStack(tanjiro, manggom)
	tanjiro.StateTimer = 0.25 // speed up wait for test

	// Step through timer expiry -> jump prep -> jump arc -> fall -> land on taskbar cleanly!
	for i := 0; i < 120; i++ {
		w.Step(1.0 / 40.0)
	}
	if tanjiro.StackParentID != 0 {
		t.Fatalf("expected Tanjiro to hop off Mang-gom's head automatically, still stacked")
	}
	if tanjiro.State != StateIdle && tanjiro.State != StateWalk {
		t.Fatalf("expected Tanjiro landed safely after hopping off, got state=%v", tanjiro.State)
	}
}

func saveMockup(t *testing.T, w *World, filename string) {
	t.Helper()
	outDir := os.Getenv("PET_PREVIEW_DIR")
	if outDir == "" {
		return
	}
	canvas := image.NewRGBA(w.Screen)
	for y := 0; y < w.Screen.Dy(); y++ {
		for x := 0; x < w.Screen.Dx(); x++ {
			if y >= w.WorkArea.Max.Y {
				c := color.RGBA{32, 36, 44, 255}
				if y == w.WorkArea.Max.Y {
					c = color.RGBA{68, 74, 86, 255}
				}
				canvas.SetRGBA(x, y, c)
			} else {
				tY := float64(y) / float64(w.WorkArea.Max.Y)
				r := uint8(46 + tY*34)
				g := uint8(58 + tY*36)
				b := uint8(78 + tY*42)
				canvas.SetRGBA(x, y, color.RGBA{r, g, b, 255})
			}
		}
	}

	for _, win := range w.Windows {
		r := win.Rect.Intersect(w.WorkArea)
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				var c color.RGBA
				if y == r.Min.Y || y == r.Max.Y-1 || x == r.Min.X || x == r.Max.X-1 {
					c = color.RGBA{120, 130, 150, 255}
				} else if y < r.Min.Y+32 {
					c = color.RGBA{232, 236, 244, 255}
				} else {
					c = color.RGBA{248, 250, 252, 255}
				}
				canvas.SetRGBA(x, y, c)
			}
		}
	}

	overlay := w.RenderRGBA(w.Screen)
	for y := 0; y < w.Screen.Dy(); y++ {
		for x := 0; x < w.Screen.Dx(); x++ {
			s := overlay.RGBAAt(x, y)
			if s.A == 0 {
				continue
			}
			d := canvas.RGBAAt(x, y)
			inv := 1.0 - float64(s.A)/255.0
			canvas.SetRGBA(x, y, color.RGBA{
				R: clampByte(float64(s.R) + float64(d.R)*inv),
				G: clampByte(float64(s.G) + float64(d.G)*inv),
				B: clampByte(float64(s.B) + float64(d.B)*inv),
				A: 255,
			})
		}
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	f, err := os.Create(filepath.Join(outDir, filename))
	if err != nil {
		t.Fatalf("create %s: %v", filename, err)
	}
	defer f.Close()
	if err := png.Encode(f, canvas); err != nil {
		t.Fatalf("encode %s: %v", filename, err)
	}
}
