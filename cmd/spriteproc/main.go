package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const outSize = 160

func main() {
	inFile := flag.String("in", "", "input sprite sheet (.jpg or .png)")
	outDir := flag.String("out", "internal/pet/sprites", "output directory")
	prefix := flag.String("prefix", "", "character id prefix (e.g. tanjiro)")
	posesFlag := flag.String("poses", "idle,grabbed,react", "comma-separated pose names left-to-right")
	bgMode := flag.String("bg", "green", "chroma background color: green or magenta")
	cleanTopArm := flag.Bool("clean-top-arm", false, "erase disembodied beige arm touching top center edge")
	cleanTopString := flag.Bool("clean-top-string", false, "erase thin vertical string above pose 2 head/ears")
	cleanVertLine := flag.Bool("clean-vert-line", false, "erase thin dark vertical wall line in pose 3")
	noInteriorGreen := flag.Bool("no-interior-green", false, "skip interior green hole punch (preserves green costume details)")
	copyPose := flag.String("copy-pose", "", "optional srcIdx:dstIdx (0-based) to replace a bad pose with another")
	preview := flag.String("preview", "", "optional preview contact sheet path")
	flag.Parse()

	if *inFile == "" || *prefix == "" {
		fmt.Println("usage: spriteproc -in <sheet.jpg> -out <dir> -prefix <char> -poses <p1,p2,...> [-bg green|magenta] [-preview out.png]")
		os.Exit(1)
	}

	poses := strings.Split(*posesFlag, ",")
	if len(poses) < 1 {
		panic("at least 1 pose required")
	}

	img, err := loadImage(*inFile)
	if err != nil {
		panic(err)
	}
	if *cleanTopArm {
		eraseTopCenterArmMagenta(img)
	}
	var clean *image.NRGBA
	if *bgMode == "magenta" {
		clean = removeChromaMagenta(img)
	} else {
		clean = removeChromaGreen(img, !*noInteriorGreen)
	}
	if *cleanVertLine {
		eraseThinVerticalWallLine(clean)
	}

	crops := splitPoses(clean, len(poses))
	if *cleanTopString && len(crops) >= 2 {
		c := crops[1]
		cw, ch := c.Bounds().Dx(), c.Bounds().Dy()
		// Erase thin vertical hanging string pixels descending from the very top above ears
		for y := 0; y < int(float64(ch)*0.22); y++ {
			maxRun := 0
			run := 0
			for x := 0; x < cw; x++ {
				if c.NRGBAAt(x, y).A >= 20 {
					run++
					if run > maxRun {
						maxRun = run
					}
				} else {
					run = 0
				}
			}
			if maxRun == 0 {
				continue
			}
			if maxRun > 9 {
				break
			}
			for x := 0; x < cw; x++ {
				c.SetNRGBA(x, y, color.NRGBA{0, 0, 0, 0})
			}
		}
		crops[1] = cropTight(c, c.Bounds())
	}
	if *cleanTopArm && len(crops) >= 2 {
		c := crops[1]
		cw, ch := c.Bounds().Dx(), c.Bounds().Dy()
		// Erase the two tiny leftover arm-outline whiskers strictly above Tanjiro's hair crown
		for y := 0; y < int(float64(ch)*0.115); y++ {
			for x := int(float64(cw) * 0.48); x < int(float64(cw)*0.70); x++ {
				if y < int(float64(ch)*0.075) || x > int(float64(cw)*0.60) {
					c.SetNRGBA(x, y, color.NRGBA{0, 0, 0, 0})
				}
			}
		}
		crops[1] = cropTight(c, c.Bounds())
	}
	if *copyPose != "" {
		var si, di int
		if _, err := fmt.Sscanf(*copyPose, "%d:%d", &si, &di); err == nil && si >= 0 && si < len(crops) && di >= 0 && di < len(crops) {
			crops[di] = crops[si]
		}
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		panic(err)
	}

	var sheet *image.RGBA
	if *preview != "" {
		sheet = image.NewRGBA(image.Rect(0, 0, outSize*len(poses)+32, outSize+32))
		drawCheckerboard(sheet)
	}

	for i, pose := range poses {
		pose = strings.TrimSpace(pose)
		centerV := pose == "grabbed" || strings.HasPrefix(pose, "climb")
		rightAlign := strings.HasPrefix(pose, "climb")
		norm := normalizeSprite(crops[i], outSize, centerV, rightAlign)
		dstPath := filepath.Join(*outDir, fmt.Sprintf("%s_%s.png", *prefix, pose))
		if err := savePNG(dstPath, norm); err != nil {
			panic(err)
		}
		if sheet != nil {
			compositeOver(sheet, norm, 16+i*outSize, 16)
		}
		fmt.Printf("wrote %s\n", dstPath)
	}

	if sheet != nil && *preview != "" {
		if err := savePNG(*preview, sheet); err != nil {
			panic(err)
		}
		fmt.Printf("wrote preview %s\n", *preview)
	}
}

func eraseTopCenterArmMagenta(img *image.NRGBA) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	// Erase the small beige arm stub descending from top center (x=0.45..0.58, y=0..0.22)
	// stopping cleanly right above Tanjiro's dark-burgundy hair!
	minX := int(float64(w) * 0.45)
	maxX := int(float64(w) * 0.58)
	maxY := int(float64(h) * 0.22)

	isMagenta := func(p color.NRGBA) bool {
		return minU8(p.R, p.B) > 140 && int(minU8(p.R, p.B))-int(p.G) > 50
	}
	isHair := func(p color.NRGBA) bool {
		// Real burgundy hair interior (R >> B, unlike magenta-blended dark outline where R ≈ B!)
		return p.R > 74 && p.R < 160 && p.G < 75 && p.B < 90 && int(p.R)-int(p.G) > 32 && int(p.R)-int(p.B) > 22
	}
	isSkinOrOutline := func(p color.NRGBA) bool {
		if isMagenta(p) || isHair(p) {
			return false
		}
		isBeige := p.R > 180 && p.G > 145 && p.B > 110
		isDark := p.R < 95 && p.G < 85 && p.B < 85
		return isBeige || isDark
	}

	visited := make([]bool, w*h)
	var q []int
	// Seed from y=0..20% where beige skin is found at top center
	for y := 0; y < int(float64(h)*0.16); y++ {
		for x := minX; x <= maxX; x++ {
			p := img.NRGBAAt(x, y)
			if p.R > 195 && p.G > 155 && p.B > 120 && !isMagenta(p) {
				idx := y*w + x
				if !visited[idx] {
					visited[idx] = true
					q = append(q, idx)
				}
			}
		}
	}
	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for head := 0; head < len(q); head++ {
		cur := q[head]
		cx, cy := cur%w, cur/w
		for _, d := range dirs {
			nx, ny := cx+d[0], cy+d[1]
			if nx < minX || nx > maxX || ny < 0 || ny > maxY {
				continue
			}
			nidx := ny*w + nx
			if visited[nidx] {
				continue
			}
			p := img.NRGBAAt(nx, ny)
			if isSkinOrOutline(p) {
				// Preserve dark hair outline directly bordering burgundy hair
				if p.R < 95 && p.G < 85 && p.B < 85 {
					touchesHair := false
					for oy := -3; oy <= 3 && !touchesHair; oy++ {
						for ox := -3; ox <= 3 && !touchesHair; ox++ {
							tx, ty := nx+ox, ny+oy
							if tx >= 0 && tx < w && ty >= 0 && ty < h && isHair(img.NRGBAAt(tx, ty)) {
								touchesHair = true
							}
						}
					}
					if touchesHair {
						continue
					}
				}
				visited[nidx] = true
				q = append(q, nidx)
			}
		}
	}
	for _, idx := range q {
		img.SetNRGBA(idx%w, idx/w, color.NRGBA{255, 0, 255, 255})
	}
	// Scan each column in the top arm zone from y=0 down to just above the first burgundy hair pixel!
	topCut := int(float64(h) * 0.24)
	for x := minX; x <= maxX; x++ {
		hairY := -1
		for y := 0; y < topCut; y++ {
			if isHair(img.NRGBAAt(x, y)) {
				hairY = y
				break
			}
		}
		eraseUntil := topCut
		if hairY >= 0 {
			eraseUntil = hairY - 5 // preserve crisp dark outline right above burgundy hair!
		}
		for y := 0; y < eraseUntil; y++ {
			img.SetNRGBA(x, y, color.NRGBA{255, 0, 255, 255})
		}
	}
}

func eraseThinVerticalWallLine(img *image.NRGBA) {
	// Erase thin (<= 6px wide) dark vertical line segments in the 3rd quarter (x=0.68..0.75)
	// that are surrounded on left and right by transparent background!
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	xStart := int(float64(w) * 0.68)
	xEnd := int(float64(w) * 0.76)
	for y := 0; y < int(float64(h)*0.46); y++ {
		x := xStart
		for x < xEnd {
			if img.NRGBAAt(x, y).A < 20 {
				x++
				continue
			}
			runStart := x
			for x < xEnd && img.NRGBAAt(x, y).A >= 20 {
				x++
			}
			runLen := x - runStart
			if runLen <= 7 && runStart > xStart && x < xEnd {
				for rx := runStart; rx < x; rx++ {
					img.SetNRGBA(rx, y, color.NRGBA{0, 0, 0, 0})
				}
			}
		}
	}
}

func loadImage(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			dst.Set(x, y, color.NRGBAModel.Convert(src.At(b.Min.X+x, b.Min.Y+y)))
		}
	}
	return dst, nil
}

func savePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	return enc.Encode(f, img)
}

func removeChromaGreen(src *image.NRGBA, punchInterior bool) *image.NRGBA {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	copy(out.Pix, src.Pix)

	bgR, bgG, bgB := avgCorners(src)
	bgMask := make([]bool, w*h)

	isBG := func(r, g, b uint8) bool {
		fr, fg, fb := float64(r), float64(g), float64(b)
		excess := fg - math.Max(fr, fb)
		dist := math.Sqrt((fr-bgR)*(fr-bgR) + (fg-bgG)*(fg-bgG) + (fb-bgB)*(fb-bgB))
		return (dist < 65 && excess > 30) || (fg > 85 && excess > 35 && fr < 110 && fb < 110)
	}

	floodBorder(out, bgMask, isBG)

	if punchInterior {
		// Interior pure chroma-green holes
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				idx := y*w + x
				if bgMask[idx] {
					continue
				}
				p := out.NRGBAAt(x, y)
				if p.G > 165 && int(p.G)-int(maxU8(p.R, p.B)) > 75 && p.R < 120 && p.B < 120 {
					bgMask[idx] = true
				}
			}
		}
	}
	applyMask(out, bgMask)

	// Soft 2-pass green despill
	for pass := 0; pass < 2; pass++ {
		despillEdge(out, func(p color.NRGBA) (color.NRGBA, bool) {
			maxRB := maxU8(p.R, p.B)
			excess := int(p.G) - int(maxRB)
			if excess <= 12 {
				return p, false
			}
			alphaScale := math.Max(0, 1.0-float64(excess)/140.0)
			newA := uint8(float64(p.A) * alphaScale)
			if newA < 24 {
				newA = 0
			}
			return color.NRGBA{p.R, maxRB, p.B, newA}, true
		})
	}
	return out
}

func removeChromaMagenta(src *image.NRGBA) *image.NRGBA {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	copy(out.Pix, src.Pix)

	bgR, bgG, bgB := avgCorners(src)
	bgMask := make([]bool, w*h)

	isBG := func(r, g, b uint8) bool {
		fr, fg, fb := float64(r), float64(g), float64(b)
		minRB := math.Min(fr, fb)
		excess := minRB - fg
		dist := math.Sqrt((fr-bgR)*(fr-bgR) + (fg-bgG)*(fg-bgG) + (fb-bgB)*(fb-bgB))
		return (dist < 70 && excess > 35) || (minRB > 140 && excess > 55 && fg < 130)
	}

	floodBorder(out, bgMask, isBG)

	// Interior pure chroma-magenta holes
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := y*w + x
			if bgMask[idx] {
				continue
			}
			p := out.NRGBAAt(x, y)
			minRB := minU8(p.R, p.B)
			if minRB > 165 && int(minRB)-int(p.G) > 85 && p.G < 110 {
				bgMask[idx] = true
			}
		}
	}
	applyMask(out, bgMask)

	// Soft 2-pass magenta despill
	for pass := 0; pass < 2; pass++ {
		despillEdge(out, func(p color.NRGBA) (color.NRGBA, bool) {
			minRB := minU8(p.R, p.B)
			excess := int(minRB) - int(p.G)
			if excess <= 18 {
				return p, false
			}
			alphaScale := math.Max(0, 1.0-float64(excess)/145.0)
			newA := uint8(float64(p.A) * alphaScale)
			if newA < 24 {
				newA = 0
			}
			nr, nb := p.R, p.B
			if nr > p.G {
				nr = p.G
			}
			if nb > p.G {
				nb = p.G
			}
			return color.NRGBA{nr, p.G, nb, newA}, true
		})
	}
	return out
}

func avgCorners(src *image.NRGBA) (float64, float64, float64) {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	corners := [][2]int{{2, 2}, {w - 3, 2}, {2, h - 3}, {w - 3, h - 3}}
	var r, g, b float64
	for _, c := range corners {
		p := src.NRGBAAt(c[0], c[1])
		r += float64(p.R)
		g += float64(p.G)
		b += float64(p.B)
	}
	return r / 4, g / 4, b / 4
}

func floodBorder(img *image.NRGBA, mask []bool, isBG func(r, g, b uint8) bool) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	queue := make([]int, 0, w*2+h*2)
	push := func(x, y int) {
		idx := y*w + x
		if mask[idx] {
			return
		}
		p := img.NRGBAAt(x, y)
		if isBG(p.R, p.G, p.B) {
			mask[idx] = true
			queue = append(queue, idx)
		}
	}
	for x := 0; x < w; x++ {
		push(x, 0)
		push(x, h-1)
	}
	for y := 0; y < h; y++ {
		push(0, y)
		push(w-1, y)
	}
	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for head := 0; head < len(queue); head++ {
		idx := queue[head]
		x, y := idx%w, idx/w
		for _, d := range dirs {
			nx, ny := x+d[0], y+d[1]
			if nx >= 0 && nx < w && ny >= 0 && ny < h {
				push(nx, ny)
			}
		}
	}
}

func applyMask(img *image.NRGBA, mask []bool) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if mask[y*w+x] {
				img.SetNRGBA(x, y, color.NRGBA{0, 0, 0, 0})
			}
		}
	}
}

func despillEdge(out *image.NRGBA, fn func(color.NRGBA) (color.NRGBA, bool)) {
	w, h := out.Bounds().Dx(), out.Bounds().Dy()
	type adj struct {
		x, y int
		c    color.NRGBA
	}
	var list []adj
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			p := out.NRGBAAt(x, y)
			if p.A == 0 {
				continue
			}
			nearTrans := false
			for dy := -1; dy <= 1 && !nearTrans; dy++ {
				for dx := -1; dx <= 1 && !nearTrans; dx++ {
					if out.NRGBAAt(x+dx, y+dy).A < 128 {
						nearTrans = true
					}
				}
			}
			if !nearTrans {
				continue
			}
			if nc, ok := fn(p); ok {
				list = append(list, adj{x, y, nc})
			}
		}
	}
	for _, it := range list {
		out.SetNRGBA(it.x, it.y, it.c)
	}
}

type component struct {
	pixels []int
	sumX   float64
}

func splitPoses(img *image.NRGBA, count int) []*image.NRGBA {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	visited := make([]bool, w*h)
	var comps []component
	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {-1, 1}, {1, -1}, {-1, -1}}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := y*w + x
			if visited[idx] || img.NRGBAAt(x, y).A < 20 {
				continue
			}
			visited[idx] = true
			q := []int{idx}
			c := component{}
			for head := 0; head < len(q); head++ {
				cur := q[head]
				cx, cy := cur%w, cur/w
				c.pixels = append(c.pixels, cur)
				c.sumX += float64(cx)
				for _, d := range dirs {
					nx, ny := cx+d[0], cy+d[1]
					if nx >= 0 && nx < w && ny >= 0 && ny < h {
						nidx := ny*w + nx
						if !visited[nidx] && img.NRGBAAt(nx, ny).A >= 20 {
							visited[nidx] = true
							q = append(q, nidx)
						}
					}
				}
			}
			if len(c.pixels) >= 12 {
				comps = append(comps, c)
			}
		}
	}

	if len(comps) < count {
		res := make([]*image.NRGBA, count)
		tw := w / count
		for i := 0; i < count; i++ {
			x0 := i * tw
			x1 := (i + 1) * tw
			if i == count-1 {
				x1 = w
			}
			res[i] = cropTight(img, image.Rect(x0, 0, x1, h))
		}
		return res
	}

	sort.Slice(comps, func(i, j int) bool { return len(comps[i].pixels) > len(comps[j].pixels) })
	anchors := make([]component, count)
	copy(anchors, comps[:count])
	sort.Slice(anchors, func(i, j int) bool {
		return (anchors[i].sumX / float64(len(anchors[i].pixels))) < (anchors[j].sumX / float64(len(anchors[j].pixels)))
	})

	anchorCX := make([]float64, count)
	buckets := make([]*image.NRGBA, count)
	for k := 0; k < count; k++ {
		anchorCX[k] = anchors[k].sumX / float64(len(anchors[k].pixels))
		buckets[k] = image.NewNRGBA(image.Rect(0, 0, w, h))
	}

	for _, c := range comps {
		cx := c.sumX / float64(len(c.pixels))
		best := 0
		bestDist := math.Abs(cx - anchorCX[0])
		for k := 1; k < count; k++ {
			if d := math.Abs(cx - anchorCX[k]); d < bestDist {
				bestDist = d
				best = k
			}
		}
		for _, idx := range c.pixels {
			x, y := idx%w, idx/w
			buckets[best].SetNRGBA(x, y, img.NRGBAAt(x, y))
		}
	}

	res := make([]*image.NRGBA, count)
	for k := 0; k < count; k++ {
		res[k] = cropTight(buckets[k], buckets[k].Bounds())
	}
	return res
}

func cropTight(img *image.NRGBA, region image.Rectangle) *image.NRGBA {
	minX, minY := region.Max.X, region.Max.Y
	maxX, maxY := region.Min.X, region.Min.Y
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			if img.NRGBAAt(x, y).A > 15 {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if maxX < minX || maxY < minY {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	pad := 4
	minX = maxInt(region.Min.X, minX-pad)
	minY = maxInt(region.Min.Y, minY-pad)
	maxX = minInt(region.Max.X-1, maxX+pad)
	maxY = minInt(region.Max.Y-1, maxY+pad)
	cw, ch := maxX-minX+1, maxY-minY+1
	dst := image.NewNRGBA(image.Rect(0, 0, cw, ch))
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			dst.SetNRGBA(x, y, img.NRGBAAt(minX+x, minY+y))
		}
	}
	return dst
}

// normalizeSprite scales into size x size. For climb poses (rightAlign=true),
// aligns the rightmost opaque pixels near x = size*0.76 so hands/feet touch the exact window wall!
func normalizeSprite(src *image.NRGBA, size int, centerV, rightAlign bool) *image.NRGBA {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	maxDim := float64(size - 12)
	scale := math.Min(maxDim/float64(sw), maxDim/float64(sh))
	dw := maxInt(1, int(math.Round(float64(sw)*scale)))
	dh := maxInt(1, int(math.Round(float64(sh)*scale)))

	resized := resizeArea(src, dw, dh)
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))

	ox := (size - dw) / 2
	if rightAlign {
		// Place right edge of climbing sprite right at x = 0.76 * size (matching world.go wall offset 0.255!)
		targetRight := int(math.Round(float64(size) * 0.76))
		ox = targetRight - dw
		if ox < 2 {
			ox = (size - dw) / 2
		}
	}
	oy := size - dh - 4
	if centerV {
		oy = (size - dh) / 2
	}
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			tx, ty := ox+x, oy+y
			if tx >= 0 && tx < size && ty >= 0 && ty < size {
				dst.SetNRGBA(tx, ty, resized.NRGBAAt(x, y))
			}
		}
	}
	return dst
}

func resizeArea(src *image.NRGBA, dw, dh int) *image.NRGBA {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	scaleX := float64(sw) / float64(dw)
	scaleY := float64(sh) / float64(dh)
	for dy := 0; dy < dh; dy++ {
		sy0, sy1 := float64(dy)*scaleY, float64(dy+1)*scaleY
		iy0, iy1 := int(math.Floor(sy0)), minInt(sh, int(math.Ceil(sy1)))
		for dx := 0; dx < dw; dx++ {
			sx0, sx1 := float64(dx)*scaleX, float64(dx+1)*scaleX
			ix0, ix1 := int(math.Floor(sx0)), minInt(sw, int(math.Ceil(sx1)))
			var rS, gS, bS, aS, wS float64
			for sy := iy0; sy < iy1; sy++ {
				wy := math.Min(float64(sy+1), sy1) - math.Max(float64(sy), sy0)
				if wy <= 0 {
					continue
				}
				for sx := ix0; sx < ix1; sx++ {
					wx := math.Min(float64(sx+1), sx1) - math.Max(float64(sx), sx0)
					if wx <= 0 {
						continue
					}
					wt := wx * wy
					p := src.NRGBAAt(sx, sy)
					af := float64(p.A) / 255.0
					rS += float64(p.R) * af * wt
					gS += float64(p.G) * af * wt
					bS += float64(p.B) * af * wt
					aS += float64(p.A) * wt
					wS += wt
				}
			}
			if wS > 0 && aS > 0 {
				aOut := aS / wS
				afOut := aOut / 255.0
				dst.SetNRGBA(dx, dy, color.NRGBA{
					clampU8(rS / wS / afOut),
					clampU8(gS / wS / afOut),
					clampU8(bS / wS / afOut),
					clampU8(aOut),
				})
			}
		}
	}
	return dst
}

func drawCheckerboard(img *image.RGBA) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.RGBA{225, 228, 232, 255}
			if ((x/12)+(y/12))%2 == 0 {
				c = color.RGBA{245, 247, 250, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
}

func compositeOver(dst *image.RGBA, src *image.NRGBA, ox, oy int) {
	sb := src.Bounds()
	for y := 0; y < sb.Dy(); y++ {
		for x := 0; x < sb.Dx(); x++ {
			s := src.NRGBAAt(x, y)
			if s.A == 0 {
				continue
			}
			tx, ty := ox+x, oy+y
			if !image.Pt(tx, ty).In(dst.Bounds()) {
				continue
			}
			d := dst.RGBAAt(tx, ty)
			af := float64(s.A) / 255.0
			inv := 1.0 - af
			dst.SetRGBA(tx, ty, color.RGBA{
				clampU8(float64(s.R)*af + float64(d.R)*inv),
				clampU8(float64(s.G)*af + float64(d.G)*inv),
				clampU8(float64(s.B)*af + float64(d.B)*inv),
				255,
			})
		}
	}
}

func maxU8(a, b uint8) uint8 {
	if a > b {
		return a
	}
	return b
}
func minU8(a, b uint8) uint8 {
	if a < b {
		return a
	}
	return b
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func clampU8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(math.Round(v))
}
