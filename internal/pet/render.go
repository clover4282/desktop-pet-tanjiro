package pet

import (
	"image"
	"image/color"
	"math"
)

// RenderBounds computes the tight screen rectangle containing all active pets, claws, effects, and particles.
func (w *World) RenderBounds() image.Rectangle {
	if len(w.Pets) == 0 && len(w.DetachedClaws) == 0 && len(w.Particles) == 0 && len(w.Effects) == 0 {
		return image.Rectangle{}
	}
	size := w.PetSize()
	pad := int(math.Ceil(size * 0.45))

	minX, minY := 1<<30, 1<<30
	maxX, maxY := -(1 << 30), -(1 << 30)

	expand := func(x0, y0, x1, y1 int) {
		if x0 < minX {
			minX = x0
		}
		if y0 < minY {
			minY = y0
		}
		if x1 > maxX {
			maxX = x1
		}
		if y1 > maxY {
			maxY = y1
		}
	}

	for _, p := range w.Pets {
		halfW := int(math.Ceil(size * 0.85))
		h := int(math.Ceil(size * 1.45))
		cx, by := int(math.Round(p.X)), int(math.Round(p.Y))
		expand(cx-halfW, by-h, cx+halfW, by+pad/2)
		if sprite, scaleX, scaleY, drawX, drawY, angle := w.petDrawParams(p); sprite != nil && scaleX > 0 && scaleY > 0 {
			hw := float64(sprite.W) * 0.5 * scaleX
			sh := float64(sprite.H) * scaleY
			if math.Abs(angle) < 1e-4 {
				expand(
					int(math.Floor(drawX-hw)),
					int(math.Floor(drawY-sh)),
					int(math.Ceil(drawX+hw)),
					int(math.Ceil(drawY)),
				)
			} else {
				cosA := math.Cos(angle)
				sinA := math.Sin(angle)
				corners := [4][2]float64{
					{-hw, -sh},
					{hw, -sh},
					{-hw, 0},
					{hw, 0},
				}
				for _, c := range corners {
					rx := drawX + c[0]*cosA - c[1]*sinA
					ry := drawY + c[0]*sinA + c[1]*cosA
					expand(int(math.Floor(rx)), int(math.Floor(ry)), int(math.Ceil(rx)), int(math.Ceil(ry)))
				}
			}
		}
		if p.State == StateClawSpawn || p.State == StateClawDismiss {
			clawR := int(math.Ceil(size * 0.75))
			cy := int(math.Round(p.ClawY))
			expand(cx-clawR, cy-clawR, cx+clawR, cy+clawR)
		}
	}
	for _, c := range w.DetachedClaws {
		clawR := int(math.Ceil(size * 0.75))
		cx, cy := int(math.Round(c.X)), int(math.Round(c.Y))
		expand(cx-clawR, cy-clawR, cx+clawR, cy+clawR)
	}
	for _, fx := range w.Effects {
		r := int(math.Ceil(fx.MaxR * 1.25))
		cx, cy := int(math.Round(fx.X)), int(math.Round(fx.Y))
		expand(cx-r, cy-r, cx+r, cy+r/2+12)
	}
	for _, pt := range w.Particles {
		r := int(math.Ceil(pt.Size * 1.8))
		cx, cy := int(math.Round(pt.X)), int(math.Round(pt.Y))
		expand(cx-r, cy-r, cx+r, cy+r)
	}

	rect := image.Rect(minX-8, minY-8, maxX+8, maxY+8)
	// Clip to full monitor rect (allowing tiny bottom overhang onto taskbar edge if desired)
	clip := w.Screen
	if clip.Empty() {
		clip = w.WorkArea
	}
	rect = rect.Intersect(clip)
	if rect.Empty() {
		return image.Rectangle{}
	}
	return rect
}

// HitTest returns the ID of the topmost pet under (screenX, screenY), checking actual sprite alpha.
func (w *World) HitTest(screenX, screenY float64) int {
	maxReach := w.PetSize() * 1.65
	for i := len(w.Pets) - 1; i >= 0; i-- {
		p := w.Pets[i]
		if p.State == StateClawSpawn || p.State == StateClawDismiss {
			continue
		}
		if math.Abs(screenX-p.X) > maxReach || math.Abs(screenY-p.Y) > maxReach {
			continue
		}
		sprite, scaleX, scaleY, drawX, drawY, angle := w.petDrawParams(p)
		if sprite == nil || scaleX <= 0 || scaleY <= 0 {
			continue
		}
		// Inverse transform around bottom-center pivot (drawX, drawY)
		dx := screenX - drawX
		dy := screenY - drawY
		var lx, ly float64
		if math.Abs(angle) < 1e-4 {
			lx, ly = dx, dy
		} else {
			cosA := math.Cos(-angle)
			sinA := math.Sin(-angle)
			lx = dx*cosA - dy*sinA
			ly = dx*sinA + dy*cosA
		}

		sw := float64(sprite.W)
		sh := float64(sprite.H)
		u := lx/scaleX + sw*0.5
		if p.FacingLeft {
			u = sw - u
		}
		v := ly/scaleY + sh
		ix, iy := int(math.Floor(u)), int(math.Floor(v))
		if ix >= 0 && ix < sprite.W && iy >= 0 && iy < sprite.H {
			if sprite.Pix[iy*sprite.Stride+ix*4+3] > 20 {
				return p.ID
			}
		}
	}
	return 0
}

func (w *World) petDrawParams(p *Pet) (sprite *PremulSprite, scaleX, scaleY, drawX, drawY, angle float64) {
	if w.Bank == nil {
		return nil, 1, 1, p.X, p.Y, 0
	}
	ss := w.Bank.ByID[p.CharID]
	if ss == nil {
		return nil, 1, 1, p.X, p.Y, 0
	}
	pose := PoseIdle
	extraScale := 1.0
	switch {
	case p.State == StateGrabbed || p.State == StateClawSpawn || (p.State == StateClawDismiss && p.ClawGrabbed) ||
		p.State == StateJumpArc:
		pose = PoseGrabbed
	case p.State == StateWalk:
		if math.Sin(p.WalkPhase) >= 0 {
			pose = PoseWalk1
		} else {
			pose = PoseWalk2
		}
	case p.State == StateClimbUp:
		if math.Sin(p.ClimbPhase) >= 0 {
			pose = PoseClimb1
		} else {
			pose = PoseClimb2
		}
	case p.State == StateMantle:
		pose = PoseClimb1
	case p.State == StateJumpPrep:
		pose = PoseIdle
	case p.State == StateFall:
		switch p.CharID {
		case "zenitsu":
			if p.FallTime < 0.16 {
				pose = PoseGrabbed
			} else {
				pose = PoseReact
				extraScale = ss.Def.ReactScale
			}
		case "akaza":
			if p.FallTime >= 0.15 {
				pose = PoseReact
				extraScale = ss.Def.ReactScale
			} else {
				pose = PoseGrabbed
			}
		case "tanjiro":
			if p.FallTime >= 0.18 {
				pose = PoseReact
				extraScale = ss.Def.ReactScale
			} else {
				pose = PoseGrabbed
			}
		default:
			pose = PoseGrabbed
		}
	case p.RubActiveTimer > 0:
		switch p.CharID {
		case "akaza":
			if p.RubMeter < 0.55 {
				pose = PoseGrabbed // tsundere blushing pout!
			} else {
				pose = PoseReact
				extraScale = ss.Def.ReactScale
			}
		case "zenitsu":
			if p.RubMeter < 0.45 {
				pose = PoseGrabbed // flustered panic!
			} else {
				pose = PoseReact
				extraScale = ss.Def.ReactScale
			}
		default:
			if p.RubMeter >= 0.22 {
				pose = PoseReact
				extraScale = ss.Def.ReactScale
			}
		}
	case p.ReactTimer > 0:
		pose = PoseReact
		extraScale = ss.Def.ReactScale
	}
	sprite = ss.Premul[pose]
	sw := float64(sprite.W)
	baseScale := (w.PetSize() / sw) * extraScale

	// Gentle idle breathing + squash/stretch + character-specific rubbing bounce
	breathe := 0.0
	if p.State == StateIdle || p.State == StateStacked {
		breathe = math.Sin(p.AnimTime*2.8+float64(p.ID)*1.3) * 0.022
	}
	if p.ReactTimer > 0 {
		breathe += math.Sin(p.AnimTime*9.5) * 0.032
	}
	if p.RubActiveTimer > 0 {
		breathe += math.Sin(p.AnimTime*15.0) * 0.045 * (0.4 + p.RubMeter)
	}

	sq := clampF(p.Squash, -0.45, 0.62)
	sx := baseScale * (1.0 + sq*0.42 - breathe*0.5)
	sy := baseScale * (1.0 - sq*0.38 + breathe)

	hopY := 0.0
	if p.State == StateWalk {
		gait := characterGait(p.CharID)
		speedFrac := clampF(math.Abs(p.VX)/(gait.speed*w.Scale), 0, 1.2)
		// Parabolic step lift synchronized directly with distance-based WalkPhase!
		stepSin := math.Abs(math.Sin(p.WalkPhase))
		hopY = math.Pow(stepSin, 0.85) * gait.hop * w.Scale * speedFrac
	} else if p.RubActiveTimer > 0 && (p.CharID == "kyojuro" || p.CharID == "inosuke") {
		hopY = math.Abs(math.Sin(p.AnimTime*10.0)) * 6.0 * w.Scale
	} else if p.ReactTimer > 0 {
		hopY = math.Abs(math.Sin(p.AnimTime*6.0)) * 4.0 * w.Scale
	}

	return sprite, sx, sy, p.X + p.JitterX, p.Y - hopY, p.Tilt
}

// RenderBGRA renders the visible world into a tightly packed premultiplied BGRA byte slice.
func (w *World) RenderBGRA(buf []byte, width, height, originX, originY int) {
	w.RenderBGRAWithStride(buf, width*4, width, height, originX, originY)
}

// RenderBGRAWithStride renders the visible world into a premultiplied BGRA buffer
// with row stride strideBytes (useful for reused Win32 DIBSections).
func (w *World) RenderBGRAWithStride(buf []byte, strideBytes, width, height, originX, originY int) {
	if width <= 0 || height <= 0 || strideBytes < width*4 {
		return
	}
	if len(buf) < (height-1)*strideBytes+width*4 {
		return
	}
	// Clear only the active width x height region
	rowBytes := width * 4
	for y := 0; y < height; y++ {
		off := y * strideBytes
		clear(buf[off : off+rowBytes])
	}

	ox := float64(originX)
	oy := float64(originY)

	// Draw ground/landing shockwave & compass effects beneath pets
	for _, fx := range w.Effects {
		drawGroundEffectBGRA(buf, strideBytes, width, height, fx, ox, oy)
	}

	// Draw pets in painter order (bottom-most first, topmost/stacked last)
	for _, p := range w.Pets {
		if p.State == StateIdle || p.State == StateWalk || p.State == StateJumpPrep {
			drawGroundShadowBGRA(buf, strideBytes, width, height, p, w.PetSize(), ox, oy)
		}
		if p.RubMeter > 0.18 {
			drawRubAuraBGRA(buf, strideBytes, width, height, p, w.PetSize(), ox, oy)
		}
		sprite, scaleX, scaleY, drawX, drawY, angle := w.petDrawParams(p)
		if sprite != nil {
			blitSpriteBGRA(buf, strideBytes, width, height, sprite, drawX-ox, drawY-oy, scaleX, scaleY, angle, p.FacingLeft)
		}
		if p.State == StateClawSpawn || p.State == StateClawDismiss {
			drawPropellerClawBGRA(buf, strideBytes, width, height, p.X-ox, p.ClawY-oy, w.PetSize(), p.AnimTime, p.ClawOpen)
		}
	}

	// Draw detached ascending claws
	for _, c := range w.DetachedClaws {
		drawPropellerClawBGRA(buf, strideBytes, width, height, c.X-ox, c.Y-oy, w.PetSize(), c.AnimTime, c.Open)
	}

	// Draw particles on top
	for _, pt := range w.Particles {
		drawParticleBGRA(buf, strideBytes, width, height, pt, ox, oy)
	}
}

// RenderRGBA convenience helper (used for macOS visual verification & tests).
func (w *World) RenderRGBA(bounds image.Rectangle) *image.RGBA {
	bw, bh := bounds.Dx(), bounds.Dy()
	bgra := make([]byte, bw*bh*4)
	w.RenderBGRA(bgra, bw, bh, bounds.Min.X, bounds.Min.Y)

	out := image.NewRGBA(image.Rect(0, 0, bw, bh))
	for i := 0; i < len(bgra); i += 4 {
		// BGRA -> RGBA (both premultiplied)
		out.Pix[i+0] = bgra[i+2]
		out.Pix[i+1] = bgra[i+1]
		out.Pix[i+2] = bgra[i+0]
		out.Pix[i+3] = bgra[i+3]
	}
	return out
}

func drawGroundShadowBGRA(dst []byte, stride, dw, dh int, p *Pet, petSize, ox, oy float64) {
	px := p.X - ox
	py := p.Y - oy - 2.0
	rx := petSize * 0.30
	ry := petSize * 0.065
	alpha := 52.0
	if p.State == StateWalk {
		lift := math.Abs(math.Sin(p.WalkPhase))
		rx *= 1.0 - lift*0.18
		alpha *= 1.0 - lift*0.30
	} else if p.State == StateJumpPrep {
		rx *= 1.15
		alpha = 68.0
	}
	c := color.NRGBA{30, 22, 24, clampByte(alpha)}
	x0 := maxI(0, int(math.Floor(px-rx-2)))
	x1 := minI(dw-1, int(math.Ceil(px+rx+2)))
	y0 := maxI(0, int(math.Floor(py-ry-2)))
	y1 := minI(dh-1, int(math.Ceil(py+ry+2)))
	for y := y0; y <= y1; y++ {
		ny := (float64(y) + 0.5 - py) / ry
		row := y * stride
		for x := x0; x <= x1; x++ {
			nx := (float64(x) + 0.5 - px) / rx
			d := math.Hypot(nx, ny)
			if d < 1.0 {
				cov := clampF((1.0-d)*1.6, 0, 1)
				blendPixelBGRA(dst, row+x*4, c, cov)
			}
		}
	}
}

// blitSpriteBGRA blits a premultiplied BGRA sprite onto a premultiplied BGRA buffer with bilinear filtering,
// pivot at bottom-center (px, py).
func blitSpriteBGRA(dst []byte, stride, dw, dh int, src *PremulSprite, px, py, scaleX, scaleY, angle float64, flipX bool) {
	if src == nil || src.W <= 0 || src.H <= 0 || scaleX <= 0 || scaleY <= 0 || dw <= 0 || dh <= 0 {
		return
	}
	sw, sh := src.W, src.H
	fsw, fsh := float64(sw), float64(sh)
	invScaleX := 1.0 / scaleX
	invScaleY := 1.0 / scaleY
	srcPix := src.Pix
	srcStride := src.Stride

	// Upright fast-path (most idle/standing/stacked frames have angle == 0):
	// avoids trig/rotation math and hoists Y-axis bilinear weights out of the inner X loop.
	if math.Abs(angle) < 1e-4 {
		halfW := fsw * 0.5 * scaleX
		x0 := maxI(0, int(math.Floor(px-halfW))-1)
		y0 := maxI(0, int(math.Floor(py-fsh*scaleY))-1)
		x1 := minI(dw-1, int(math.Ceil(px+halfW))+1)
		y1 := minI(dh-1, int(math.Ceil(py))+1)

		for y := y0; y <= y1; y++ {
			v := (float64(y)+0.5-py)*invScaleY + fsh - 0.5
			if v < -0.5 || v > fsh-0.5 {
				continue
			}
			iy := int(math.Floor(v))
			fy := v - float64(iy)
			wy0 := 1.0 - fy
			wy1 := fy
			y0Valid := iy >= 0 && iy < sh
			y1Valid := iy+1 >= 0 && iy+1 < sh
			sRow0 := iy * srcStride
			sRow1 := (iy + 1) * srcStride
			rowOff := y * stride

			for x := x0; x <= x1; x++ {
				u := (float64(x)+0.5-px)*invScaleX + fsw*0.5 - 0.5
				if flipX {
					u = fsw - 1.0 - u
				}
				if u < -0.5 || u > fsw-0.5 {
					continue
				}
				ix := int(math.Floor(u))
				fx := u - float64(ix)
				wx0 := 1.0 - fx
				wx1 := fx

				var pb, pg, pr, pa float64
				if y0Valid {
					if ix >= 0 && ix < sw {
						si := sRow0 + ix*4
						if srcPix[si+3] != 0 {
							wt := wx0 * wy0
							pb += float64(srcPix[si+0]) * wt
							pg += float64(srcPix[si+1]) * wt
							pr += float64(srcPix[si+2]) * wt
							pa += float64(srcPix[si+3]) * wt
						}
					}
					if ix+1 >= 0 && ix+1 < sw {
						si := sRow0 + (ix+1)*4
						if srcPix[si+3] != 0 {
							wt := wx1 * wy0
							pb += float64(srcPix[si+0]) * wt
							pg += float64(srcPix[si+1]) * wt
							pr += float64(srcPix[si+2]) * wt
							pa += float64(srcPix[si+3]) * wt
						}
					}
				}
				if y1Valid {
					if ix >= 0 && ix < sw {
						si := sRow1 + ix*4
						if srcPix[si+3] != 0 {
							wt := wx0 * wy1
							pb += float64(srcPix[si+0]) * wt
							pg += float64(srcPix[si+1]) * wt
							pr += float64(srcPix[si+2]) * wt
							pa += float64(srcPix[si+3]) * wt
						}
					}
					if ix+1 >= 0 && ix+1 < sw {
						si := sRow1 + (ix+1)*4
						if srcPix[si+3] != 0 {
							wt := wx1 * wy1
							pb += float64(srcPix[si+0]) * wt
							pg += float64(srcPix[si+1]) * wt
							pr += float64(srcPix[si+2]) * wt
							pa += float64(srcPix[si+3]) * wt
						}
					}
				}
				if pa < 0.5 {
					continue
				}

				di := rowOff + x*4
				invA := 1.0 - pa/255.0
				dst[di+0] = clampByte(pb + float64(dst[di+0])*invA)
				dst[di+1] = clampByte(pg + float64(dst[di+1])*invA)
				dst[di+2] = clampByte(pr + float64(dst[di+2])*invA)
				dst[di+3] = clampByte(pa + float64(dst[di+3])*invA)
			}
		}
		return
	}

	// General rotated path
	cosA := math.Cos(angle)
	sinA := math.Sin(angle)
	corners := [4][2]float64{
		{-fsw * 0.5 * scaleX, -fsh * scaleY},
		{fsw * 0.5 * scaleX, -fsh * scaleY},
		{-fsw * 0.5 * scaleX, 0},
		{fsw * 0.5 * scaleX, 0},
	}
	minX, minY := 1e9, 1e9
	maxX, maxY := -1e9, -1e9
	for _, c := range corners {
		rx := px + c[0]*cosA - c[1]*sinA
		ry := py + c[0]*sinA + c[1]*cosA
		if rx < minX {
			minX = rx
		}
		if rx > maxX {
			maxX = rx
		}
		if ry < minY {
			minY = ry
		}
		if ry > maxY {
			maxY = ry
		}
	}

	x0 := maxI(0, int(math.Floor(minX))-1)
	y0 := maxI(0, int(math.Floor(minY))-1)
	x1 := minI(dw-1, int(math.Ceil(maxX))+1)
	y1 := minI(dh-1, int(math.Ceil(maxY))+1)

	for y := y0; y <= y1; y++ {
		dy := float64(y) + 0.5 - py
		dySin := dy * sinA
		dyCos := dy * cosA
		rowOff := y * stride
		for x := x0; x <= x1; x++ {
			dx := float64(x) + 0.5 - px
			// Inverse rotate
			lx := dx*cosA + dySin
			ly := -dx*sinA + dyCos

			u := lx*invScaleX + fsw*0.5 - 0.5
			if flipX {
				u = fsw - 1.0 - u
			}
			v := ly*invScaleY + fsh - 0.5
			if u < -0.5 || u > fsw-0.5 || v < -0.5 || v > fsh-0.5 {
				continue
			}

			// Bilinear sample in premultiplied BGRA space
			ix := int(math.Floor(u))
			iy := int(math.Floor(v))
			fx := u - float64(ix)
			fy := v - float64(iy)

			var pb, pg, pr, pa float64
			for oy := 0; oy <= 1; oy++ {
				sy := iy + oy
				if sy < 0 || sy >= sh {
					continue
				}
				wy := 1.0 - fy
				if oy == 1 {
					wy = fy
				}
				sRow := sy * srcStride
				for ox := 0; ox <= 1; ox++ {
					sx := ix + ox
					if sx < 0 || sx >= sw {
						continue
					}
					wx := 1.0 - fx
					if ox == 1 {
						wx = fx
					}
					si := sRow + sx*4
					if srcPix[si+3] == 0 {
						continue
					}
					wt := wx * wy
					pb += float64(srcPix[si+0]) * wt
					pg += float64(srcPix[si+1]) * wt
					pr += float64(srcPix[si+2]) * wt
					pa += float64(srcPix[si+3]) * wt
				}
			}
			if pa < 0.5 {
				continue
			}

			// Porter-Duff source-over onto premultiplied BGRA
			di := rowOff + x*4
			invA := 1.0 - pa/255.0
			dst[di+0] = clampByte(pb + float64(dst[di+0])*invA)
			dst[di+1] = clampByte(pg + float64(dst[di+1])*invA)
			dst[di+2] = clampByte(pr + float64(dst[di+2])*invA)
			dst[di+3] = clampByte(pa + float64(dst[di+3])*invA)
		}
	}
}

// drawPropellerClawBGRA renders the adorable spinning golden propeller + metallic claw from the Naver Clip!
func drawPropellerClawBGRA(dst []byte, stride, dw, dh int, cx, cy, petSize, animTime, openAmt float64) {
	s := petSize / 132.0
	outline := color.NRGBA{72, 48, 38, 255}
	shaftCol := color.NRGBA{210, 218, 226, 255}
	clawCol := color.NRGBA{188, 198, 212, 255}
	goldLight := color.NRGBA{255, 225, 115, 255}
	goldMid := color.NRGBA{248, 188, 68, 255}
	ringCol := color.NRGBA{255, 212, 95, 145}

	// 1. Two curved metallic claw prongs below (cy + 4*s .. cy + 36*s)
	spread := 5.0*s + openAmt*14.0*s
	for _, side := range []float64{-1, 1} {
		// Upper prong segment
		x0, y0 := cx+side*4*s, cy+6*s
		x1, y1 := cx+side*(16*s+spread*0.6), cy+20*s
		x2, y2 := cx+side*(7*s+spread), cy+34*s
		drawCapsuleBGRA(dst, stride, dw, dh, x0, y0, x1, y1, 4.4*s, outline)
		drawCapsuleBGRA(dst, stride, dw, dh, x1, y1, x2, y2, 4.0*s, outline)
		drawCapsuleBGRA(dst, stride, dw, dh, x0, y0, x1, y1, 2.5*s, clawCol)
		drawCapsuleBGRA(dst, stride, dw, dh, x1, y1, x2, y2, 2.2*s, clawCol)
	}

	// 2. Central vertical stem
	drawCapsuleBGRA(dst, stride, dw, dh, cx, cy-24*s, cx, cy+8*s, 4.4*s, outline)
	drawCapsuleBGRA(dst, stride, dw, dh, cx, cy-24*s, cx, cy+8*s, 2.5*s, shaftCol)

	// 3. Claw collar joint
	drawCircleBGRA(dst, stride, dw, dh, cx, cy+6*s, 7.5*s, outline)
	drawCircleBGRA(dst, stride, dw, dh, cx, cy+6*s, 5.2*s, goldMid)

	// 4. Spinning golden motion-blur arcs around the propeller blades (exact video look!)
	propY := cy - 26*s
	phase := animTime * 28.0
	bladeLen := (34.0 + 10.0*math.Abs(math.Cos(phase))) * s
	drawEllipseRingBGRA(dst, stride, dw, dh, cx, propY, 50*s, 11*s, 2.4*s, ringCol)
	drawEllipseRingBGRA(dst, stride, dw, dh, cx, propY, 44*s, 8.5*s, 1.8*s, color.NRGBA{255, 238, 165, 115})

	// 5. Left and right spinning golden rotor blades
	bladeH := 5.8 * s
	drawCapsuleBGRA(dst, stride, dw, dh, cx-bladeLen, propY, cx+bladeLen, propY, bladeH+2.0*s, outline)
	drawCapsuleBGRA(dst, stride, dw, dh, cx-bladeLen, propY, cx+bladeLen, propY, bladeH, goldLight)
	// Inner warm shade on blades
	drawCapsuleBGRA(dst, stride, dw, dh, cx-bladeLen*0.8, propY+1.4*s, cx+bladeLen*0.8, propY+1.4*s, bladeH*0.5, goldMid)

	// 6. Top propeller hub knob
	drawCircleBGRA(dst, stride, dw, dh, cx, propY-5*s, 8.5*s, outline)
	drawCircleBGRA(dst, stride, dw, dh, cx, propY-5*s, 6.2*s, goldMid)
	drawCircleBGRA(dst, stride, dw, dh, cx-1.8*s, propY-6.8*s, 2.5*s, goldLight)
}

func drawParticleBGRA(dst []byte, stride, dw, dh int, pt *Particle, ox, oy float64) {
	t := pt.Life / pt.Max
	alpha := 1.0
	if t < 0.15 {
		alpha = t / 0.15
	} else if t > 0.65 {
		alpha = (1.0 - t) / 0.35
	}
	if alpha <= 0 {
		return
	}
	c := pt.Color
	c.A = clampByte(float64(c.A) * alpha)
	px := pt.X - ox
	py := pt.Y - oy
	r := pt.Size * (0.85 + 0.25*math.Sin(t*math.Pi))

	switch pt.Kind {
	case ParticleHeart:
		// Cute two-lobe heart
		drawCircleBGRA(dst, stride, dw, dh, px-r*0.36, py-r*0.18, r*0.48, c)
		drawCircleBGRA(dst, stride, dw, dh, px+r*0.36, py-r*0.18, r*0.48, c)
		drawCapsuleBGRA(dst, stride, dw, dh, px-r*0.58, py-r*0.02, px, py+r*0.72, r*0.32, c)
		drawCapsuleBGRA(dst, stride, dw, dh, px+r*0.58, py-r*0.02, px, py+r*0.72, r*0.32, c)
	case ParticleLightning:
		// Crisp little lightning bolt
		drawCapsuleBGRA(dst, stride, dw, dh, px+r*0.3, py-r, px-r*0.25, py, r*0.28, c)
		drawCapsuleBGRA(dst, stride, dw, dh, px-r*0.25, py, px+r*0.35, py+r*0.1, r*0.28, c)
		drawCapsuleBGRA(dst, stride, dw, dh, px+r*0.35, py+r*0.1, px-r*0.3, py+r, r*0.25, c)
	case ParticleBubble:
		// Fluffy pink-white cloud bubble
		drawCircleBGRA(dst, stride, dw, dh, px, py, r*0.65, c)
		drawCircleBGRA(dst, stride, dw, dh, px-r*0.4, py+r*0.1, r*0.45, c)
		drawCircleBGRA(dst, stride, dw, dh, px+r*0.4, py+r*0.1, r*0.45, c)
		white := color.NRGBA{255, 255, 255, clampByte(180 * alpha)}
		drawCircleBGRA(dst, stride, dw, dh, px-r*0.15, py-r*0.15, r*0.32, white)
	case ParticleSnowflake:
		// 6-ray glowing ice crystal
		for k := 0; k < 3; k++ {
			a := float64(k) * math.Pi / 3.0
			dx, dy := math.Cos(a)*r, math.Sin(a)*r
			drawCapsuleBGRA(dst, stride, dw, dh, px-dx, py-dy, px+dx, py+dy, r*0.2, c)
		}
		drawCircleBGRA(dst, stride, dw, dh, px, py, r*0.3, color.NRGBA{235, 252, 255, c.A})
	case ParticleFlame:
		drawCircleBGRA(dst, stride, dw, dh, px, py+r*0.2, r*0.62, c)
		drawCapsuleBGRA(dst, stride, dw, dh, px, py+r*0.2, px, py-r*0.85, r*0.32, c)
		drawCircleBGRA(dst, stride, dw, dh, px, py+r*0.22, r*0.32, color.NRGBA{255, 235, 110, c.A})
	case ParticleSweat:
		// Cute anime blue teardrop
		drawCircleBGRA(dst, stride, dw, dh, px, py+r*0.28, r*0.52, c)
		drawCapsuleBGRA(dst, stride, dw, dh, px, py+r*0.2, px, py-r*0.72, r*0.24, c)
		drawCircleBGRA(dst, stride, dw, dh, px-r*0.14, py+r*0.22, r*0.18, color.NRGBA{240, 252, 255, c.A})
	case ParticleDust:
		// Fluffy impact dust puff
		drawCircleBGRA(dst, stride, dw, dh, px, py, r*0.68, c)
		drawCircleBGRA(dst, stride, dw, dh, px-r*0.42, py+r*0.12, r*0.46, c)
		drawCircleBGRA(dst, stride, dw, dh, px+r*0.42, py+r*0.12, r*0.46, c)
	case ParticleWater:
		// Sparkling Water Breathing splash droplet!
		drawCircleBGRA(dst, stride, dw, dh, px, py+r*0.2, r*0.58, c)
		drawCapsuleBGRA(dst, stride, dw, dh, px, py+r*0.18, px, py-r*0.76, r*0.28, c)
		drawCircleBGRA(dst, stride, dw, dh, px-r*0.16, py+r*0.16, r*0.22, color.NRGBA{245, 253, 255, c.A})
	default: // ParticleSparkle (4-point star)
		drawCapsuleBGRA(dst, stride, dw, dh, px-r, py, px+r, py, r*0.22, c)
		drawCapsuleBGRA(dst, stride, dw, dh, px, py-r, px, py+r, r*0.22, c)
		drawCircleBGRA(dst, stride, dw, dh, px, py, r*0.36, color.NRGBA{255, 252, 220, c.A})
	}
}

func drawRubAuraBGRA(dst []byte, stride, dw, dh int, p *Pet, petSize, ox, oy float64) {
	px := p.X - ox
	py := p.Y - oy - petSize*0.48
	strength := clampF((p.RubMeter-0.18)/0.82, 0, 1)
	pulse := 1.0 + 0.08*math.Sin(p.AnimTime*10.0)
	rx := petSize * 0.56 * pulse
	ry := petSize * 0.52 * pulse

	var col color.NRGBA
	switch p.CharID {
	case "tanjiro":
		col = color.NRGBA{65, 205, 245, clampByte(130 * strength)}
	case "kyojuro":
		col = color.NRGBA{255, 140, 40, clampByte(120 * strength)}
	case "zenitsu":
		if p.RubMeter < 0.45 {
			col = color.NRGBA{130, 210, 255, clampByte(95 * strength)}
		} else {
			col = color.NRGBA{255, 230, 65, clampByte(130 * strength)}
		}
	case "inosuke":
		col = color.NRGBA{255, 185, 220, clampByte(130 * strength)}
	case "akaza":
		if p.RubMeter < 0.55 {
			col = color.NRGBA{255, 125, 165, clampByte(85 * strength)}
		} else {
			col = color.NRGBA{105, 225, 255, clampByte(135 * strength)}
		}
	case "manggom":
		col = color.NRGBA{255, 165, 195, clampByte(110 * strength)}
	case "yakgwa_boy", "yakgwa_girl":
		col = color.NRGBA{250, 185, 75, clampByte(125 * strength)}
	default:
		col = color.NRGBA{255, 220, 105, clampByte(115 * strength)}
	}

	drawEllipseRingBGRA(dst, stride, dw, dh, px, py, rx, ry, 4.5*(petSize/132.0), col)
	if p.CharID == "akaza" && p.RubMeter >= 0.55 {
		// Subtle rotating snowflake compass under Akaza when happy!
		drawSnowCompassBGRA(dst, stride, dw, dh, p.X-ox, p.Y-oy-4, petSize*0.58, p.AnimTime*0.8, col)
	}
}

func drawGroundEffectBGRA(dst []byte, stride, dw, dh int, fx *GroundEffect, ox, oy float64) {
	t := fx.Life / fx.Max
	if t <= 0 || t >= 1 {
		return
	}
	ease := 1.0 - (1.0-t)*(1.0-t)
	fade := 1.0 - t
	if t < 0.15 {
		fade = t / 0.15
	}
	c := fx.Color
	c.A = clampByte(float64(c.A) * fade)
	px := fx.X - ox
	py := fx.Y - oy
	rx := fx.MaxR * (0.25 + 0.75*ease)
	ry := rx * 0.30
	thick := math.Max(1.8, (5.5-3.5*t)*(fx.MaxR/90.0))

	switch fx.Kind {
	case EffectWaterWave:
		drawEllipseRingBGRA(dst, stride, dw, dh, px, py, rx, ry, thick*1.25, c)
		foam := color.NRGBA{235, 252, 255, clampByte(float64(c.A) * 0.9)}
		drawEllipseRingBGRA(dst, stride, dw, dh, px, py-2, rx*0.68, ry*0.68, thick*0.75, foam)
	case EffectFlameRing:
		drawEllipseRingBGRA(dst, stride, dw, dh, px, py, rx, ry, thick*1.2, c)
		inner := color.NRGBA{255, 225, 90, clampByte(float64(c.A) * 0.85)}
		drawEllipseRingBGRA(dst, stride, dw, dh, px, py, rx*0.72, ry*0.72, thick*0.8, inner)
	case EffectLightningRing:
		// Jagged golden-cyan electric burst ring
		segs := 10
		for k := 0; k < segs; k++ {
			a0 := float64(k) * 2 * math.Pi / float64(segs)
			a1 := float64(k+1) * 2 * math.Pi / float64(segs)
			jag0 := 1.0 + 0.18*math.Sin(float64(k)*3.7+t*20)
			jag1 := 1.0 + 0.18*math.Sin(float64(k+1)*3.7+t*20)
			x0, y0 := px+math.Cos(a0)*rx*jag0, py+math.Sin(a0)*ry*jag0
			x1, y1 := px+math.Cos(a1)*rx*jag1, py+math.Sin(a1)*ry*jag1
			segCol := c
			if k%2 == 1 {
				segCol = color.NRGBA{115, 230, 255, c.A}
			}
			drawCapsuleBGRA(dst, stride, dw, dh, x0, y0, x1, y1, thick*0.55, segCol)
		}
	case EffectSnowCompass:
		drawSnowCompassBGRA(dst, stride, dw, dh, px, py, rx, t*0.5, c)
	case EffectDustPuff:
		for k := 0; k < 6; k++ {
			a := float64(k) * math.Pi / 3.0
			cx := px + math.Cos(a)*rx*0.85
			cy := py + math.Sin(a)*ry*0.75 - 4*ease
			drawCircleBGRA(dst, stride, dw, dh, cx, cy, thick*2.4*(1.1-0.4*t), c)
		}
	default: // EffectMochiRing
		drawEllipseRingBGRA(dst, stride, dw, dh, px, py, rx, ry, thick*1.1, c)
	}
}

func drawSnowCompassBGRA(dst []byte, stride, dw, dh int, px, py, rx, rot float64, c color.NRGBA) {
	ry := rx * 0.34
	thick := math.Max(1.6, rx*0.032)
	drawEllipseRingBGRA(dst, stride, dw, dh, px, py, rx, ry, thick*1.15, c)
	whiteGlow := color.NRGBA{235, 252, 255, clampByte(float64(c.A) * 0.9)}
	for k := 0; k < 6; k++ {
		a := float64(k)*math.Pi/3.0 + rot
		x1 := px + math.Cos(a)*rx*1.12
		y1 := py + math.Sin(a)*ry*1.12
		drawCapsuleBGRA(dst, stride, dw, dh, px, py, x1, y1, thick*0.65, c)
		drawCapsuleBGRA(dst, stride, dw, dh, px, py, x1, y1, thick*0.32, whiteGlow)
		// Little snowflake crystal barbs near each tip
		bx := px + math.Cos(a)*rx*0.75
		by := py + math.Sin(a)*ry*0.75
		for _, side := range []float64{-0.55, 0.55} {
			tx := bx + math.Cos(a+side)*rx*0.24
			ty := by + math.Sin(a+side)*ry*0.24
			drawCapsuleBGRA(dst, stride, dw, dh, bx, by, tx, ty, thick*0.42, c)
		}
	}
}

func blendPixelBGRA(dst []byte, idx int, c color.NRGBA, cov float64) {
	if cov <= 0 || c.A == 0 {
		return
	}
	sa := (float64(c.A) / 255.0) * cov
	if sa <= 0.002 {
		return
	}
	inv := 1.0 - sa
	dst[idx+0] = clampByte(float64(c.B)*sa + float64(dst[idx+0])*inv)
	dst[idx+1] = clampByte(float64(c.G)*sa + float64(dst[idx+1])*inv)
	dst[idx+2] = clampByte(float64(c.R)*sa + float64(dst[idx+2])*inv)
	dst[idx+3] = clampByte(sa*255.0 + float64(dst[idx+3])*inv)
}

func drawCircleBGRA(dst []byte, stride, dw, dh int, cx, cy, r float64, c color.NRGBA) {
	x0 := maxI(0, int(math.Floor(cx-r-1)))
	y0 := maxI(0, int(math.Floor(cy-r-1)))
	x1 := minI(dw-1, int(math.Ceil(cx+r+1)))
	y1 := minI(dh-1, int(math.Ceil(cy+r+1)))
	for y := y0; y <= y1; y++ {
		dy := float64(y) + 0.5 - cy
		row := y * stride
		for x := x0; x <= x1; x++ {
			dx := float64(x) + 0.5 - cx
			d := math.Hypot(dx, dy)
			if d < r+0.7 {
				cov := clampF(r+0.5-d, 0, 1)
				blendPixelBGRA(dst, row+x*4, c, cov)
			}
		}
	}
}

func drawCapsuleBGRA(dst []byte, stride, dw, dh int, ax, ay, bx, by, r float64, c color.NRGBA) {
	minX := math.Min(ax, bx) - r - 1
	maxX := math.Max(ax, bx) + r + 1
	minY := math.Min(ay, by) - r - 1
	maxY := math.Max(ay, by) + r + 1
	x0 := maxI(0, int(math.Floor(minX)))
	y0 := maxI(0, int(math.Floor(minY)))
	x1 := minI(dw-1, int(math.Ceil(maxX)))
	y1 := minI(dh-1, int(math.Ceil(maxY)))

	abx, aby := bx-ax, by-ay
	abLen2 := abx*abx + aby*aby
	for y := y0; y <= y1; y++ {
		py := float64(y) + 0.5
		row := y * stride
		for x := x0; x <= x1; x++ {
			px := float64(x) + 0.5
			var t float64
			if abLen2 > 1e-6 {
				t = clampF(((px-ax)*abx+(py-ay)*aby)/abLen2, 0, 1)
			}
			cx, cy := ax+t*abx, ay+t*aby
			d := math.Hypot(px-cx, py-cy)
			if d < r+0.7 {
				cov := clampF(r+0.5-d, 0, 1)
				blendPixelBGRA(dst, row+x*4, c, cov)
			}
		}
	}
}

func drawEllipseRingBGRA(dst []byte, stride, dw, dh int, cx, cy, rx, ry, thickness float64, c color.NRGBA) {
	pad := thickness + 2
	x0 := maxI(0, int(math.Floor(cx-rx-pad)))
	y0 := maxI(0, int(math.Floor(cy-ry-pad)))
	x1 := minI(dw-1, int(math.Ceil(cx+rx+pad)))
	y1 := minI(dh-1, int(math.Ceil(cy+ry+pad)))
	halfT := thickness * 0.5
	for y := y0; y <= y1; y++ {
		ny := (float64(y) + 0.5 - cy) / ry
		row := y * stride
		for x := x0; x <= x1; x++ {
			nx := (float64(x) + 0.5 - cx) / rx
			normD := math.Hypot(nx, ny)
			// Approximate pixel distance from ellipse perimeter
			grad := math.Hypot(nx/rx, ny/ry)
			if grad < 1e-6 {
				continue
			}
			dist := math.Abs(normD-1.0) / grad
			if dist < halfT+0.8 {
				cov := clampF(halfT+0.5-dist, 0, 1)
				blendPixelBGRA(dst, row+x*4, c, cov)
			}
		}
	}
}

func maxI(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minI(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func clampByte(v float64) byte {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return byte(math.Round(v))
}
