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
	if len(bank.List) != 8 {
		t.Fatalf("expected 8 characters, got %d", len(bank.List))
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

	// 5. Summon remaining characters -> total should be 8 (no duplicates)
	w.SpawnAll()
	if len(w.Pets) != 8 {
		t.Fatalf("expected 8 unique pets after SpawnAll, got %d", len(w.Pets))
	}
	if w.SpawnRandomWithClaw() != nil {
		t.Fatalf("expected SpawnRandomWithClaw to return nil when all 8 are already out")
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
		x := 110.0 + float64(i)*150.0
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

	// Test rubbing on all 8 characters
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

func TestStepStackingNonLastPetNoCorruption(t *testing.T) {
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

	// Place falling pet FIRST in w.Pets so attachStack->bringToFront moves it past B and C.
	host := w.newPet("manggom", 500, float64(workArea.Max.Y))
	falling := w.newPet("tanjiro", 500, w.HeadTopY(host)-18)
	bystander := w.newPet("zenitsu", 900, float64(workArea.Max.Y))
	w.Pets = []*Pet{falling, bystander, host}
	w.startFall(falling, 0, 120)

	for i := 0; i < 30; i++ {
		w.Step(1.0 / 40.0)
	}

	if len(w.Pets) != 3 {
		t.Fatalf("expected 3 pets preserved after non-last pet stacked, got %d", len(w.Pets))
	}
	seen := make(map[int]bool, 3)
	for _, p := range w.Pets {
		if seen[p.ID] {
			t.Fatalf("duplicate pet ID %d in w.Pets after Step stacking", p.ID)
		}
		seen[p.ID] = true
	}
	for _, want := range []int{falling.ID, bystander.ID, host.ID} {
		if !seen[want] {
			t.Fatalf("pet ID %d was lost from w.Pets during Step stacking", want)
		}
	}
	if falling.State != StateStacked || falling.StackParentID != host.ID {
		t.Fatalf("expected falling pet stacked on host, got state=%v parent=%d", falling.State, falling.StackParentID)
	}
}

func TestSetWindowsOccludingAndMantleAndRightWallOcclusion(t *testing.T) {
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

	win := WindowRect{ID: 201, Rect: image.Rect(300, 220, 900, 620)}
	w.SetWindows([]WindowRect{win})

	// 1. Pet standing on window falls immediately when window becomes maximized/Occluding
	kyojuro := w.newPet("kyojuro", 500, float64(win.Rect.Min.Y))
	kyojuro.AttachedWinID = win.ID
	w.Pets = append(w.Pets, kyojuro)

	w.SetWindows([]WindowRect{{ID: 201, Rect: workArea, Occluding: true}})
	if kyojuro.State != StateFall || kyojuro.AttachedWinID != 0 {
		t.Fatalf("expected pet to fall when attached window becomes Occluding, got state=%v win=%d", kyojuro.State, kyojuro.AttachedWinID)
	}
	if kyojuro.Y == float64(workArea.Min.Y) {
		t.Fatalf("pet should not teleport to maximized window top Y=0 before falling")
	}

	// 2. Pet in StateMantle tracks window move and falls if window maximizes or closes mid-mantle
	w.SetWindows([]WindowRect{win})
	tanjiro := w.newPet("tanjiro", float64(win.Rect.Min.X)-w.PetSize()*0.24, float64(win.Rect.Min.Y)+2)
	w.Pets = []*Pet{tanjiro}
	w.StartClimbWindow(tanjiro.ID, win.ID, true)
	for i := 0; i < 10 && tanjiro.State != StateMantle; i++ {
		w.Step(1.0 / 40.0)
	}
	if tanjiro.State != StateMantle {
		t.Fatalf("expected Tanjiro to enter StateMantle at top of window, got state=%v", tanjiro.State)
	}
	oldEndX := tanjiro.MantleEndX
	w.SetWindows([]WindowRect{{ID: 201, Rect: image.Rect(360, 250, 960, 650)}})
	if tanjiro.MantleEndX != oldEndX+60 || tanjiro.MantleTopY != 250 {
		t.Fatalf("expected StateMantle to track moved window, got endX=%.1f (want %.1f) topY=%.1f", tanjiro.MantleEndX, oldEndX+60, tanjiro.MantleTopY)
	}
	w.SetWindows([]WindowRect{{ID: 201, Rect: workArea, Occluding: true}})
	if tanjiro.State != StateFall || tanjiro.AttachedWinID != 0 {
		t.Fatalf("expected mantling pet to fall when window maximizes, got state=%v win=%d", tanjiro.State, tanjiro.AttachedWinID)
	}

	// 3. Right-wall climb drops when a foreground window shares exact Max.X
	backWin := WindowRect{ID: 301, Rect: image.Rect(400, 180, 900, 620)}
	w.SetWindows([]WindowRect{backWin})
	inosuke := w.newPet("inosuke", 930, 500)
	w.Pets = []*Pet{inosuke}
	w.StartClimbWindow(inosuke.ID, backWin.ID, false)
	if inosuke.State != StateClimbUp {
		t.Fatalf("expected Inosuke climbing right wall, got %v", inosuke.State)
	}
	frontSameMaxX := WindowRect{ID: 302, Rect: image.Rect(500, 180, 900, 620)}
	w.SetWindows([]WindowRect{frontSameMaxX, backWin})
	w.Step(1.0 / 40.0)
	if inosuke.State != StateFall || inosuke.AttachedWinID != 0 {
		t.Fatalf("expected right-wall climber to fall when foreground window shares Max.X, got state=%v", inosuke.State)
	}
}

func TestClawSpawnTargetParentUnavailable(t *testing.T) {
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

	win := WindowRect{ID: 401, Rect: image.Rect(300, 180, 900, 620)}
	w.SetWindows([]WindowRect{win})

	// Case A: TargetParent starts climbing before claw arrives -> claw clears TargetParent and lands on floor
	host := w.newPet("manggom", 268, float64(workArea.Max.Y))
	w.Pets = []*Pet{host}
	spawned := w.SpawnWithClaw("tanjiro", host.X, host.ID)
	w.StartClimbWindow(host.ID, win.ID, true)

	for i := 0; i < 120; i++ {
		w.Step(1.0 / 40.0)
	}
	if spawned.StackParentID == host.ID {
		t.Fatalf("spawned pet must NOT stack onto climbing TargetParent")
	}
	if spawned.State != StateIdle || spawned.Y != float64(workArea.Max.Y) {
		t.Fatalf("expected spawned pet to land on floor, got state=%v y=%.1f", spawned.State, spawned.Y)
	}

	// Case B: TargetParent already gains a stack child while claw is descending -> no multi-child fork
	host2 := w.newPet("kyojuro", 700, float64(workArea.Max.Y))
	occupier := w.newPet("akaza", 700, float64(workArea.Max.Y)-100)
	w.Pets = []*Pet{host2, occupier}
	spawned2 := w.SpawnWithClaw("zenitsu", host2.X, host2.ID)
	w.attachStack(occupier, host2)

	for i := 0; i < 120; i++ {
		w.Step(1.0 / 40.0)
	}
	childrenOfHost2 := 0
	for _, p := range w.Pets {
		if p.StackParentID == host2.ID {
			childrenOfHost2++
		}
	}
	if childrenOfHost2 > 1 {
		t.Fatalf("expected at most 1 direct child on host2, got %d (spawned2 parent=%d)", childrenOfHost2, spawned2.StackParentID)
	}
}

func TestRotatedFallRenderBoundsAndRenderBGRA(t *testing.T) {
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

	// Upright pet RenderBGRA sanity check
	p := w.newPet("inosuke", 640, 360)
	w.Pets = []*Pet{p}
	upBounds := w.RenderBounds()
	if upBounds.Empty() {
		t.Fatalf("expected non-empty RenderBounds for upright pet")
	}
	upBuf := make([]byte, upBounds.Dx()*upBounds.Dy()*4)
	w.RenderBGRA(upBuf, upBounds.Dx(), upBounds.Dy(), upBounds.Min.X, upBounds.Min.Y)
	upNonZero := 0
	for i := 3; i < len(upBuf); i += 4 {
		if upBuf[i] > 0 {
			upNonZero++
		}
	}
	if upNonZero == 0 {
		t.Fatalf("expected visible pixels in upright RenderBGRA")
	}

	// Rotated StateFall (45 deg & 180 deg) must stay inside RenderBounds with no clipping at edges
	p.State = StateFall
	for _, angle := range []float64{0.785398, 1.570796, 3.14159} {
		p.Tilt = angle
		rotBounds := w.RenderBounds()
		if rotBounds.Empty() {
			t.Fatalf("expected non-empty RenderBounds for rotated fall angle=%.2f", angle)
		}
		// Render into a larger canvas and verify no non-zero pixels fall outside rotBounds
		pad := 60
		big := rotBounds.Inset(-pad).Intersect(screen)
		bigBuf := make([]byte, big.Dx()*big.Dy()*4)
		w.RenderBGRA(bigBuf, big.Dx(), big.Dy(), big.Min.X, big.Min.Y)

		rotNonZero := 0
		for y := big.Min.Y; y < big.Max.Y; y++ {
			for x := big.Min.X; x < big.Max.X; x++ {
				a := bigBuf[((y-big.Min.Y)*big.Dx()+(x-big.Min.X))*4+3]
				if a > 0 {
					rotNonZero++
					if !image.Pt(x, y).In(rotBounds) {
						t.Fatalf("rotated fall pixel (%d,%d) clipped outside RenderBounds %v at angle=%.2f", x, y, rotBounds, angle)
					}
				}
			}
		}
		if rotNonZero == 0 {
			t.Fatalf("expected visible pixels in rotated RenderBGRA at angle=%.2f", angle)
		}
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
