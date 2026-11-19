package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: mkicon <chiikawa_react.png> <out_icon.png>")
		os.Exit(1)
	}
	src, err := loadNRGBA(os.Args[1])
	if err != nil {
		panic(err)
	}

	const sz = 512
	dst := image.NewNRGBA(image.Rect(0, 0, sz, sz))

	// 1. Rounded-square pastel gradient badge (Windows 11 app icon proportions: 448x448 inside 512x512)
	const pad = 26.0
	const rad = 112.0
	const border = 14.0
	for y := 0; y < sz; y++ {
		for x := 0; x < sz; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			sd := roundedRectSDF(fx, fy, sz*0.5, sz*0.5, sz*0.5-pad, sz*0.5-pad, rad)

			// Subtle drop shadow just below the badge
			sdShadow := roundedRectSDF(fx, fy-12, sz*0.5, sz*0.5, sz*0.5-pad-4, sz*0.5-pad-4, rad)
			if sd > 0.5 && sdShadow < 22 {
				shA := clamp01(1.0-sdShadow/22.0) * 0.24
				dst.SetNRGBA(x, y, color.NRGBA{75, 48, 52, byte(shA * 255)})
			}

			if sd < 1.0 {
				edgeCov := clamp01(0.8 - sd)
				// Warm pastel sunrise gradient: soft sky-cream top -> sakura peach-pink bottom
				ty := clamp01((fy - pad) / (sz - 2*pad))
				bgR := lerp(255, 255, ty)
				bgG := lerp(244, 214, ty)
				bgB := lerp(212, 230, ty)

				// Radial warm sunlit glow in center
				distC := math.Hypot(fx-sz*0.5, fy-sz*0.52) / (sz * 0.45)
				glow := clamp01(1.0-distC) * 0.42
				bgR = lerp(bgR, 255, glow)
				bgG = lerp(bgG, 253, glow)
				bgB = lerp(bgB, 246, glow)

				// Subtle glossy highlight arc near top
				if fy < sz*0.44 {
					hDist := math.Hypot((fx-sz*0.5)/210.0, (fy-110.0)/90.0)
					if hDist < 1.0 {
						hi := (1.0 - hDist) * 0.32
						bgR = lerp(bgR, 255, hi)
						bgG = lerp(bgG, 255, hi)
						bgB = lerp(bgB, 255, hi)
					}
				}

				col := color.NRGBA{clampU8(bgR), clampU8(bgG), clampU8(bgB), 255}

				// Inner crisp white rim
				if sd > -border-9 && sd <= -border {
					rimT := clamp01((sd - (-border - 9)) / 3.0)
					col.R = clampU8(lerp(float64(col.R), 255, rimT*0.85))
					col.G = clampU8(lerp(float64(col.G), 255, rimT*0.85))
					col.B = clampU8(lerp(float64(col.B), 255, rimT*0.85))
				}
				// Cute thick warm dark-brown sticker outline
				if sd > -border {
					bt := clamp01((sd + border) / 2.5)
					col.R = clampU8(lerp(float64(col.R), 78, bt))
					col.G = clampU8(lerp(float64(col.G), 50, bt))
					col.B = clampU8(lerp(float64(col.B), 42, bt))
				}

				col.A = clampU8(255.0 * edgeCov)
				blendOver(dst, x, y, col)
			}
		}
	}

	// 2. Cute decorative golden sparkles in corners
	drawStar(dst, 118, 128, 28, color.NRGBA{255, 198, 60, 255})
	drawStar(dst, 404, 136, 24, color.NRGBA{255, 185, 65, 255})
	drawStar(dst, 106, 378, 18, color.NRGBA{255, 150, 185, 240})
	drawStar(dst, 412, 370, 20, color.NRGBA{255, 198, 60, 245})

	// 3. Cute mini golden propeller at top center of the badge (signature motif!)
	drawMiniPropeller(dst, sz*0.5, 108)

	// 4. Blit Chiikawa (hugging pink heart) big, crisp, and centered inside the badge!
	blitScaled(dst, src, sz*0.5, 314, 342)

	// 5. Supersample downscale 512 -> 256 for ultra-clean anti-aliasing
	out := downscale2x(dst)
	f, err := os.Create(os.Args[2])
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, out); err != nil {
		panic(err)
	}
	fmt.Printf("wrote %s (256x256)\n", os.Args[2])
}

func drawMiniPropeller(dst *image.NRGBA, cx, cy float64) {
	outline := color.NRGBA{78, 50, 42, 255}
	goldLight := color.NRGBA{255, 228, 118, 255}
	goldMid := color.NRGBA{248, 185, 60, 255}
	shaft := color.NRGBA{205, 214, 226, 255}

	// Shaft down toward head
	drawCapsule(dst, cx, cy, cx, cy+42, 9.5, outline)
	drawCapsule(dst, cx, cy, cx, cy+42, 5.2, shaft)
	// Propeller blades
	drawCapsule(dst, cx-76, cy, cx+76, cy, 16, outline)
	drawCapsule(dst, cx-76, cy, cx+76, cy, 10.5, goldLight)
	drawCapsule(dst, cx-64, cy+3, cx+64, cy+3, 5.5, goldMid)
	// Center knob
	drawCircle(dst, cx, cy-8, 17, outline)
	drawCircle(dst, cx, cy-8, 11.5, goldMid)
	drawCircle(dst, cx-3.5, cy-11, 4.5, goldLight)
}

func drawStar(dst *image.NRGBA, cx, cy, r float64, c color.NRGBA) {
	outline := color.NRGBA{78, 50, 42, 160}
	drawCapsule(dst, cx-r, cy, cx+r, cy, r*0.26, outline)
	drawCapsule(dst, cx, cy-r, cx, cy+r, r*0.26, outline)
	drawCapsule(dst, cx-r*0.92, cy, cx+r*0.92, cy, r*0.18, c)
	drawCapsule(dst, cx, cy-r*0.92, cx, cy+r*0.92, r*0.18, c)
	drawCircle(dst, cx, cy, r*0.34, color.NRGBA{255, 252, 228, 255})
}

func blitScaled(dst, src *image.NRGBA, cx, cy, targetSize float64) {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	scale := targetSize / float64(sw)
	x0 := int(math.Floor(cx - targetSize*0.5))
	y0 := int(math.Floor(cy - float64(sh)*scale*0.5))
	w := int(math.Ceil(targetSize))
	h := int(math.Ceil(float64(sh) * scale))

	for dy := 0; dy < h; dy++ {
		ty := y0 + dy
		if ty < 0 || ty >= dst.Bounds().Dy() {
			continue
		}
		v := (float64(dy)+0.5)/scale - 0.5
		for dx := 0; dx < w; dx++ {
			tx := x0 + dx
			if tx < 0 || tx >= dst.Bounds().Dx() {
				continue
			}
			u := (float64(dx)+0.5)/scale - 0.5
			p := sampleBilinear(src, u, v)
			if p.A > 0 {
				blendOver(dst, tx, ty, p)
			}
		}
	}
}

func sampleBilinear(src *image.NRGBA, u, v float64) color.NRGBA {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	ix, iy := int(math.Floor(u)), int(math.Floor(v))
	fx, fy := u-float64(ix), v-float64(iy)
	var pr, pg, pb, pa float64
	for oy := 0; oy <= 1; oy++ {
		sy := iy + oy
		if sy < 0 || sy >= sh {
			continue
		}
		wy := 1 - fy
		if oy == 1 {
			wy = fy
		}
		for ox := 0; ox <= 1; ox++ {
			sx := ix + ox
			if sx < 0 || sx >= sw {
				continue
			}
			wx := 1 - fx
			if ox == 1 {
				wx = fx
			}
			wt := wx * wy
			p := src.NRGBAAt(sx, sy)
			af := float64(p.A) / 255.0
			pr += float64(p.R) * af * wt
			pg += float64(p.G) * af * wt
			pb += float64(p.B) * af * wt
			pa += float64(p.A) * wt
		}
	}
	if pa < 0.5 {
		return color.NRGBA{}
	}
	af := pa / 255.0
	return color.NRGBA{clampU8(pr / af), clampU8(pg / af), clampU8(pb / af), clampU8(pa)}
}

func downscale2x(src *image.NRGBA) *image.NRGBA {
	w, h := src.Bounds().Dx()/2, src.Bounds().Dy()/2
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var pr, pg, pb, pa float64
			for oy := 0; oy < 2; oy++ {
				for ox := 0; ox < 2; ox++ {
					p := src.NRGBAAt(x*2+ox, y*2+oy)
					af := float64(p.A) / 255.0
					pr += float64(p.R) * af
					pg += float64(p.G) * af
					pb += float64(p.B) * af
					pa += float64(p.A)
				}
			}
			pa /= 4
			if pa > 0.5 {
				af := pa / 255.0
				dst.SetNRGBA(x, y, color.NRGBA{clampU8(pr / 4 / af), clampU8(pg / 4 / af), clampU8(pb / 4 / af), clampU8(pa)})
			}
		}
	}
	return dst
}

func roundedRectSDF(px, py, cx, cy, halfW, halfH, r float64) float64 {
	qx := math.Abs(px-cx) - (halfW - r)
	qy := math.Abs(py-cy) - (halfH - r)
	return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - r
}

func drawCircle(dst *image.NRGBA, cx, cy, r float64, c color.NRGBA) {
	b := dst.Bounds()
	for y := maxI(0, int(cy-r-2)); y <= minI(b.Dy()-1, int(cy+r+2)); y++ {
		for x := maxI(0, int(cx-r-2)); x <= minI(b.Dx()-1, int(cx+r+2)); x++ {
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			if d < r+1 {
				cov := clamp01(r + 0.5 - d)
				cc := c
				cc.A = clampU8(float64(c.A) * cov)
				blendOver(dst, x, y, cc)
			}
		}
	}
}

func drawCapsule(dst *image.NRGBA, ax, ay, bx, by, r float64, c color.NRGBA) {
	b := dst.Bounds()
	x0 := maxI(0, int(math.Min(ax, bx)-r-2))
	x1 := minI(b.Dx()-1, int(math.Max(ax, bx)+r+2))
	y0 := maxI(0, int(math.Min(ay, by)-r-2))
	y1 := minI(b.Dy()-1, int(math.Max(ay, by)+r+2))
	abx, aby := bx-ax, by-ay
	ab2 := abx*abx + aby*aby
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			t := 0.0
			if ab2 > 1e-6 {
				t = clamp01(((px-ax)*abx + (py-ay)*aby) / ab2)
			}
			d := math.Hypot(px-(ax+t*abx), py-(ay+t*aby))
			if d < r+1 {
				cov := clamp01(r + 0.5 - d)
				cc := c
				cc.A = clampU8(float64(c.A) * cov)
				blendOver(dst, x, y, cc)
			}
		}
	}
}

func blendOver(dst *image.NRGBA, x, y int, s color.NRGBA) {
	if s.A == 0 {
		return
	}
	d := dst.NRGBAAt(x, y)
	sa := float64(s.A) / 255.0
	da := float64(d.A) / 255.0
	outA := sa + da*(1-sa)
	if outA < 1e-5 {
		return
	}
	r := (float64(s.R)*sa + float64(d.R)*da*(1-sa)) / outA
	g := (float64(s.G)*sa + float64(d.G)*da*(1-sa)) / outA
	b := (float64(s.B)*sa + float64(d.B)*da*(1-sa)) / outA
	dst.SetNRGBA(x, y, color.NRGBA{clampU8(r), clampU8(g), clampU8(b), clampU8(outA * 255)})
}

func loadNRGBA(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			dst.Set(x, y, color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)))
		}
	}
	return dst, nil
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
func clampU8(v float64) byte {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return byte(math.Round(v))
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
