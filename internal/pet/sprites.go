package pet

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	"image/color"
	"image/png"
)

//go:embed sprites/*.png
var spriteFS embed.FS

type Pose int

const (
	PoseIdle Pose = iota
	PoseGrabbed
	PoseReact
	PoseWalk1
	PoseWalk2
	PoseClimb1
	PoseClimb2
	poseCount
)

type ParticleKind int

const (
	ParticleSparkle ParticleKind = iota
	ParticleHeart
	ParticleFlame
	ParticleLightning
	ParticleBubble
	ParticleSnowflake
	ParticleSweat
	ParticleDust
	ParticleWater
)

type CharacterDef struct {
	ID           string
	Name         string
	Group        string
	ReactScale   float64
	HeadTopFrac  float64 // 0..1 from top of 160x160 sprite where another pet's feet rest
	ParticleKind ParticleKind
	AccentColor  color.NRGBA
}

var Catalog = []CharacterDef{
	{
		ID: "manggom", Name: "망곰", Group: "망곰",
		ReactScale: 1.05, HeadTopFrac: 0.14,
		ParticleKind: ParticleHeart, AccentColor: color.NRGBA{255, 130, 165, 255},
	},
	{
		ID: "tanjiro", Name: "카마도 탄지로", Group: "귀멸의 칼날",
		ReactScale: 1.08, HeadTopFrac: 0.14,
		ParticleKind: ParticleWater, AccentColor: color.NRGBA{75, 200, 255, 255},
	},
	{
		ID: "kyojuro", Name: "렌고쿠 쿄쥬로", Group: "귀멸의 칼날",
		ReactScale: 1.06, HeadTopFrac: 0.15,
		ParticleKind: ParticleFlame, AccentColor: color.NRGBA{255, 115, 35, 255},
	},
	{
		ID: "zenitsu", Name: "아가츠마 젠이츠", Group: "귀멸의 칼날",
		ReactScale: 1.06, HeadTopFrac: 0.14,
		ParticleKind: ParticleLightning, AccentColor: color.NRGBA{255, 220, 45, 255},
	},
	{
		ID: "inosuke", Name: "하시비라 이노스케", Group: "귀멸의 칼날",
		ReactScale: 1.06, HeadTopFrac: 0.18,
		ParticleKind: ParticleBubble, AccentColor: color.NRGBA{255, 185, 215, 255},
	},
	{
		ID: "akaza", Name: "아카자", Group: "귀멸의 칼날",
		ReactScale: 1.08, HeadTopFrac: 0.15,
		ParticleKind: ParticleSnowflake, AccentColor: color.NRGBA{110, 220, 255, 255},
	},
}

// PremulSprite stores a sprite pre-converted to premultiplied BGRA (matching Win32 DIB layout)
// so 40 FPS software blitting avoids per-pixel alpha premultiplication.
type PremulSprite struct {
	W, H   int
	Stride int
	Pix    []byte
}

type SpriteSet struct {
	Def    CharacterDef
	Poses  [poseCount]*image.NRGBA
	Premul [poseCount]*PremulSprite
}

type SpriteBank struct {
	ByID map[string]*SpriteSet
	List []*SpriteSet
}

func LoadSpriteBank() (*SpriteBank, error) {
	bank := &SpriteBank{
		ByID: make(map[string]*SpriteSet, len(Catalog)),
		List: make([]*SpriteSet, 0, len(Catalog)),
	}
	poseNames := [poseCount]string{"idle", "grabbed", "react", "walk1", "walk2", "climb1", "climb2"}
	for _, def := range Catalog {
		ss := &SpriteSet{Def: def}
		for i, pn := range poseNames {
			path := fmt.Sprintf("sprites/%s_%s.png", def.ID, pn)
			raw, err := spriteFS.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("read embedded %s: %w", path, err)
			}
			img, err := png.Decode(bytes.NewReader(raw))
			if err != nil {
				return nil, fmt.Errorf("decode %s: %w", path, err)
			}
			nrgba := toNRGBA(img)
			ss.Poses[i] = nrgba
			ss.Premul[i] = toPremulBGRA(nrgba)
		}
		bank.ByID[def.ID] = ss
		bank.List = append(bank.List, ss)
	}
	return bank, nil
}

func toPremulBGRA(src *image.NRGBA) *PremulSprite {
	w := src.Bounds().Dx()
	h := src.Bounds().Dy()
	stride := w * 4
	pix := make([]byte, stride*h)
	for y := 0; y < h; y++ {
		sRow := y * src.Stride
		dRow := y * stride
		for x := 0; x < w; x++ {
			si := sRow + x*4
			di := dRow + x*4
			a := uint32(src.Pix[si+3])
			if a == 0 {
				continue
			}
			pix[di+0] = byte((uint32(src.Pix[si+2])*a + 127) / 255) // B
			pix[di+1] = byte((uint32(src.Pix[si+1])*a + 127) / 255) // G
			pix[di+2] = byte((uint32(src.Pix[si+0])*a + 127) / 255) // R
			pix[di+3] = byte(a)                                     // A
		}
	}
	return &PremulSprite{W: w, H: h, Stride: stride, Pix: pix}
}

func toNRGBA(src image.Image) *image.NRGBA {
	if n, ok := src.(*image.NRGBA); ok {
		return n
	}
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			dst.Set(x, y, color.NRGBAModel.Convert(src.At(b.Min.X+x, b.Min.Y+y)))
		}
	}
	return dst
}
