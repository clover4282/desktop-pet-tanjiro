package pet

import (
	"image"
	"image/color"
	"math"
	"math/rand"
	"time"
)

type PetState int

const (
	StateClawSpawn PetState = iota
	StateIdle
	StateWalk
	StateGrabbed
	StateFall
	StateStacked
	StateClawDismiss
	StateClimbUp
	StateMantle
	StateJumpPrep
	StateJumpArc
)

// WindowRect represents an open visible desktop application window on Windows (in front-to-back Z-order).
type WindowRect struct {
	ID        uintptr
	Rect      image.Rectangle
	Occluding bool // true for maximized/full-area windows: blocks windows behind it, cannot be climbed on top
}

type EffectKind int

const (
	EffectMochiRing EffectKind = iota
	EffectFlameRing
	EffectLightningRing
	EffectDustPuff
	EffectSnowCompass
	EffectWaterWave
)

// GroundEffect represents an expanding landing or character aura effect on the floor/pet.
type GroundEffect struct {
	X, Y  float64
	Life  float64
	Max   float64
	MaxR  float64
	Kind  EffectKind
	Color color.NRGBA
}

type Pet struct {
	ID            int
	CharID        string
	X, Y          float64 // bottom-center anchor in screen coordinates
	VX, VY        float64
	FacingLeft    bool
	State         PetState
	StateTimer    float64
	AnimTime      float64
	ReactTimer    float64 // >0 means showing react pose (works both grounded AND stacked!)
	Squash        float64 // positive = squashed wide/short, negative = stretched tall/narrow
	SquashVel     float64
	Tilt          float64 // radians
	JitterX       float64 // horizontal panic/excitement shiver offset
	StackParentID int     // 0 if not stacked on another pet

	// Character-specific falling state
	FallTime     float64
	FallTrailAcc float64
	FallZigDir   float64

	// Character-specific rubbing (문지르기) state
	RubMeter       float64 // 0..1 intensity that builds up while rubbing
	RubActiveTimer float64 // >0 while actively being rubbed
	RubAccum       float64 // accumulated cursor travel over pet to distinguish rub from casual pass
	RubLastDirX    int     // -1, 0, +1 for direction reversal bonus
	RubParticleAcc float64

	// Natural walk cycle state
	TargetVX  float64
	WalkPhase float64

	// Window climbing / parkour state
	AttachedWinID uintptr // 0 = taskbar / screen edge, non-zero = HWND of tracked window
	ClimbFaceDir  float64 // +1 = wall is to pet's right, -1 = wall is to pet's left
	ClimbPhase    float64
	ClimbGoalY    float64 // optional height to stop climbing and jump off
	MantleStartX  float64
	MantleEndX    float64
	MantleTopY    float64
	MantleT       float64
	JumpDirX      float64 // -1 or +1 launch direction
	IgnoreWinID     uintptr // briefly ignore the window just jumped off
	IgnoreWinTime   float64
	IgnoreStackID   int // briefly ignore the pet whose head we just hopped off
	IgnoreStackTime float64

	// Propeller claw state (used for StateClawSpawn & StateClawDismiss)
	ClawY        float64
	ClawOpen     float64 // 0 = closed gripping, 1 = open
	TargetX      float64
	TargetY      float64
	TargetParent int // optional parent pet ID to stack on upon spawn landing
	ClawGrabbed  bool
}

// DetachedClaw represents a propeller claw flying back up after dropping a pet.
type DetachedClaw struct {
	X, Y     float64
	VY       float64
	AnimTime float64
	Open     float64
}

type Particle struct {
	X, Y   float64
	VX, VY float64
	Life   float64
	Max    float64
	Size   float64
	Kind   ParticleKind
	Color  color.NRGBA
}

type World struct {
	Bank     *SpriteBank
	WorkArea image.Rectangle // Primary monitor work area (MaxY = top of taskbar)
	Screen   image.Rectangle // Full monitor rect (for bounds clipping)

	BaseSize    float64 // Logical sprite size in pixels (e.g. 132 * dpiScale)
	Scale       float64 // User scale multiplier (0.85, 1.0, 1.25, 1.5)
	FreeRoam    bool
	WindowClimb bool
	TopMost     bool

	Windows       []WindowRect
	Pets          []*Pet
	DetachedClaws []*DetachedClaw
	Particles     []*Particle
	Effects       []*GroundEffect

	nextID         int
	rng            *rand.Rand
	dragPetID      int
	dragStartX     float64
	dragStartY     float64
	dragGrabOffX   float64
	dragGrabOffY   float64
	dragMoved      bool
	lastMouseX     float64
	hoverPetID     int
	hoverLastX     float64
	hoverLastY     float64
	hoverHasPrev   bool
	hoverResetSec  float64
	pendingIntro   float64
	introBaseID    int
	introSpawnDone bool
}

func NewWorld(bank *SpriteBank, workArea, screen image.Rectangle, dpiScale float64) *World {
	if dpiScale < 0.75 {
		dpiScale = 1.0
	}
	w := &World{
		Bank:        bank,
		WorkArea:    workArea,
		Screen:      screen,
		BaseSize:    132.0 * dpiScale,
		Scale:       1.0,
		FreeRoam:    true, // Walk naturally & explore windows by default!
		WindowClimb: true,
		TopMost:     true,
		nextID:      1,
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	w.StartClipIntro()
	return w
}

// StartClipIntro opens with Mang-gom sitting above the bottom-right taskbar,
// and 0.55s later the propeller claw drops Tanjiro right onto Mang-gom's head!
func (w *World) StartClipIntro() {
	w.Pets = nil
	w.DetachedClaws = nil
	w.Particles = nil
	w.Effects = nil
	w.dragPetID = 0
	w.hoverPetID = 0
	w.hoverHasPrev = false

	size := w.PetSize()
	startX := float64(w.WorkArea.Max.X) - size*0.72
	floorY := float64(w.WorkArea.Max.Y)

	manggom := w.newPet("manggom", startX, floorY)
	manggom.State = StateIdle
	manggom.StateTimer = 4.0
	w.Pets = append(w.Pets, manggom)

	w.introBaseID = manggom.ID
	w.pendingIntro = 0.55
	w.introSpawnDone = false
}

func (w *World) PetSize() float64 {
	scale := w.Scale
	if scale <= 0 {
		scale = 1.0
	}
	return w.BaseSize * scale
}

func (w *World) stackRoot(p *Pet) *Pet {
	cur := p
	for guard := 0; guard < 32 && cur != nil && cur.StackParentID != 0; guard++ {
		parent := w.FindPet(cur.StackParentID)
		if parent == nil {
			break
		}
		cur = parent
	}
	return cur
}

func (w *World) SetWorkArea(workArea, screen image.Rectangle) {
	oldFloor := float64(w.WorkArea.Max.Y)
	w.WorkArea = workArea
	w.Screen = screen
	newFloor := float64(w.WorkArea.Max.Y)
	dy := newFloor - oldFloor
	for _, p := range w.Pets {
		switch {
		case p.AttachedWinID == 0 && (p.State == StateIdle || p.State == StateWalk):
			p.Y = newFloor
		case p.State == StateStacked:
			if root := w.stackRoot(p); root != nil && root.AttachedWinID == 0 {
				p.Y += dy
			}
		case p.State == StateClawSpawn && p.TargetParent == 0:
			p.TargetY = newFloor
		}
		if p.State != StateClimbUp && p.State != StateMantle {
			w.clampPetX(p)
		}
	}
}

// SetWindows updates the list of visible Windows app windows and keeps attached pets riding along
// if a window moves, or gracefully falling if a window closes, minimizes, or becomes maximized/occluding!
func (w *World) SetWindows(wins []WindowRect) {
	oldMap := make(map[uintptr]image.Rectangle, len(w.Windows))
	for _, ow := range w.Windows {
		oldMap[ow.ID] = ow.Rect
	}
	w.Windows = wins

	newIdx := make(map[uintptr]int, len(wins))
	for i, nw := range wins {
		newIdx[nw.ID] = i
	}

	for _, p := range w.Pets {
		if p.AttachedWinID == 0 {
			continue
		}
		idx, ok := newIdx[p.AttachedWinID]
		if !ok || wins[idx].Occluding {
			// Window closed, minimized, or maximized/occluding -> fall gracefully with character animation!
			p.AttachedWinID = 0
			w.unstackChildren(p.ID)
			if p.State != StateGrabbed && p.State != StateClawSpawn && p.State != StateClawDismiss {
				w.startFall(p, 0, 0)
			}
			continue
		}
		nr := wins[idx].Rect
		if or, had := oldMap[p.AttachedWinID]; had {
			dx := float64(nr.Min.X - or.Min.X)
			dy := float64(nr.Min.Y - or.Min.Y)
			size := w.PetSize()
			margin := size * 0.30
			switch p.State {
			case StateClimbUp:
				// Keep hugging exact updated left/right edge and clamp Y inside resized wall
				if p.ClimbFaceDir > 0 {
					p.X = float64(nr.Min.X) - size*0.24
				} else {
					p.X = float64(nr.Max.X) + size*0.24
				}
				p.Y += dy
				if p.Y < float64(nr.Min.Y) {
					p.Y = float64(nr.Min.Y)
				}
				if p.Y > float64(nr.Max.Y) {
					p.Y = float64(nr.Max.Y)
				}
			case StateMantle:
				p.MantleStartX += dx
				p.MantleEndX += dx
				p.MantleTopY = float64(nr.Min.Y)
				p.X += dx
				p.Y += dy
				lx := float64(nr.Min.X) + margin
				rx := float64(nr.Max.X) - margin
				if rx < lx {
					p.MantleEndX = float64(nr.Min.X+nr.Max.X) * 0.5
				} else {
					p.MantleEndX = clampF(p.MantleEndX, lx, rx)
				}
			case StateIdle, StateWalk, StateJumpPrep:
				p.X += dx
				p.Y = float64(nr.Min.Y)
				lx := float64(nr.Min.X) + margin
				rx := float64(nr.Max.X) - margin
				if rx < lx {
					p.X = float64(nr.Min.X+nr.Max.X) * 0.5
				} else {
					p.X = clampF(p.X, lx, rx)
				}
			}
		}
	}
}

func (w *World) findWindow(id uintptr) (WindowRect, bool) {
	for _, wr := range w.Windows {
		if wr.ID == id {
			return wr, true
		}
	}
	return WindowRect{}, false
}

func (w *World) findWindowWithIndex(id uintptr) (WindowRect, int, bool) {
	for i, wr := range w.Windows {
		if wr.ID == id {
			return wr, i, true
		}
	}
	return WindowRect{}, -1, false
}

// isPointOccluded returns true if any higher-Z-order window (index < winIdx) covers (x, y).
func (w *World) isPointOccluded(winIdx, x, y int) bool {
	pt := image.Pt(x, y)
	for j := 0; j < winIdx && j < len(w.Windows); j++ {
		if pt.In(w.Windows[j].Rect) {
			return true
		}
	}
	return false
}

// isWallSegmentVisible checks that the vertical wall edge of w.Windows[winIdx] from startY up to topY
// is NOT covered by any foreground window in front of it!
func (w *World) isWallSegmentVisible(winIdx, wallX int, faceDir float64, startY, topY int) bool {
	if winIdx <= 0 {
		return true
	}
	// Check both the exact border and 6px inside the window body along the climb path
	inX := wallX + int(faceDir*6)
	if startY < topY {
		startY = topY
	}
	ys := [4]int{
		topY + 4,
		topY + (startY-topY)/3,
		topY + (startY-topY)*2/3,
		maxInt(topY+4, startY-18),
	}
	for _, y := range ys {
		if w.isPointOccluded(winIdx, wallX, y) || w.isPointOccluded(winIdx, inX, y) {
			return false
		}
	}
	return true
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (w *World) newPet(charID string, x, y float64) *Pet {
	if _, ok := w.Bank.ByID[charID]; !ok {
		charID = Catalog[0].ID
	}
	id := w.nextID
	w.nextID++
	return &Pet{
		ID:         id,
		CharID:     charID,
		X:          x,
		Y:          y,
		State:      StateIdle,
		StateTimer: 2.5 + w.rng.Float64()*4.0,
	}
}

// HasCharacter returns true if the given character is already active on screen (not being dismissed).
func (w *World) HasCharacter(charID string) bool {
	return w.FindPetByChar(charID) != nil
}

// FindPetByChar returns the active pet for charID, or nil if not currently on screen.
func (w *World) FindPetByChar(charID string) *Pet {
	for _, p := range w.Pets {
		if p.CharID == charID && p.State != StateClawDismiss {
			return p
		}
	}
	return nil
}

// SpawnWithClaw drops a character from above using the spinning propeller claw.
// Duplicate characters are prevented: if already on screen, returns the existing pet and triggers a cute wave!
// If stackOnID > 0 and that pet exists, it aims directly onto that pet's head!
func (w *World) SpawnWithClaw(charID string, targetX float64, stackOnID int) *Pet {
	if existing := w.FindPetByChar(charID); existing != nil {
		w.TriggerReact(existing.ID)
		return existing
	}
	size := w.PetSize()
	floorY := float64(w.WorkArea.Max.Y)

	if targetX <= 0 {
		// Pick a nice spot near bottom-right / existing pets without excessive overlap
		targetX = w.pickSpawnX()
	}
	margin := size * 0.55
	if targetX < float64(w.WorkArea.Min.X)+margin {
		targetX = float64(w.WorkArea.Min.X) + margin
	}
	if targetX > float64(w.WorkArea.Max.X)-margin {
		targetX = float64(w.WorkArea.Max.X) - margin
	}

	targetY := floorY
	if stackOnID > 0 {
		if parent := w.FindPet(stackOnID); parent != nil {
			targetX = parent.X
			targetY = w.HeadTopY(parent)
		} else {
			stackOnID = 0
		}
	}

	startY := float64(w.WorkArea.Min.Y) - size*0.4
	p := w.newPet(charID, targetX, startY)
	p.State = StateClawSpawn
	p.TargetX = targetX
	p.TargetY = targetY
	p.TargetParent = stackOnID
	p.ClawY = startY - size*0.96
	p.ClawOpen = 0
	w.Pets = append(w.Pets, p)
	return p
}

func (w *World) SpawnRandomWithClaw() *Pet {
	avail := make([]CharacterDef, 0, len(Catalog))
	for _, def := range Catalog {
		if !w.HasCharacter(def.ID) {
			avail = append(avail, def)
		}
	}
	if len(avail) == 0 {
		return nil
	}
	def := avail[w.rng.Intn(len(avail))]
	return w.SpawnWithClaw(def.ID, 0, 0)
}

func (w *World) SpawnAll() {
	size := w.PetSize()
	span := float64(w.WorkArea.Dx()) - size*1.4
	step := span / float64(len(Catalog))
	if step > size*0.92 {
		step = size * 0.92
	}
	rightEdge := float64(w.WorkArea.Max.X) - size*0.65
	spawned := 0
	for i, def := range Catalog {
		if w.HasCharacter(def.ID) {
			continue
		}
		tx := rightEdge - float64(i)*step
		p := w.SpawnWithClaw(def.ID, tx, 0)
		// Stagger vertical starts slightly so they cascade down adorably!
		p.Y -= float64(spawned) * size * 0.7
		p.ClawY = p.Y - size*0.96
		spawned++
	}
}

func (w *World) DismissPet(id int) {
	p := w.FindPet(id)
	if p == nil || p.State == StateClawDismiss {
		return
	}
	if w.dragPetID == id {
		w.dragPetID = 0
		w.dragMoved = false
	}
	// Release any pets stacked on top of this pet
	w.unstackChildren(id)
	p.StackParentID = 0
	p.AttachedWinID = 0
	p.State = StateClawDismiss
	p.ClawGrabbed = false
	p.ClawOpen = 1.0
	p.ClawY = float64(w.WorkArea.Min.Y) - w.PetSize()*0.6
	p.ReactTimer = 0
	p.RubActiveTimer = 0
	p.RubMeter = 0
	p.RubAccum = 0
}

func (w *World) UnstackAll() {
	for _, p := range w.Pets {
		if p.StackParentID != 0 {
			p.StackParentID = 0
			w.startFall(p, (w.rng.Float64()-0.5)*260, -120)
		}
	}
}

func (w *World) ClearAll() {
	w.dragPetID = 0
	w.dragMoved = false
	for _, p := range w.Pets {
		if p.State != StateClawDismiss {
			p.StackParentID = 0
			p.AttachedWinID = 0
			p.State = StateClawDismiss
			p.ClawGrabbed = false
			p.ClawOpen = 1.0
			p.ClawY = float64(w.WorkArea.Min.Y) - w.PetSize()*0.6 - w.rng.Float64()*200
			p.ReactTimer = 0
			p.RubActiveTimer = 0
			p.RubMeter = 0
			p.RubAccum = 0
		}
	}
}

func (w *World) pickSpawnX() float64 {
	size := w.PetSize()
	// Start near bottom-right like the clip, shifting left as more pets appear
	baseRight := float64(w.WorkArea.Max.X) - size*0.72
	for slot := 0; slot < 14; slot++ {
		cand := baseRight - float64(slot)*size*0.85
		if cand < float64(w.WorkArea.Min.X)+size*0.6 {
			break
		}
		occupied := false
		for _, p := range w.Pets {
			if p.StackParentID == 0 && math.Abs(p.X-cand) < size*0.55 {
				occupied = true
				break
			}
		}
		if !occupied {
			return cand
		}
	}
	margin := size * 0.7
	return float64(w.WorkArea.Min.X) + margin + w.rng.Float64()*math.Max(10, float64(w.WorkArea.Dx())-margin*2)
}

func (w *World) FindPet(id int) *Pet {
	for _, p := range w.Pets {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func (w *World) HeadTopY(p *Pet) float64 {
	size := w.PetSize()
	ss := w.Bank.ByID[p.CharID]
	headFrac := 0.15
	if ss != nil {
		headFrac = ss.Def.HeadTopFrac
	}
	// Slight squash shifts head down proportionally
	effScaleY := 1.0 - p.Squash*0.35
	spriteH := size * effScaleY
	return p.Y - spriteH*(1.0-headFrac) + size*0.04
}

func (w *World) TriggerReact(id int) {
	p := w.FindPet(id)
	if p == nil {
		return
	}
	if p.State == StateClawSpawn || p.State == StateClawDismiss || p.State == StateGrabbed {
		return
	}
	p.ReactTimer = 4.2
	p.Squash = -0.22 // pop upward!
	p.SquashVel = 0
	if p.State == StateWalk {
		p.State = StateIdle
		p.VX = 0
	}
	w.spawnReactionParticles(p, 7)
}

func (w *World) spawnReactionParticles(p *Pet, count int) {
	ss := w.Bank.ByID[p.CharID]
	if ss == nil {
		return
	}
	size := w.PetSize()
	for i := 0; i < count; i++ {
		// Spawn around the outer halo (left/right/above head) so the character's cute face stays clear!
		theta := -math.Pi*0.95 + w.rng.Float64()*math.Pi*0.9
		radX := size * (0.42 + w.rng.Float64()*0.18)
		radY := size * (0.45 + w.rng.Float64()*0.15)
		spd := (32 + w.rng.Float64()*48) * w.Scale
		w.Particles = append(w.Particles, &Particle{
			X:     p.X + math.Cos(theta)*radX,
			Y:     p.Y - size*0.58 + math.Sin(theta)*radY,
			VX:    math.Cos(theta) * spd,
			VY:    math.Sin(theta)*spd - 24*w.Scale,
			Life:  0,
			Max:   0.85 + w.rng.Float64()*0.45,
			Size:  (6 + w.rng.Float64()*4) * w.Scale,
			Kind:  ss.Def.ParticleKind,
			Color: ss.Def.AccentColor,
		})
	}
}

// MouseDown returns the hit pet ID (0 if none).
func (w *World) MouseDown(screenX, screenY float64) int {
	id := w.HitTest(screenX, screenY)
	if id == 0 {
		return 0
	}
	p := w.FindPet(id)
	if p == nil || p.State == StateClawSpawn || p.State == StateClawDismiss {
		return 0
	}
	w.dragPetID = id
	w.dragStartX = screenX
	w.dragStartY = screenY
	w.lastMouseX = screenX
	w.dragGrabOffX = p.X - screenX
	w.dragGrabOffY = p.Y - screenY
	w.dragMoved = false
	w.bringToFront(id)
	return id
}

func (w *World) MouseMove(screenX, screenY float64) {
	if w.dragPetID == 0 {
		return
	}
	p := w.FindPet(w.dragPetID)
	if p == nil {
		w.dragPetID = 0
		return
	}
	dxStart := screenX - w.dragStartX
	dyStart := screenY - w.dragStartY
	if !w.dragMoved && math.Hypot(dxStart, dyStart) > 7.0 {
		w.dragMoved = true
		p.StackParentID = 0
		p.AttachedWinID = 0
		p.State = StateGrabbed
		p.ReactTimer = 0
		p.RubActiveTimer = 0
		p.RubMeter = 0
		p.RubAccum = 0
		w.unstackChildren(p.ID)
	}
	if w.dragMoved {
		vx := (screenX - w.lastMouseX) * 35.0
		p.VX = p.VX*0.6 + vx*0.4
		p.X = screenX + w.dragGrabOffX
		p.Y = screenY + w.dragGrabOffY
		// Keep inside screen bounds
		if p.Y > float64(w.WorkArea.Max.Y)+w.PetSize()*0.15 {
			p.Y = float64(w.WorkArea.Max.Y) + w.PetSize()*0.15
		}
		w.clampPetX(p)
	}
	w.lastMouseX = screenX
}

func (w *World) MouseUp(screenX, screenY float64) {
	if w.dragPetID == 0 {
		return
	}
	id := w.dragPetID
	moved := w.dragMoved
	w.dragPetID = 0
	w.dragMoved = false

	p := w.FindPet(id)
	if p == nil {
		return
	}
	if !moved {
		// Short click / petting -> trigger cute reaction!
		w.TriggerReact(id)
		return
	}
	// Released after dragging:
	// 1. Over another pet's head -> cozy stack!
	if host := w.findStackTarget(p, true); host != nil {
		p.AttachedWinID = 0
		w.attachStack(p, host)
		return
	}
	// 2. Near a visible window's vertical side wall -> grab right onto the window and climb!
	if w.WindowClimb {
		if win, faceDir, ok := w.findWindowWallNear(p.X, p.Y, w.PetSize()*0.42); ok {
			w.startClimbUp(p, win.ID, faceDir)
			return
		}
	}
	p.AttachedWinID = 0
	w.startFall(p, clampF(p.VX*0.25, -280, 280), 0)
}

func (w *World) startFall(p *Pet, vx, vy float64) {
	p.State = StateFall
	p.VX = vx
	p.VY = vy
	p.FallTime = 0
	p.FallTrailAcc = 0
	p.FallZigDir = 1
	if w.rng.Float64() < 0.5 {
		p.FallZigDir = -1
	}
	p.ReactTimer = 0
	p.RubActiveTimer = 0
	p.RubMeter = 0
	// Character-specific initial fall burst
	switch p.CharID {
	case "zenitsu":
		w.spawnParticleCustom(p.X-w.PetSize()*0.3, p.Y-w.PetSize()*0.75, -45, -70, 0.55, 7, ParticleSweat, color.NRGBA{120, 210, 255, 255})
		w.spawnParticleCustom(p.X+w.PetSize()*0.3, p.Y-w.PetSize()*0.75, 45, -70, 0.55, 7, ParticleSweat, color.NRGBA{120, 210, 255, 255})
	case "tanjiro":
		p.Squash = -0.14
	case "kyojuro":
		p.Squash = -0.16
		p.VY = math.Max(vy, 120*w.Scale) // bold meteor plunge!
	case "inosuke":
		p.VY = math.Max(vy, 90*w.Scale)
	}
}

// MouseHover handles cursor movement over pets when not dragging, enabling character-specific rubbing (문지르기)!
func (w *World) MouseHover(screenX, screenY float64) {
	if w.Scale <= 0 {
		w.Scale = 1.0
	}
	if w.dragPetID != 0 {
		w.hoverHasPrev = false
		return
	}
	id := w.HitTest(screenX, screenY)
	if id == 0 {
		// Allow brief cursor overshoot off slender character silhouettes while scrubbing back and forth!
		if w.hoverPetID != 0 {
			if prev := w.FindPet(w.hoverPetID); prev != nil {
				size := w.PetSize()
				if math.Abs(screenX-prev.X) <= size*0.62 && screenY >= prev.Y-size*1.08 && screenY <= prev.Y+size*0.1 {
					id = prev.ID
				}
			}
		}
		if id == 0 {
			w.hoverPetID = 0
			w.hoverHasPrev = false
			return
		}
	}
	p := w.FindPet(id)
	if !isStackableHost(p) {
		w.hoverPetID = 0
		w.hoverHasPrev = false
		return
	}
	w.hoverResetSec = 0
	if w.hoverPetID != id || !w.hoverHasPrev {
		w.hoverPetID = id
		w.hoverLastX = screenX
		w.hoverLastY = screenY
		w.hoverHasPrev = true
		return
	}

	dx := screenX - w.hoverLastX
	dy := screenY - w.hoverLastY
	w.hoverLastX = screenX
	w.hoverLastY = screenY

	dist := math.Hypot(dx, dy)
	if dist < 1.5 || dist > 180 {
		return
	}

	// Detect horizontal direction reversals (classic back-and-forth petting motion!)
	dirX := 0
	if dx > 2.5 {
		dirX = 1
	} else if dx < -2.5 {
		dirX = -1
	}
	bonus := 1.0
	if dirX != 0 && p.RubLastDirX != 0 && dirX != p.RubLastDirX {
		bonus = 2.2 // reversing hand direction quickly signals intentional rubbing!
	}
	if dirX != 0 {
		p.RubLastDirX = dirX
	}

	p.RubAccum += dist * bonus
	threshold := 58.0 * w.Scale
	if p.RubAccum >= threshold {
		p.RubActiveTimer = 0.34
		gain := math.Min(0.14, (dist*bonus)/(260.0*w.Scale))
		p.RubMeter = math.Min(1.0, p.RubMeter+gain)
		if p.State == StateWalk {
			p.State = StateIdle
			p.VX = 0
		}
		w.applyRubImpulse(p, dx, dy)
	}
}

func (w *World) applyRubImpulse(p *Pet, dx, dy float64) {
	dir := clampF(dx/18.0, -1, 1)
	switch p.CharID {
	case "manggom":
		// Super squishy mochi dough stretch under your cursor!
		p.Squash = clampF(p.Squash+0.14+math.Abs(dy)*0.004, -0.25, 0.44)
		p.Tilt = clampF(p.Tilt+dir*0.16, -0.34, 0.34)
	case "tanjiro":
		// Warm kind sway + gentle water bounce
		p.Squash = clampF(p.Squash-0.08, -0.22, 0.22)
		p.Tilt = clampF(p.Tilt+dir*0.10, -0.20, 0.20)
	case "kyojuro":
		// Enthusiastic fiery nod/bounce!
		p.Squash = clampF(p.Squash-0.09, -0.24, 0.22)
		p.Tilt = clampF(p.Tilt+dir*0.09, -0.18, 0.18)
	case "zenitsu":
		// Flustered shiver -> static zap!
		p.Squash = clampF(p.Squash+0.07, -0.22, 0.22)
		p.Tilt = clampF(p.Tilt+dir*0.14, -0.24, 0.24)
	case "inosuke":
		// Proud fluffy sway ("호와호와")!
		p.Squash = clampF(p.Squash-0.08, -0.22, 0.22)
		p.Tilt = clampF(p.Tilt+dir*0.13, -0.24, 0.24)
	case "akaza":
		// Tsundere head-shake at first, then proud steady stance
		if p.RubMeter < 0.55 {
			p.Tilt = clampF(p.Tilt-dir*0.14, -0.24, 0.24) // resists your hand slightly!
		} else {
			p.Squash = clampF(p.Squash-0.07, -0.20, 0.20)
			p.Tilt = clampF(p.Tilt+dir*0.07, -0.15, 0.15)
		}
	}
}

func (w *World) spawnParticleCustom(x, y, vx, vy, maxLife, size float64, kind ParticleKind, col color.NRGBA) {
	if len(w.Particles) >= 120 {
		w.Particles = w.Particles[len(w.Particles)-119:]
	}
	w.Particles = append(w.Particles, &Particle{
		X:     x,
		Y:     y,
		VX:    vx * w.Scale,
		VY:    vy * w.Scale,
		Life:  0,
		Max:   maxLife,
		Size:  size * w.Scale,
		Kind:  kind,
		Color: col,
	})
}

func (w *World) addGroundEffect(x, y, maxR, maxLife float64, kind EffectKind, col color.NRGBA) {
	if len(w.Effects) >= 24 {
		w.Effects = w.Effects[len(w.Effects)-23:]
	}
	w.Effects = append(w.Effects, &GroundEffect{
		X:     x,
		Y:     y,
		Life:  0,
		Max:   maxLife,
		MaxR:  maxR * w.Scale,
		Kind:  kind,
		Color: col,
	})
}

// isStackableHost returns true only when a pet is stably resting/walking/stacked on a horizontal surface
// (never while climbing a wall, mantling, jumping, falling, grabbed, or being carried by a claw).
func isStackableHost(p *Pet) bool {
	if p == nil {
		return false
	}
	return p.State == StateIdle || p.State == StateWalk || p.State == StateStacked
}

func (w *World) IsDragging() bool {
	return w.dragPetID != 0 && w.dragMoved
}

func (w *World) bringToFront(id int) {
	for i, p := range w.Pets {
		if p.ID == id {
			w.Pets = append(w.Pets[:i], w.Pets[i+1:]...)
			w.Pets = append(w.Pets, p)
			return
		}
	}
}

func (w *World) unstackChildren(parentID int) {
	for _, c := range w.Pets {
		if c.StackParentID == parentID {
			c.StackParentID = 0
			w.startFall(c, (w.rng.Float64()-0.5)*120, -60)
			w.unstackChildren(c.ID)
		}
	}
}

func (w *World) isDescendantOf(candID, ancestorID int) bool {
	cur := w.FindPet(candID)
	for guard := 0; guard < 32 && cur != nil; guard++ {
		if cur.StackParentID == ancestorID {
			return true
		}
		if cur.StackParentID == 0 {
			return false
		}
		cur = w.FindPet(cur.StackParentID)
	}
	return false
}

func (w *World) hasStackChild(parentID int) bool {
	for _, p := range w.Pets {
		if p.StackParentID == parentID {
			return true
		}
	}
	return false
}

func (w *World) findStackTarget(p *Pet, generous bool) *Pet {
	size := w.PetSize()
	xTol := size * 0.44
	yTolUp := size * 0.35
	yTolDown := size * 0.25
	if generous {
		xTol = size * 0.52
		yTolUp = size * 0.55
		yTolDown = size * 0.45
	}

	var best *Pet
	bestDist := 1e9
	for _, other := range w.Pets {
		if other.ID == p.ID || (p.IgnoreStackTime > 0 && other.ID == p.IgnoreStackID) {
			continue
		}
		if !isStackableHost(other) {
			continue
		}
		if w.hasStackChild(other.ID) {
			continue // Stack onto the top-most pet of the tower!
		}
		if w.isDescendantOf(other.ID, p.ID) {
			continue
		}
		dx := math.Abs(p.X - other.X)
		if dx > xTol {
			continue
		}
		headY := w.HeadTopY(other)
		dy := p.Y - headY // positive if p's feet are slightly below headY
		if dy >= -yTolUp && dy <= yTolDown {
			dist := dx + math.Abs(dy)
			if dist < bestDist {
				bestDist = dist
				best = other
			}
		}
	}
	return best
}

func (w *World) attachStack(p, host *Pet) {
	p.StackParentID = host.ID
	p.State = StateStacked
	p.StateTimer = 7.0 + w.rng.Float64()*8.0 // stay perched for 7~15s, then hop off naturally if FreeRoam is ON!
	p.X = host.X
	p.Y = w.HeadTopY(host)
	p.VX = 0
	p.VY = 0
	p.Tilt = 0
	p.Squash = 0.24
	host.Squash = 0.16
	w.bringToFront(p.ID)
	w.spawnReactionParticles(p, 4)
}

func (w *World) clampPetX(p *Pet) {
	half := w.PetSize() * 0.42
	minX := float64(w.WorkArea.Min.X) + half
	maxX := float64(w.WorkArea.Max.X) - half
	if minX > maxX {
		p.X = float64(w.WorkArea.Min.X+w.WorkArea.Max.X) * 0.5
		p.VX = 0
		return
	}
	if p.X < minX {
		p.X = minX
		if p.VX < 0 {
			p.VX = -p.VX
			p.FacingLeft = false
		}
	}
	if p.X > maxX {
		p.X = maxX
		if p.VX > 0 {
			p.VX = -p.VX
			p.FacingLeft = true
		}
	}
}

// Step advances the simulation by dt seconds.
func (w *World) Step(dt float64) {
	if dt <= 0 {
		return
	}
	if dt > 0.05 {
		dt = 0.05
	}
	if w.Scale <= 0 {
		w.Scale = 1.0
	}
	if w.hoverHasPrev {
		w.hoverResetSec += dt
		if w.hoverResetSec > 0.45 {
			w.hoverHasPrev = false
			w.hoverPetID = 0
		}
	}

	// Trigger opening Tanjiro drop onto Mang-gom!
	if !w.introSpawnDone {
		w.pendingIntro -= dt
		if w.pendingIntro <= 0 {
			w.introSpawnDone = true
			if base := w.FindPet(w.introBaseID); base != nil {
				w.SpawnWithClaw("tanjiro", base.X, base.ID)
			}
		}
	}

	size := w.PetSize()
	floorY := float64(w.WorkArea.Max.Y)

	// Iterate over a snapshot so attachStack->bringToFront cannot corrupt iteration or overwrite pets.
	snapshot := make([]*Pet, len(w.Pets))
	copy(snapshot, w.Pets)
	var removedIDs []int
	for _, p := range snapshot {
		p.AnimTime += dt

		// Spring-damper for juicy squash & stretch
		springK := 160.0
		damp := 14.0
		p.SquashVel += (-springK*p.Squash - damp*p.SquashVel) * dt
		p.Squash += p.SquashVel * dt

		if p.ReactTimer > 0 {
			p.ReactTimer -= dt
			if p.ReactTimer < 0 {
				p.ReactTimer = 0
			}
		}

		// Update character-specific rubbing (문지르기) state & continuous particles
		p.JitterX = 0
		if p.IgnoreWinTime > 0 {
			p.IgnoreWinTime -= dt
			if p.IgnoreWinTime <= 0 {
				p.IgnoreWinTime = 0
				p.IgnoreWinID = 0
			}
		}
		if p.IgnoreStackTime > 0 {
			p.IgnoreStackTime -= dt
			if p.IgnoreStackTime <= 0 {
				p.IgnoreStackTime = 0
				p.IgnoreStackID = 0
			}
		}
		w.stepRubbing(p, dt)

		remove := false
		switch p.State {
		case StateClawSpawn:
			// Update target if aimed at a stackable, unoccupied parent pet
			if p.TargetParent != 0 {
				if parent := w.FindPet(p.TargetParent); isStackableHost(parent) && !w.hasStackChild(parent.ID) {
					p.TargetX = parent.X
					p.TargetY = w.HeadTopY(parent)
				} else {
					p.TargetParent = 0
					p.TargetY = floorY
				}
			}
			// Smooth descent
			speed := 360.0 * w.Scale
			dy := p.TargetY - p.Y
			if dy <= speed*dt+2 {
				p.Y = p.TargetY
				p.X = p.TargetX
				p.Tilt = 0
				// Detach propeller claw so it zooms back up!
				w.DetachedClaws = append(w.DetachedClaws, &DetachedClaw{
					X:        p.X,
					Y:        p.ClawY,
					VY:       -60 * w.Scale,
					AnimTime: p.AnimTime,
					Open:     0.85,
				})
				if p.TargetParent != 0 {
					if parent := w.FindPet(p.TargetParent); isStackableHost(parent) && !w.hasStackChild(parent.ID) {
						w.attachStack(p, parent)
						break
					}
					p.TargetParent = 0
				}
				// Check if we happened to land on someone's head
				if host := w.findStackTarget(p, false); host != nil {
					w.attachStack(p, host)
				} else if p.Y < floorY-4 {
					w.startFall(p, 0, 0)
				} else {
					p.State = StateIdle
					p.Squash = 0.24
					p.StateTimer = 2.5 + w.rng.Float64()*4.0
					w.spawnReactionParticles(p, 4)
				}
			} else {
				// Ease out slightly as it approaches target
				stepSpeed := speed
				if dy < size*1.2 {
					stepSpeed = math.Max(140*w.Scale, dy*2.6)
				}
				p.Y += stepSpeed * dt
				p.X += (p.TargetX - p.X) * math.Min(1, dt*8)
				p.ClawY = p.Y - size*0.96
				p.Tilt = math.Sin(p.AnimTime*7.5) * 0.08
			}

		case StateClawDismiss:
			if !p.ClawGrabbed {
				targetClawY := p.Y - size*0.96
				speed := 520.0 * w.Scale
				if p.ClawY+speed*dt >= targetClawY {
					p.ClawY = targetClawY
					p.ClawGrabbed = true
					p.ClawOpen = 0
					p.Squash = -0.15
				} else {
					p.ClawY += speed * dt
				}
			} else {
				liftSpeed := 460.0 * w.Scale
				p.Y -= liftSpeed * dt
				p.ClawY = p.Y - size*0.96
				p.Tilt = math.Sin(p.AnimTime*9.0) * 0.1
				if p.Y < float64(w.WorkArea.Min.Y)-size*0.9 {
					remove = true
				}
			}

		case StateIdle:
			w.stepIdle(p, dt, floorY)

		case StateWalk:
			w.stepWalk(p, dt, floorY)

		case StateClimbUp:
			w.stepClimbUp(p, dt)

		case StateMantle:
			w.stepMantle(p, dt)

		case StateJumpPrep:
			w.stepJumpPrep(p, dt)

		case StateJumpArc:
			w.stepJumpArc(p, dt, floorY)

		case StateGrabbed:
			targetTilt := clampF(p.VX*0.00055, -0.32, 0.32)
			p.Tilt += (targetTilt - p.Tilt) * math.Min(1, dt*14)
			p.VX *= math.Max(0, 1-dt*8)

		case StateFall:
			w.stepFall(p, dt, floorY)

		case StateStacked:
			parent := w.FindPet(p.StackParentID)
			if !isStackableHost(parent) {
				p.StackParentID = 0
				w.startFall(p, 0, 0)
				break
			}
			targetX := parent.X
			targetY := w.HeadTopY(parent)
			// Cute soft spring follow so stack sways gently when bottom pet walks!
			p.X += (targetX - p.X) * math.Min(1, dt*18)
			p.Y += (targetY - p.Y) * math.Min(1, dt*24)
			if p.RubActiveTimer <= 0 {
				p.Tilt = parent.Tilt * 0.65
			} else {
				p.Tilt *= math.Max(0, 1-dt*5)
			}
			// Naturally hop off a friend's head after resting for a while (when FreeRoam is ON & top of stack)
			if w.FreeRoam && !w.hasStackChild(p.ID) && p.ReactTimer <= 0 && p.RubActiveTimer <= 0 {
				p.StateTimer -= dt
				if p.StateTimer <= 0 {
					dir := 1.0
					if p.X > float64(w.WorkArea.Min.X+w.WorkArea.Max.X)*0.5 {
						dir = -1.0 // jump toward the open center of the screen!
					}
					if w.rng.Float64() < 0.3 {
						dir = -dir
					}
					p.IgnoreStackID = parent.ID
					p.IgnoreStackTime = 0.85
					p.StackParentID = 0
					w.startJumpPrep(p, dir)
				}
			}
		}

		if remove {
			removedIDs = append(removedIDs, p.ID)
		}
	}
	if len(removedIDs) > 0 {
		kept := w.Pets[:0]
		for _, p := range w.Pets {
			drop := false
			for _, id := range removedIDs {
				if p.ID == id {
					drop = true
					break
				}
			}
			if !drop {
				kept = append(kept, p)
			}
		}
		w.Pets = kept
	}

	// Update detached propeller claws flying back up off screen
	keptClaws := w.DetachedClaws[:0]
	for _, c := range w.DetachedClaws {
		c.AnimTime += dt
		c.VY -= 950.0 * w.Scale * dt
		c.Y += c.VY * dt
		if c.Y > float64(w.WorkArea.Min.Y)-size*1.2 {
			keptClaws = append(keptClaws, c)
		}
	}
	w.DetachedClaws = keptClaws

	// Update particles
	keptParts := w.Particles[:0]
	for _, pt := range w.Particles {
		pt.Life += dt
		if pt.Life < pt.Max {
			pt.X += pt.VX * dt
			pt.Y += pt.VY * dt
			if pt.Kind == ParticleSweat {
				pt.VY += 280 * w.Scale * dt // sweat drops fall slightly
			} else {
				pt.VY -= 18 * w.Scale * dt // gentle float upward
			}
			keptParts = append(keptParts, pt)
		}
	}
	w.Particles = keptParts

	// Update ground/aura effects
	keptFX := w.Effects[:0]
	for _, fx := range w.Effects {
		fx.Life += dt
		if fx.Life < fx.Max {
			keptFX = append(keptFX, fx)
		}
	}
	w.Effects = keptFX
}

func (w *World) stepRubbing(p *Pet, dt float64) {
	// Decay accumulated casual cursor pass quickly
	if p.RubActiveTimer <= 0 {
		p.RubAccum = math.Max(0, p.RubAccum-140.0*w.Scale*dt)
	} else {
		p.RubActiveTimer -= dt
		if p.RubActiveTimer <= 0 {
			p.RubActiveTimer = 0
			p.RubAccum = 0
			// Leave a pleasant afterglow reaction if rubbed deeply!
			if p.RubMeter >= 0.45 {
				p.ReactTimer = math.Max(p.ReactTimer, 2.2)
			}
		}
	}
	if p.RubActiveTimer <= 0 {
		p.RubMeter = math.Max(0, p.RubMeter-0.55*dt)
		return
	}

	// Active rubbing! Drive character-specific shiver/aura/particles.
	size := w.PetSize()
	p.RubParticleAcc += dt
	emitInterval := 0.14
	switch p.CharID {
	case "manggom":
		// Super squishy jelly sway + hearts
		p.Tilt += math.Sin(p.AnimTime*14) * 0.025
		emitInterval = 0.14
	case "tanjiro":
		// Warm kind Water Breathing ripples & hearts
		emitInterval = 0.11
	case "kyojuro":
		// Energetic blazing aura + rising embers
		emitInterval = 0.10
	case "zenitsu":
		// Panicked rapid shiver -> static electric zaps!
		if p.RubMeter < 0.45 {
			p.JitterX = math.Sin(p.AnimTime*58) * 3.8 * w.Scale
			emitInterval = 0.14
		} else {
			p.JitterX = math.Sin(p.AnimTime*48) * 2.2 * w.Scale
			emitInterval = 0.09
		}
	case "inosuke":
		// "호와호와" dreamy floating clouds galore!
		emitInterval = 0.09
	case "akaza":
		// Tsundere pout shiver -> serene snowflake compass glow!
		if p.RubMeter < 0.55 {
			p.JitterX = math.Sin(p.AnimTime*42) * 2.6 * w.Scale
			emitInterval = 0.22
		} else {
			emitInterval = 0.12
		}
	}

	if p.RubParticleAcc < emitInterval {
		return
	}
	p.RubParticleAcc = 0

	// Spawn character-specific rubbing particles around outer halo
	theta := -math.Pi*0.92 + w.rng.Float64()*math.Pi*0.84
	px := p.X + math.Cos(theta)*size*0.52
	py := p.Y - size*0.58 + math.Sin(theta)*size*0.42
	vx := math.Cos(theta) * (25 + w.rng.Float64()*35)
	vy := math.Sin(theta)*30 - 35

	switch p.CharID {
	case "manggom":
		w.spawnParticleCustom(px, py, vx, vy, 1.0, 8.5, ParticleHeart, color.NRGBA{255, 125, 165, 255})
	case "tanjiro":
		if w.rng.Float64() < 0.35 {
			w.spawnParticleCustom(px, py, vx, vy, 0.95, 7.5, ParticleHeart, color.NRGBA{255, 145, 175, 245})
		} else {
			w.spawnParticleCustom(px, py, vx*1.1, vy-22, 0.85, 8.5, ParticleWater, color.NRGBA{70, 205, 255, 255})
		}
		if p.RubMeter >= 0.6 && w.rng.Float64() < 0.24 {
			w.addGroundEffect(p.X, p.Y-size*0.04, size*0.68, 0.50, EffectWaterWave, color.NRGBA{65, 200, 255, 220})
		}
	case "kyojuro":
		col := color.NRGBA{255, 120, 30, 255}
		if w.rng.Float64() < 0.45 {
			col = color.NRGBA{255, 200, 45, 255}
		}
		w.spawnParticleCustom(px, py, vx*0.8, vy-35, 0.85, 8.5, ParticleFlame, col)
		if p.RubMeter >= 0.6 && w.rng.Float64() < 0.25 {
			w.addGroundEffect(p.X, p.Y-size*0.08, size*0.65, 0.45, EffectFlameRing, color.NRGBA{255, 130, 35, 210})
		}
	case "zenitsu":
		if p.RubMeter < 0.45 {
			w.spawnParticleCustom(px, py, vx*1.3, -45, 0.6, 6.5, ParticleSweat, color.NRGBA{115, 210, 255, 255})
		} else {
			col := color.NRGBA{255, 225, 45, 255}
			if w.rng.Float64() < 0.4 {
				col = color.NRGBA{115, 225, 255, 255}
			}
			w.spawnParticleCustom(px, py, vx*1.5, vy-15, 0.55, 9.0, ParticleLightning, col)
		}
	case "inosuke":
		// Giant fluffy "호와호와" pink-white bubbles!
		w.spawnParticleCustom(px, py, vx*0.8, vy-22, 1.15, 10.5, ParticleBubble, color.NRGBA{255, 180, 215, 255})
	case "akaza":
		if p.RubMeter < 0.55 {
			// Mild irritated puff/spark
			w.spawnParticleCustom(px, py, vx*1.2, vy, 0.5, 6.0, ParticleSparkle, color.NRGBA{255, 130, 175, 255})
		} else {
			w.spawnParticleCustom(px, py, vx, vy-15, 1.0, 9.0, ParticleSnowflake, color.NRGBA{110, 225, 255, 255})
			if w.rng.Float64() < 0.22 {
				w.addGroundEffect(p.X, p.Y-size*0.04, size*0.72, 0.55, EffectSnowCompass, color.NRGBA{110, 225, 255, 220})
			}
		}
	}
}

func (w *World) stepFall(p *Pet, dt, floorY float64) {
	p.FallTime += dt
	p.FallTrailAcc += dt
	size := w.PetSize()

	// Character-specific gravity, mid-air rotation, horizontal motion & trail particles!
	gravity := 1650.0 * w.Scale
	switch p.CharID {
	case "manggom":
		// Stretchy squishy jelly/mochi fall!
		gravity = 1380.0 * w.Scale
		p.VY += gravity * dt
		p.X += p.VX * dt
		p.Squash = -0.20 + math.Sin(p.FallTime*16)*0.12 // stretches vertically while wobbling like jelly!
		p.Tilt = math.Sin(p.FallTime*9) * 0.16
		if p.FallTrailAcc >= 0.12 {
			p.FallTrailAcc = 0
			w.spawnParticleCustom(p.X+(w.rng.Float64()-0.5)*size*0.5, p.Y-size*0.5, 0, -40, 0.7, 6.5, ParticleHeart, color.NRGBA{255, 165, 195, 215})
		}

	case "tanjiro":
		// Agile Water Breathing swirl descent!
		gravity = 1620.0 * w.Scale
		p.VY += gravity * dt
		p.X += (p.VX + math.Sin(p.FallTime*11)*55*w.Scale) * dt
		p.Tilt = math.Sin(p.FallTime*11) * 0.18
		if p.FallTrailAcc >= 0.045 {
			p.FallTrailAcc = 0
			col := color.NRGBA{65, 205, 255, 250}
			if w.rng.Float64() < 0.35 {
				col = color.NRGBA{235, 250, 255, 240}
			}
			w.spawnParticleCustom(p.X+(w.rng.Float64()-0.5)*size*0.5, p.Y-size*0.5, (w.rng.Float64()-0.5)*70, -75, 0.55, 8.0, ParticleWater, col)
		}
	case "kyojuro":
		// Blazing Flame Hashira meteor drop!
		gravity = 2050.0 * w.Scale
		p.VY += gravity * dt
		p.X += p.VX * dt
		p.Squash = -0.14
		p.Tilt = clampF(p.VX*0.0007, -0.22, 0.22)
		if p.FallTrailAcc >= 0.035 {
			p.FallTrailAcc = 0
			col := color.NRGBA{255, 110, 25, 255}
			if w.rng.Float64() < 0.45 {
				col = color.NRGBA{255, 210, 45, 255}
			}
			w.spawnParticleCustom(p.X+(w.rng.Float64()-0.5)*size*0.5, p.Y-size*0.55, (w.rng.Float64()-0.5)*70, -110-w.rng.Float64()*80, 0.52, 9.0, ParticleFlame, col)
		}

	case "zenitsu":
		// Panicked flailing for 0.16s -> Thunderclap Flash zigzag burst!
		if p.FallTime < 0.16 {
			gravity = 900.0 * w.Scale
			p.VY += gravity * dt
			p.X += p.VX * dt
			p.JitterX = math.Sin(p.FallTime*65) * 4.5 * w.Scale
			p.Tilt = math.Sin(p.FallTime*24) * 0.25
		} else {
			gravity = 2200.0 * w.Scale
			p.VY = math.Max(p.VY+gravity*dt, 520*w.Scale)
			// Crisp zigzag lightning bursts every 0.08s
			zigPhase := int(p.FallTime / 0.08)
			sign := p.FallZigDir
			if zigPhase%2 == 1 {
				sign = -sign
			}
			zigVX := sign * 240.0 * w.Scale
			p.X += (p.VX*0.3 + zigVX) * dt
			p.Tilt = sign * 0.22
			if p.FallTrailAcc >= 0.032 {
				p.FallTrailAcc = 0
				col := color.NRGBA{255, 228, 50, 255}
				if w.rng.Float64() < 0.42 {
					col = color.NRGBA{110, 225, 255, 255}
				}
				w.spawnParticleCustom(p.X+(w.rng.Float64()-0.5)*size*0.5, p.Y-size*0.45, -zigVX*0.25, -80, 0.42, 9.0, ParticleLightning, col)
			}
		}

	case "inosuke":
		// Wild Boar spinning cannonball charge!
		gravity = 2100.0 * w.Scale
		p.VY += gravity * dt
		p.X += p.VX * dt
		p.Tilt += 15.5 * dt
		if p.FallTrailAcc >= 0.045 {
			p.FallTrailAcc = 0
			w.spawnParticleCustom(p.X+(w.rng.Float64()-0.5)*size*0.45, p.Y-size*0.45, (w.rng.Float64()-0.5)*90, -70, 0.48, 8.0, ParticleDust, color.NRGBA{235, 228, 220, 220})
		}

	case "akaza":
		// Sharp controlled martial-arts flip into poised landing stance + ice crystals!
		gravity = 1950.0 * w.Scale
		p.VY += gravity * dt
		p.X += p.VX * dt
		if p.FallTime < 0.20 {
			p.Tilt += 13.0 * dt
		} else {
			// Snaps cleanly upright into martial arts landing stance!
			p.Tilt *= math.Max(0, 1-dt*18)
		}
		if p.FallTrailAcc >= 0.05 {
			p.FallTrailAcc = 0
			w.spawnParticleCustom(p.X+(w.rng.Float64()-0.5)*size*0.5, p.Y-size*0.45, (w.rng.Float64()-0.5)*50, -65, 0.55, 8.0, ParticleSnowflake, color.NRGBA{110, 225, 255, 245})
		}

	default:
		p.VY += gravity * dt
		p.X += p.VX * dt
		p.Tilt *= math.Max(0, 1-dt*6)
	}

	prevY := p.Y
	p.Y += p.VY * dt
	p.VX *= math.Max(0, 1-dt*2.2)
	w.clampPetX(p)

	if p.VY > 0 {
		// 1. Check landing on top of another pet's head
		if host := w.findStackTarget(p, false); host != nil {
			p.AttachedWinID = 0
			w.attachStack(p, host)
			w.triggerCharacterLandingFX(p, true)
			return
		}
		// 2. Check landing on top of a visible open Windows application window!
		if w.WindowClimb {
			if win, ok := w.findLandingWindowTop(p, prevY, p.Y); ok {
				p.Y = float64(win.Rect.Min.Y)
				p.AttachedWinID = win.ID
				p.VX = 0
				p.VY = 0
				p.Tilt = 0
				p.JitterX = 0
				p.State = StateIdle
				p.StateTimer = 1.6 + w.rng.Float64()*2.6
				w.triggerCharacterLandingFX(p, false)
				return
			}
		}
	}

	if p.Y >= floorY {
		p.Y = floorY
		p.AttachedWinID = 0
		p.VX = 0
		p.VY = 0
		p.Tilt = 0
		p.JitterX = 0
		p.State = StateIdle
		p.StateTimer = 2.0 + w.rng.Float64()*3.5
		w.triggerCharacterLandingFX(p, false)
	}
}

type walkGait struct {
	speed     float64 // base px/s at scale 1.0
	stride    float64 // px per half-step cycle
	hop       float64 // vertical step arc height
	waddle    float64 // side-to-side roll radians
	lean      float64 // slight forward lean radians
	stepDust  bool
}

func characterGait(charID string) walkGait {
	switch charID {
	case "manggom":
		return walkGait{speed: 52, stride: 28, hop: 5.5, waddle: 0.055, lean: 0.03}
	case "tanjiro":
		return walkGait{speed: 74, stride: 32, hop: 7.5, waddle: 0.045, lean: 0.05, stepDust: true}
	case "kyojuro":
		return walkGait{speed: 78, stride: 32, hop: 8.5, waddle: 0.065, lean: 0.05, stepDust: true}
	case "zenitsu":
		return walkGait{speed: 74, stride: 22, hop: 6.5, waddle: 0.09, lean: 0.06}
	case "inosuke":
		return walkGait{speed: 88, stride: 30, hop: 10.5, waddle: 0.11, lean: 0.08, stepDust: true}
	case "akaza":
		return walkGait{speed: 80, stride: 34, hop: 7.0, waddle: 0.05, lean: 0.06}
	default:
		return walkGait{speed: 62, stride: 28, hop: 6.5, waddle: 0.075, lean: 0.04}
	}
}

func (w *World) surfaceBounds(p *Pet, floorY float64) (surfY, minX, maxX float64, ok bool) {
	size := w.PetSize()
	if p.AttachedWinID == 0 {
		half := size * 0.42
		return floorY, float64(w.WorkArea.Min.X) + half, float64(w.WorkArea.Max.X) - half, true
	}
	win, winIdx, exists := w.findWindowWithIndex(p.AttachedWinID)
	if !exists || win.Occluding || w.isPointOccluded(winIdx, int(p.X), win.Rect.Min.Y+4) {
		return floorY, 0, 0, false
	}
	margin := size * 0.30
	lx := float64(win.Rect.Min.X) + margin
	rx := float64(win.Rect.Max.X) - margin
	if rx < lx {
		mid := float64(win.Rect.Min.X+win.Rect.Max.X) * 0.5
		lx, rx = mid, mid
	}
	return float64(win.Rect.Min.Y), lx, rx, true
}

func (w *World) stepIdle(p *Pet, dt, floorY float64) {
	surfY, minX, maxX, ok := w.surfaceBounds(p, floorY)
	if !ok {
		p.AttachedWinID = 0
		w.unstackChildren(p.ID)
		w.startFall(p, 0, 0)
		return
	}
	p.Y = surfY
	if p.X < minX {
		p.X = minX
	}
	if p.X > maxX {
		p.X = maxX
	}
	p.VX *= math.Max(0, 1-dt*12)
	if p.RubActiveTimer <= 0 {
		p.Tilt *= math.Max(0, 1-dt*10)
	}
	p.StateTimer -= dt
	if p.StateTimer > 0 || p.ReactTimer > 0 || p.RubActiveTimer > 0 {
		return
	}
	if !w.FreeRoam {
		p.StateTimer = 3.5 + w.rng.Float64()*4.5
		return
	}

	// On top of a window: walk along titlebar, jump off, or climb down!
	// Never jump off or climb walls if someone is stacked on top of your head!
	carryingFriend := w.hasStackChild(p.ID)
	if p.AttachedWinID != 0 {
		roll := w.rng.Float64()
		if roll < 0.52 || carryingFriend {
			if roll < 0.72 {
				w.startWalk(p, 0)
			} else {
				w.TriggerReact(p.ID)
				p.StateTimer = 4.0 + w.rng.Float64()*4.0
			}
		} else if roll < 0.85 && w.WindowClimb {
			// Always jump off when coming down from a window!
			win, _ := w.findWindow(p.AttachedWinID)
			dir := 1.0
			if p.X-float64(win.Rect.Min.X) < float64(win.Rect.Max.X)-p.X {
				dir = -1.0
			}
			w.startJumpPrep(p, dir)
		} else {
			w.TriggerReact(p.ID)
			p.StateTimer = 4.0 + w.rng.Float64()*4.0
		}
		return
	}

	// On the taskbar: walk naturally (sometimes heading toward an open window to climb it if not carrying a friend!)
	roll := w.rng.Float64()
	if roll < 0.72 {
		dir := 0.0
		if !carryingFriend && w.WindowClimb && len(w.Windows) > 0 && w.rng.Float64() < 0.50 {
			// Head toward a climbable window edge!
			if edgeX, ok := w.pickClimbableWindowEdgeX(p); ok {
				if edgeX > p.X+12 {
					dir = 1.0
				} else if edgeX < p.X-12 {
					dir = -1.0
				}
			}
		}
		w.startWalk(p, dir)
	} else if roll < 0.88 {
		w.TriggerReact(p.ID)
		p.StateTimer = 4.5 + w.rng.Float64()*5.0
	} else {
		p.StateTimer = 2.0 + w.rng.Float64()*3.0
	}
}

func (w *World) startWalk(p *Pet, dir float64) {
	if dir == 0 {
		dir = 1.0
		if w.rng.Float64() < 0.5 {
			dir = -1.0
		}
	}
	gait := characterGait(p.CharID)
	speed := gait.speed * (0.88 + w.rng.Float64()*0.24) * w.Scale
	p.State = StateWalk
	p.TargetVX = dir * speed
	p.FacingLeft = dir < 0
	p.StateTimer = 1.8 + w.rng.Float64()*3.2
}

func (w *World) stepWalk(p *Pet, dt, floorY float64) {
	surfY, minX, maxX, ok := w.surfaceBounds(p, floorY)
	if !ok {
		p.AttachedWinID = 0
		w.unstackChildren(p.ID)
		w.startFall(p, 0, 0)
		return
	}
	p.Y = surfY
	if !w.FreeRoam || p.ReactTimer > 0 || p.RubActiveTimer > 0 {
		p.State = StateIdle
		p.VX = 0
		p.TargetVX = 0
		p.StateTimer = 2.4
		return
	}

	gait := characterGait(p.CharID)
	// Smooth ease-in acceleration & gentle braking right before stopping!
	desiredVX := p.TargetVX
	if p.StateTimer < 0.28 {
		desiredVX = 0
	}
	p.VX += (desiredVX - p.VX) * math.Min(1, dt*7.5)
	if math.Abs(p.VX) > 3 {
		p.FacingLeft = p.VX < 0
	}

	prevX := p.X
	p.X += p.VX * dt

	// Advance walk step phase strictly proportional to distance traveled (zero foot sliding!)
	distMoved := math.Abs(p.X - prevX)
	stridePx := math.Max(10, gait.stride*w.Scale)
	prevSin := math.Sin(p.WalkPhase)
	p.WalkPhase += (distMoved / stridePx) * math.Pi
	curSin := math.Sin(p.WalkPhase)

	// Detect foot contact moment (phase crossing 0) -> subtle natural weight squash & step puff!
	if (prevSin >= 0 && curSin < 0) || (prevSin <= 0 && curSin > 0) {
		p.Squash = clampF(p.Squash+0.065, -0.15, 0.22)
		if gait.stepDust && w.rng.Float64() < 0.45 {
			backDir := -1.0
			if p.VX < 0 {
				backDir = 1.0
			}
			w.spawnParticleCustom(p.X+backDir*w.PetSize()*0.18, p.Y-4, backDir*26, -18, 0.35, 4.5, ParticleDust, color.NRGBA{235, 230, 224, 175})
		}
	}

	// Natural side-to-side waddle roll + subtle forward lean into direction of motion
	speedFrac := clampF(math.Abs(p.VX)/(gait.speed*w.Scale), 0, 1.2)
	dirSign := 1.0
	if p.VX < 0 {
		dirSign = -1.0
	}
	p.Tilt = (math.Cos(p.WalkPhase)*gait.waddle + dirSign*gait.lean) * speedFrac

	carryingFriend := w.hasStackChild(p.ID)

	// Check climbing ANY visible (unoccluded) window wall while walking on the taskbar OR on top of a lower window!
	// Never climb when someone is sitting on top of your head!
	if !carryingFriend && w.WindowClimb && math.Abs(p.VX) > 15 {
		if win, faceDir, found := w.findWindowWallHit(p, prevX, p.X); found {
			if w.rng.Float64() < 0.65 {
				w.startClimbUp(p, win.ID, faceDir)
				return
			}
		}
	}

	// Edge handling (either taskbar monitor edge OR top-of-window edge!)
	hitLeft := p.X <= minX
	hitRight := p.X >= maxX
	if hitLeft || hitRight {
		if hitLeft {
			p.X = minX
		} else {
			p.X = maxX
		}
		if p.AttachedWinID != 0 {
			// Reached the edge of a window titlebar! If carrying a friend, always turn around safely.
			if !carryingFriend && w.WindowClimb {
				if w.rng.Float64() < 0.55 {
					jumpDir := -1.0
					if hitRight {
						jumpDir = 1.0
					}
					w.startJumpPrep(p, jumpDir)
					return
				}
			}
			// Turn around smoothly
			p.TargetVX = -p.TargetVX
			p.VX = p.TargetVX * 0.4
			p.FacingLeft = p.TargetVX < 0
		} else {
			// Reached monitor left/right edge! Only climb screen wall if NOT carrying a friend!
			if !carryingFriend && w.WindowClimb && w.rng.Float64() < 0.38 {
				faceDir := -1.0 // left screen edge -> wall is to pet's left
				if hitRight {
					faceDir = 1.0 // right screen edge -> wall is to pet's right
				}
				w.startClimbUp(p, 0, faceDir)
				return
			}
			p.TargetVX = -p.TargetVX
			p.VX = p.TargetVX * 0.4
			p.FacingLeft = p.TargetVX < 0
		}
	}

	p.StateTimer -= dt
	if p.StateTimer <= 0 && math.Abs(p.VX) < 12*w.Scale {
		p.State = StateIdle
		p.VX = 0
		p.TargetVX = 0
		p.Squash = 0.08 // cozy stop settle
		p.StateTimer = 1.8 + w.rng.Float64()*3.4
	}
}

func (w *World) startClimbUp(p *Pet, winID uintptr, faceDir float64) {
	if w.hasStackChild(p.ID) {
		return // Never climb walls while someone is stacked on your head!
	}
	var targetWin WindowRect
	if winID != 0 {
		win, ok := w.findWindow(winID)
		if !ok || win.Occluding {
			return
		}
		targetWin = win
	}
	p.StackParentID = 0
	p.AttachedWinID = winID
	p.ClimbFaceDir = faceDir // +1 = wall to right, -1 = wall to left
	p.FacingLeft = faceDir < 0
	p.State = StateClimbUp
	p.ClimbPhase = 0
	p.VX = 0
	p.VY = 0
	p.TargetVX = 0
	p.ReactTimer = 0
	p.RubActiveTimer = 0
	p.RubMeter = 0
	p.RubAccum = 0
	p.Squash = 0.14

	// Snap cleanly onto wall X
	size := w.PetSize()
	if winID != 0 {
		if faceDir > 0 {
			p.X = float64(targetWin.Rect.Min.X) - size*0.24
		} else {
			p.X = float64(targetWin.Rect.Max.X) + size*0.24
		}
		p.ClimbGoalY = float64(targetWin.Rect.Min.Y)
	} else {
		if faceDir < 0 {
			p.X = float64(w.WorkArea.Min.X) + size*0.28
		} else {
			p.X = float64(w.WorkArea.Max.X) - size*0.28
		}
		// Pick a fun climb height on screen edge before jumping off or descending
		minY := float64(w.WorkArea.Min.Y) + size*1.35
		maxY := float64(w.WorkArea.Max.Y) - size*2.2
		if maxY < minY {
			maxY = minY
		}
		p.ClimbGoalY = minY + w.rng.Float64()*(maxY-minY)
	}
}

func (w *World) stepClimbUp(p *Pet, dt float64) {
	size := w.PetSize()
	var topY float64
	var attachedWin WindowRect
	if p.AttachedWinID != 0 {
		win, winIdx, ok := w.findWindowWithIndex(p.AttachedWinID)
		if !ok || win.Occluding {
			p.AttachedWinID = 0
			w.startFall(p, 0, 0)
			return
		}
		attachedWin = win
		wallX := win.Rect.Min.X
		if p.ClimbFaceDir > 0 {
			p.X = float64(win.Rect.Min.X) - size*0.24
		} else {
			wallX = win.Rect.Max.X
			p.X = float64(win.Rect.Max.X) + size*0.24
		}
		// If a foreground window now covers this wall edge, drop off cleanly instead of phasing through it!
		inX := wallX + int(p.ClimbFaceDir*6)
		probeY := int(p.Y - size*0.35)
		if w.isPointOccluded(winIdx, wallX, probeY) || w.isPointOccluded(winIdx, inX, probeY) {
			p.AttachedWinID = 0
			w.startFall(p, 0, 0)
			return
		}
		topY = float64(win.Rect.Min.Y)
	} else {
		topY = p.ClimbGoalY
		if topY < float64(w.WorkArea.Min.Y)+size*1.2 {
			topY = float64(w.WorkArea.Min.Y) + size*1.2
		}
	}

	// Rhythmic hand-over-hand climb cycle!
	freq := 6.6
	baseSpd := 76.0 * w.Scale
	switch p.CharID {
	case "inosuke", "akaza":
		freq, baseSpd = 8.2, 106.0*w.Scale
	case "kyojuro":
		freq, baseSpd = 7.4, 92.0*w.Scale
	case "manggom":
		freq, baseSpd = 5.6, 60.0*w.Scale
	}
	prevSin := math.Sin(p.ClimbPhase)
	p.ClimbPhase += dt * freq
	curSin := math.Sin(p.ClimbPhase)

	// Hand-over-hand pull surge: each hand grab produces a crisp upward heave + subtle wall-grip puff!
	pull := math.Abs(curSin)
	climbSpeed := baseSpd * (0.28 + 1.15*pull)
	p.Y -= climbSpeed * dt
	p.FacingLeft = p.ClimbFaceDir < 0 // Right wall -> face right; Left wall -> face left!
	p.Tilt = p.ClimbFaceDir * (0.04 * curSin)
	p.Squash = -0.05*pull + 0.03*(1-pull)

	// Emit tiny wall-grip contact puff each time a hand grabs higher on the window edge!
	if (prevSin <= 0 && curSin > 0) || (prevSin >= 0 && curSin < 0) {
		gripX := p.X + p.ClimbFaceDir*size*0.24
		gripY := p.Y - size*0.42
		if p.CharID == "zenitsu" && w.rng.Float64() < 0.45 {
			w.spawnParticleCustom(p.X-p.ClimbFaceDir*size*0.25, p.Y-size*0.72, -p.ClimbFaceDir*35, -30, 0.48, 5.5, ParticleSweat, color.NRGBA{135, 215, 255, 225})
		} else if p.CharID == "kyojuro" && w.rng.Float64() < 0.4 {
			w.spawnParticleCustom(gripX, gripY, -p.ClimbFaceDir*25, 20, 0.4, 5.5, ParticleFlame, color.NRGBA{255, 145, 45, 215})
		} else if w.rng.Float64() < 0.55 {
			w.spawnParticleCustom(gripX, gripY, -p.ClimbFaceDir*22, 18, 0.35, 4.2, ParticleDust, color.NRGBA{236, 232, 226, 185})
		}
	}

	if p.Y <= topY {
		p.Y = topY
		if p.AttachedWinID != 0 {
			// Smooth parabolic corner vault (StateMantle) onto the top of the window!
			endX := float64(attachedWin.Rect.Min.X) + size*0.38
			if p.ClimbFaceDir < 0 {
				endX = float64(attachedWin.Rect.Max.X) - size*0.38
			}
			p.State = StateMantle
			p.MantleStartX = p.X
			p.MantleEndX = endX
			p.MantleTopY = float64(attachedWin.Rect.Min.Y)
			p.MantleT = 0
			p.Squash = -0.18
			w.spawnParticleCustom(p.X+p.ClimbFaceDir*size*0.2, p.Y-size*0.3, -p.ClimbFaceDir*30, -35, 0.48, 6.5, ParticleSparkle, color.NRGBA{255, 225, 100, 235})
			return
		}
		// Screen edge apex: always jump heroically off the wall!
		w.startJumpPrep(p, -p.ClimbFaceDir)
	}
}

func (w *World) stepMantle(p *Pet, dt float64) {
	if p.AttachedWinID != 0 {
		win, winIdx, ok := w.findWindowWithIndex(p.AttachedWinID)
		if !ok || win.Occluding || w.isPointOccluded(winIdx, int(p.MantleEndX), win.Rect.Min.Y+4) {
			p.AttachedWinID = 0
			w.startFall(p, 0, 0)
			return
		}
	}
	const dur = 0.26
	p.MantleT += dt / dur
	if p.MantleT >= 1.0 {
		p.X = p.MantleEndX
		p.Y = p.MantleTopY
		p.Tilt = 0
		p.Squash = 0.24
		w.startWalk(p, p.ClimbFaceDir)
		return
	}
	t := p.MantleT
	ease := t * t * (3 - 2*t)
	arcH := 28.0 * w.Scale * math.Sin(t*math.Pi)
	p.X = p.MantleStartX + (p.MantleEndX-p.MantleStartX)*ease
	p.Y = p.MantleTopY - arcH
	p.Tilt = p.ClimbFaceDir * (0.24 * (1 - t))
}

func (w *World) startJumpPrep(p *Pet, dirX float64) {
	if w.hasStackChild(p.ID) {
		return // Don't jump off when carrying a friend on your head!
	}
	if dirX == 0 {
		dirX = 1
		if p.FacingLeft {
			dirX = -1
		}
	}
	p.StackParentID = 0
	p.State = StateJumpPrep
	p.JumpDirX = dirX
	p.FacingLeft = dirX < 0
	p.StateTimer = 0.22 // cute anticipation crouch before leaping!
	p.VX = 0
	p.TargetVX = 0
	p.ReactTimer = 0
	p.RubActiveTimer = 0
	p.RubMeter = 0
	p.RubAccum = 0
}

func (w *World) stepJumpPrep(p *Pet, dt float64) {
	if p.AttachedWinID != 0 {
		win, winIdx, ok := w.findWindowWithIndex(p.AttachedWinID)
		if !ok || win.Occluding || w.isPointOccluded(winIdx, int(p.X), win.Rect.Min.Y+4) {
			p.AttachedWinID = 0
			w.startFall(p, 0, 0)
			return
		}
	}
	// Crouch down and lean outward toward the jump!
	p.Squash = 0.28
	p.Tilt = p.JumpDirX * 0.18
	p.StateTimer -= dt
	if p.StateTimer <= 0 {
		// Launch into parabolic jump arc!
		p.IgnoreWinID = p.AttachedWinID
		p.IgnoreWinTime = 0.45
		p.AttachedWinID = 0
		p.State = StateJumpArc
		p.VX = p.JumpDirX * (195.0 + w.rng.Float64()*75.0) * w.Scale
		p.VY = -(340.0 + w.rng.Float64()*85.0) * w.Scale
		p.Squash = -0.26
		p.Tilt = p.JumpDirX * 0.22
		w.spawnParticleCustom(p.X, p.Y-6, -p.JumpDirX*45, -25, 0.42, 7.5, ParticleDust, color.NRGBA{240, 236, 230, 210})
		w.spawnParticleCustom(p.X, p.Y-w.PetSize()*0.4, p.JumpDirX*30, -60, 0.5, 7.0, ParticleSparkle, color.NRGBA{255, 225, 100, 235})
	}
}

func (w *World) stepJumpArc(p *Pet, dt, floorY float64) {
	p.VY += 1350.0 * w.Scale * dt
	p.X += p.VX * dt
	p.Y += p.VY * dt
	p.Tilt = clampF(p.VX*0.0009, -0.30, 0.30)
	w.clampPetX(p)

	// Once past apex and heading downward, hand off seamlessly to character-specific falling animation!
	if p.VY >= 75.0*w.Scale {
		w.startFall(p, p.VX, p.VY)
		return
	}
	if p.Y >= floorY {
		p.Y = floorY
		p.State = StateIdle
		p.StateTimer = 2.0 + w.rng.Float64()*3.5
		p.VX, p.VY, p.Tilt = 0, 0, 0
		w.triggerCharacterLandingFX(p, false)
	}
}

// StartClimbWindow triggers a pet to climb the given window ID (useful for tests & parkour).
func (w *World) StartClimbWindow(petID int, winID uintptr, leftEdge bool) {
	p := w.FindPet(petID)
	if p == nil {
		return
	}
	faceDir := 1.0 // left edge of window -> wall is to pet's right
	if !leftEdge {
		faceDir = -1.0 // right edge of window -> wall is to pet's left
	}
	w.startClimbUp(p, winID, faceDir)
}

// StartJumpOff triggers a pet on a window or wall to crouch and jump off!
func (w *World) StartJumpOff(petID int, dirX float64) {
	p := w.FindPet(petID)
	if p == nil {
		return
	}
	w.startJumpPrep(p, dirX)
}

func (w *World) findWindowWallNear(x, y, tol float64) (WindowRect, float64, bool) {
	size := w.PetSize()
	for i, win := range w.Windows {
		if win.Occluding {
			continue
		}
		// Window must have room above its top edge for a pet to stand on!
		if win.Rect.Min.Y < w.WorkArea.Min.Y+int(size*0.75) {
			continue
		}
		if y < float64(win.Rect.Min.Y)+size*0.2 || y > float64(win.Rect.Max.Y)+size*0.35 {
			continue
		}
		if math.Abs(x-float64(win.Rect.Min.X)) <= tol {
			if w.isWallSegmentVisible(i, win.Rect.Min.X, 1.0, int(y), win.Rect.Min.Y) {
				return win, 1.0, true
			}
		}
		if math.Abs(x-float64(win.Rect.Max.X)) <= tol {
			if w.isWallSegmentVisible(i, win.Rect.Max.X, -1.0, int(y), win.Rect.Min.Y) {
				return win, -1.0, true
			}
		}
	}
	return WindowRect{}, 0, false
}

func (w *World) findWindowWallHit(p *Pet, prevX, curX float64) (WindowRect, float64, bool) {
	size := w.PetSize()
	for i, win := range w.Windows {
		if win.Occluding || win.ID == p.IgnoreWinID || win.ID == p.AttachedWinID {
			continue
		}
		// Must have room above top edge & reach down close enough to the taskbar for pet to grab!
		if win.Rect.Min.Y < w.WorkArea.Min.Y+int(size*0.80) {
			continue
		}
		if float64(win.Rect.Max.Y) < p.Y-size*0.85 {
			continue
		}
		if float64(win.Rect.Min.Y) > p.Y-size*0.65 {
			continue
		}
		// Approaching left wall while walking right (only if NOT occluded by a foreground window!)
		leftWall := float64(win.Rect.Min.X) - size*0.24
		if p.VX > 0 && prevX <= leftWall && curX >= leftWall {
			if w.isWallSegmentVisible(i, win.Rect.Min.X, 1.0, int(p.Y), win.Rect.Min.Y) {
				return win, 1.0, true
			}
		}
		// Approaching right wall while walking left (only if NOT occluded by a foreground window!)
		rightWall := float64(win.Rect.Max.X) + size*0.24
		if p.VX < 0 && prevX >= rightWall && curX <= rightWall {
			if w.isWallSegmentVisible(i, win.Rect.Max.X, -1.0, int(p.Y), win.Rect.Min.Y) {
				return win, -1.0, true
			}
		}
	}
	return WindowRect{}, 0, false
}

func (w *World) pickClimbableWindowEdgeX(p *Pet) (float64, bool) {
	size := w.PetSize()
	bestDist := 1e9
	bestX := 0.0
	found := false
	for i, win := range w.Windows {
		if win.Occluding {
			continue
		}
		if win.Rect.Min.Y < w.WorkArea.Min.Y+int(size*0.80) {
			continue
		}
		if float64(win.Rect.Max.Y) < p.Y-size*0.85 || float64(win.Rect.Min.Y) > p.Y-size*0.65 {
			continue
		}
		type cand struct {
			edge    float64
			wallX   int
			faceDir float64
		}
		cands := [2]cand{
			{float64(win.Rect.Min.X) - size*0.24, win.Rect.Min.X, 1.0},
			{float64(win.Rect.Max.X) + size*0.24, win.Rect.Max.X, -1.0},
		}
		for _, c := range cands {
			if c.edge < float64(w.WorkArea.Min.X)+size*0.35 || c.edge > float64(w.WorkArea.Max.X)-size*0.35 {
				continue
			}
			if !w.isWallSegmentVisible(i, c.wallX, c.faceDir, int(p.Y), win.Rect.Min.Y) {
				continue
			}
			d := math.Abs(c.edge - p.X)
			if d < bestDist {
				bestDist = d
				bestX = c.edge
				found = true
			}
		}
	}
	return bestX, found
}

func (w *World) findLandingWindowTop(p *Pet, prevY, curY float64) (WindowRect, bool) {
	size := w.PetSize()
	margin := size * 0.25
	for i, win := range w.Windows {
		if win.Occluding || win.ID == p.IgnoreWinID {
			continue
		}
		if win.Rect.Min.Y < w.WorkArea.Min.Y+int(size*0.75) {
			continue
		}
		topY := float64(win.Rect.Min.Y)
		if prevY <= topY+6 && curY >= topY {
			if p.X >= float64(win.Rect.Min.X)+margin && p.X <= float64(win.Rect.Max.X)-margin {
				if !w.isPointOccluded(i, int(p.X), win.Rect.Min.Y+4) {
					return win, true
				}
			}
		}
	}
	return WindowRect{}, false
}

func (w *World) triggerCharacterLandingFX(p *Pet, onHead bool) {
	size := w.PetSize()
	scaleMul := 1.0
	if onHead {
		scaleMul = 0.72
	}
	y := p.Y - 2

	switch p.CharID {
	case "manggom":
		// GIANT squishy mochi pancake splat-bounce!
		p.Squash = 0.56
		p.SquashVel = 1.4
		w.addGroundEffect(p.X, y, size*0.72*scaleMul, 0.48, EffectMochiRing, color.NRGBA{255, 180, 205, 220})
		w.spawnReactionParticles(p, 5)
	case "tanjiro":
		// Water Breathing glowing wave splash!
		p.Squash = 0.26
		w.addGroundEffect(p.X, y, size*0.88*scaleMul, 0.56, EffectWaterWave, color.NRGBA{55, 195, 255, 245})
		w.spawnReactionParticles(p, 8)
	case "kyojuro":
		// Fiery golden-orange flame shockwave!
		p.Squash = 0.28
		w.addGroundEffect(p.X, y, size*0.88*scaleMul, 0.52, EffectFlameRing, color.NRGBA{255, 115, 25, 240})
		w.spawnReactionParticles(p, 8)
	case "zenitsu":
		// Electric Thunderclap Flash impact burst!
		p.Squash = 0.26
		w.addGroundEffect(p.X, y, size*0.86*scaleMul, 0.46, EffectLightningRing, color.NRGBA{255, 228, 45, 245})
		w.spawnReactionParticles(p, 8)
	case "inosuke":
		// Boar smash dust cloud + proud pink 호와호와!
		p.Squash = 0.36
		w.addGroundEffect(p.X, y, size*0.82*scaleMul, 0.5, EffectDustPuff, color.NRGBA{240, 232, 222, 235})
		for k := 0; k < 5; k++ {
			dir := -1.0
			if k%2 == 1 {
				dir = 1.0
			}
			w.spawnParticleCustom(p.X+dir*size*0.25, p.Y-size*0.15, dir*(70+w.rng.Float64()*80), -35-w.rng.Float64()*45, 0.65, 8.5, ParticleDust, color.NRGBA{235, 226, 215, 225})
		}
		w.spawnReactionParticles(p, 4)
	case "akaza":
		// Iconic glowing sky-blue Snowflake Compass Needle shockwave!
		p.Squash = 0.24
		w.addGroundEffect(p.X, y, size*0.95*scaleMul, 0.68, EffectSnowCompass, color.NRGBA{100, 225, 255, 250})
		w.spawnReactionParticles(p, 7)
	default:
		p.Squash = 0.25
	}
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
