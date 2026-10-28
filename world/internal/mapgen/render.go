package mapgen

import (
	"image"
	"image/color"
	"math"
)

// The look of the relief, as on the physical map of a good atlas.
const (
	// reliefExaggeration raises the relief for the hillshade: at a few
	// kilometres a pixel, real slopes would barely shade.
	reliefExaggeration = 22.0
	supersample        = 2
)

type rgb struct{ r, g, b float64 }

func hex(v uint32) rgb {
	return rgb{float64(v>>16&255) / 255, float64(v>>8&255) / 255, float64(v&255) / 255}
}

func (c rgb) mix(o rgb, t float64) rgb {
	return rgb{c.r + (o.r-c.r)*t, c.g + (o.g-c.g)*t, c.b + (o.b-c.b)*t}
}

func (c rgb) mul(k rgb) rgb { return rgb{c.r * k.r, c.g * k.g, c.b * k.b} }

func (c rgb) scale(k float64) rgb { return rgb{c.r * k, c.g * k, c.b * k} }

// The hypsometric tints, from the lowlands to the snow.
var hypsometric = []struct {
	metres float64
	c      rgb
}{
	{0, hex(0x7d9a6a)},
	{250, hex(0x93a874)},
	{550, hex(0xa7b67e)},
	{900, hex(0xb3ad7c)},
	{1300, hex(0xa8946e)},
	{1800, hex(0x9c8466)},
	{2500, hex(0x7a6650)},
	{3200, hex(0x948a80)},
	{3900, hex(0xc4c2bf)},
	{4500, hex(0xeef0f2)},
}

func tint(metres float64) rgb {
	if metres <= hypsometric[0].metres {
		return hypsometric[0].c
	}
	for k := 1; k < len(hypsometric); k++ {
		if metres < hypsometric[k].metres {
			a, b := hypsometric[k-1], hypsometric[k]
			return a.c.mix(b.c, (metres-a.metres)/(b.metres-a.metres))
		}
	}

	return hypsometric[len(hypsometric)-1].c
}

// The colours of the covers, laid over the low land and fading out by 2 km.
var biomeTints = [biomeCount]rgb{
	BiomeGlacier:     hex(0xeef0f2),
	BiomeTundra:      hex(0xb4b9a4),
	BiomeTaiga:       hex(0x6c8262),
	BiomeMixedForest: hex(0x6f8b5b),
	BiomeSteppe:      hex(0xb8b47e),
	BiomeMeadow:      hex(0x9db47a),
	BiomeDesert:      hex(0xd8c8a0),
	BiomeSavanna:     hex(0xc2b67a),
	BiomeJungle:      hex(0x5d8250),
	BiomeSwamp:       hex(0x5f6e4c),
}

var (
	shallow     = hex(0xa9c8d8)
	deep        = hex(0x45708f)
	lakeWater   = hex(0x9fc0d3)
	riverColour = hex(0x5b87a8)
	shadowTint  = rgb{0.40, 0.44, 0.58}
	lightTint   = rgb{1.0, 0.96, 0.86}
)

// palette is what the renderer reads of a map: the fields smoothed and the
// shade, on the grid of cells.
type palette struct {
	g      grid
	land   []float32 // the base colour of the land, r g b
	shade  []float32
	forest []float32
	depth  []float32 // metres of water, 0 on land
	water  []float32 // 1 at sea and lakes
	coast  []float32 // the land blurred, for the shadow on the water
	tex    *noise
}

func newPalette(m *Map) *palette {
	g := grid{w: m.Width, h: m.Height}
	pl := &palette{g: g, tex: newNoise(stream(m.Params.Seed, 99))}

	r, gr, b := make([]float32, g.n()), make([]float32, g.n()), make([]float32, g.n())
	forest := make([]float32, g.n())
	water := make([]float32, g.n())
	depth := make([]float32, g.n())
	for i, e := range m.Elevation {
		if e <= 0 || m.Lake[i] {
			water[i] = 1
			depth[i] = float32(math.Max(0, -float64(e)))
			if m.Lake[i] {
				depth[i] = 0
			}
			c := tint(0)
			r[i], gr[i], b[i] = float32(c.r), float32(c.g), float32(c.b)
			continue
		}
		c := tint(float64(e))
		if bt := biomeTints[m.Biome[i]]; m.Biome[i] >= BiomeGlacier {
			weight := 0.65 * (1 - smoothstep(600, 2200, float64(e)))
			if m.Biome[i] == BiomeGlacier {
				weight = 0.9
			}
			c = c.mix(bt, weight)
		}
		r[i], gr[i], b[i] = float32(c.r), float32(c.g), float32(c.b)
		if m.Biome[i].isForest() {
			forest[i] = 1
		}
	}

	soft := max(1, int(math.Round(2*m.Params.scale()*2)))
	r, gr, b = blur(g, r, soft, 2), blur(g, gr, soft, 2), blur(g, b, soft, 2)
	pl.land = make([]float32, 3*g.n())
	for i := range r {
		pl.land[3*i], pl.land[3*i+1], pl.land[3*i+2] = r[i], gr[i], b[i]
	}
	pl.forest = blur(g, forest, soft, 2)
	pl.water = water
	pl.depth = blur(g, depth, max(2, int(8*m.Params.scale())), 2)
	land := make([]float32, g.n())
	for i := range land {
		land[i] = 1 - water[i]
	}
	pl.coast = blur(g, land, max(1, soft), 2)
	pl.shade = hillshade(m)

	return pl
}

// hillshade lights the relief from three sides, mostly from the north-west
// as maps do, with the sun 45° high: 1 is flat ground, below is shadow.
func hillshade(m *Map) []float32 {
	g := grid{w: m.Width, h: m.Height}
	cell := m.Params.cellKm() * 1000
	type light struct{ x, y, z, w float64 }
	alt := math.Pi / 4
	var lights []light
	for _, l := range []struct{ az, w float64 }{{315, 0.6}, {270, 0.2}, {0, 0.2}} {
		az := l.az * math.Pi / 180
		lights = append(lights, light{math.Sin(az) * math.Cos(alt), -math.Cos(az) * math.Cos(alt), math.Sin(alt), l.w})
	}
	flat := math.Sin(alt)

	at := func(x, y int) float64 {
		x, y = clampInt(x, 0, g.w-1), clampInt(y, 0, g.h-1)
		return math.Max(0, float64(m.Elevation[y*g.w+x]))
	}
	shade := make([]float32, g.n())
	parallel(g.h, func(lo, hi int) {
		for y := lo; y < hi; y++ {
			for x := range g.w {
				// Sobel: the slope from the eight around.
				gx := (at(x+1, y-1) + 2*at(x+1, y) + at(x+1, y+1) - at(x-1, y-1) - 2*at(x-1, y) - at(x-1, y+1)) / (8 * cell)
				gy := (at(x-1, y+1) + 2*at(x, y+1) + at(x+1, y+1) - at(x-1, y-1) - 2*at(x, y-1) - at(x+1, y-1)) / (8 * cell)
				gx, gy = gx*reliefExaggeration, gy*reliefExaggeration
				l := math.Sqrt(gx*gx + gy*gy + 1)
				nx, ny, nz := -gx/l, -gy/l, 1/l
				s := 0.0
				for _, li := range lights {
					s += li.w * math.Max(0, nx*li.x+ny*li.y+nz*li.z)
				}
				shade[y*g.w+x] = float32(s / flat)
			}
		}
	})

	return shade
}

// bilinear reads a field between its cells.
func (pl *palette) bilinear(f []float32, stride, ch int, x, y float64) float64 {
	g := pl.g
	x, y = clamp(x-0.5, 0, float64(g.w-1)), clamp(y-0.5, 0, float64(g.h-1))
	cx, cy := min(int(x), g.w-2), min(int(y), g.h-2)
	u, v := x-float64(cx), y-float64(cy)
	i := cy*g.w + cx
	at := func(j int) float64 { return float64(f[j*stride+ch]) }

	return at(i)*(1-u)*(1-v) + at(i+1)*u*(1-v) + at(i+g.w)*(1-u)*v + at(i+g.w+1)*u*v
}

// colour is the colour of the map at a point, in cells.
func (pl *palette) colour(x, y, pixel float64) rgb {
	w := pl.bilinear(pl.water, 1, 0, x, y)
	coast := pl.bilinear(pl.coast, 1, 0, x, y)

	land := rgb{pl.bilinear(pl.land, 3, 0, x, y), pl.bilinear(pl.land, 3, 1, x, y), pl.bilinear(pl.land, 3, 2, x, y)}
	if f := pl.bilinear(pl.forest, 1, 0, x, y); f > 0 {
		// Woods: a fine grain, a little darker and greener.
		grain := pl.tex.fbm(x*1.7/pixel*0.5, y*1.7/pixel*0.5, 3)
		land = land.mix(rgb{0.32, 0.45, 0.30}, f*(0.18+0.10*grain))
	}
	s := pl.bilinear(pl.shade, 1, 0, x, y)
	if s < 1 {
		land = land.mul(shadowTint.mix(rgb{1, 1, 1}, s))
	} else {
		land = land.mix(lightTint, math.Min(1, (s-1)*0.9))
	}

	if w <= 0.001 {
		return land
	}

	depth := pl.bilinear(pl.depth, 1, 0, x, y)
	sea := shallow.mix(deep, math.Pow(clamp(depth/4500, 0, 1), 0.55))
	if depth < 1 {
		sea = lakeWater
	}
	sea = sea.scale(1 + 0.025*pl.tex.at(x*0.08, y*0.08))
	// The light shallows along the shore and the soft shadow of the land
	// cast to the south-east.
	sea = sea.mix(rgb{0.80, 0.88, 0.91}, 0.45*smoothstep(0.05, 0.5, coast))
	cast := pl.bilinear(pl.coast, 1, 0, x-0.8, y-0.8)
	sea = sea.scale(1 - 0.12*smoothstep(0.1, 0.7, cast))

	return land.mix(sea, smoothstep(0.35, 0.65, w))
}

// Render draws the physical map width pixels wide.
func Render(m *Map, width int) *image.RGBA {
	pl := newPalette(m)
	height := int(math.Round(float64(width) * float64(m.Height) / float64(m.Width)))
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	k := float64(m.Width) / float64(width)

	parallel(height, func(lo, hi int) {
		for py := lo; py < hi; py++ {
			for px := range width {
				var sum rgb
				for sy := range supersample {
					for sx := range supersample {
						x := (float64(px) + (float64(sx)+0.5)/supersample) * k
						y := (float64(py) + (float64(sy)+0.5)/supersample) * k
						c := pl.colour(x, y, k)
						sum = rgb{sum.r + c.r, sum.g + c.g, sum.b + c.b}
					}
				}
				img.SetRGBA(px, py, toRGBA(sum.scale(1.0/(supersample*supersample))))
			}
		}
	})
	drawRivers(img, m, k)

	return img
}

func toRGBA(c rgb) color.RGBA {
	to := func(v float64) uint8 { return uint8(clamp(v*255+0.5, 0, 255)) }
	return color.RGBA{to(c.r), to(c.g), to(c.b), 255}
}

// drawRivers draws the rivers as smooth lines, wider downstream: the
// strokes mark how much of each pixel they cover, keeping the most where
// they overlap, and the river colour is laid on by that cover once.
func drawRivers(img *image.RGBA, m *Map, k float64) {
	b := img.Bounds()
	cover := make([]float32, b.Dx()*b.Dy())
	area := m.Params.scale() * m.Params.scale()
	wander := newNoise(stream(m.Params.Seed, 98))
	for _, rv := range m.Rivers {
		pts := chaikin(meander(wander, rv.Points), 2)
		flows := resample(rv.Flow, len(pts))
		for n := 1; n < len(pts); n++ {
			f := float64(flows[n]) / area
			if f < riverFlow[1] || (k > 1.5 && f < riverFlow[2]) {
				continue
			}
			// Thin rivers stay a pixel wide but pale, so they do not
			// flicker in and out along the diagonals.
			width := clamp(0.5+0.55*math.Log2(f/riverFlow[1]), 0.6, 3.2) / math.Max(1, k*0.8)
			strength := clamp(width, 0.55, 1)
			stroke(cover, b.Dx(), b.Dy(), pts[n-1], pts[n], math.Max(1, width), k, strength)
		}
	}

	for i, c := range cover {
		if c <= 0 {
			continue
		}
		x, y := i%b.Dx(), i/b.Dx()
		o := img.RGBAAt(x, y)
		img.SetRGBA(x, y, toRGBA(rgb{float64(o.R) / 255, float64(o.G) / 255, float64(o.B) / 255}.mix(riverColour, 0.9*float64(c))))
	}
}

// meander moves the points of a river off the centres of their cells a
// little, so that it does not run along the straight lines of the grid.
func meander(n *noise, pts [][2]float32) [][2]float32 {
	out := make([][2]float32, len(pts))
	for i, p := range pts {
		x, y := float64(p[0]), float64(p[1])
		out[i] = [2]float32{p[0] + float32(0.45*n.at(x*0.35, y*0.35)), p[1] + float32(0.45*n.at(x*0.35+41, y*0.35-17))}
	}

	return out
}

// chaikin rounds a line by cutting its corners.
func chaikin(pts [][2]float32, passes int) [][2]float32 {
	for range passes {
		if len(pts) < 3 {
			return pts
		}
		out := [][2]float32{pts[0]}
		for i := 0; i+1 < len(pts); i++ {
			a, b := pts[i], pts[i+1]
			out = append(out,
				[2]float32{0.75*a[0] + 0.25*b[0], 0.75*a[1] + 0.25*b[1]},
				[2]float32{0.25*a[0] + 0.75*b[0], 0.25*a[1] + 0.75*b[1]})
		}
		pts = append(out, pts[len(pts)-1])
	}

	return pts
}

func resample(f []float32, n int) []float32 {
	out := make([]float32, n)
	for i := range out {
		out[i] = f[min(len(f)-1, i*len(f)/n)]
	}

	return out
}

// stroke marks the cover of a segment as a row of soft discs.
func stroke(cover []float32, w, h int, a, b [2]float32, width, k, strength float64) {
	ax, ay := float64(a[0])/k, float64(a[1])/k
	bx, by := float64(b[0])/k, float64(b[1])/k
	steps := max(1, int(math.Hypot(bx-ax, by-ay)*3))
	r := width / 2
	for s := 0; s <= steps; s++ {
		t := float64(s) / float64(steps)
		cx, cy := ax+(bx-ax)*t, ay+(by-ay)*t
		for y := max(0, int(cy-r-1)); y <= min(h-1, int(cy+r+1)); y++ {
			for x := max(0, int(cx-r-1)); x <= min(w-1, int(cx+r+1)); x++ {
				d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
				if c := float32(strength * clamp(r-d+0.5, 0, 1)); c > cover[y*w+x] {
					cover[y*w+x] = c
				}
			}
		}
	}
}

// RenderProvinces draws the provinces over the physical map: each its own
// muted colour, thin dark borders, the zones of the sea outlined, the free
// cities and the centres marked.
func RenderProvinces(m *Map, width int) *image.RGBA {
	img := Render(m, width)
	k := float64(m.Width) / float64(width)
	b := img.Bounds()

	cell := func(px, py int) int32 {
		x := clampInt(int((float64(px)+0.5)*k), 0, m.Width-1)
		y := clampInt(int((float64(py)+0.5)*k), 0, m.Height-1)
		return m.Province[y*m.Width+x]
	}
	colours := make([]rgb, len(m.Provinces))
	for i := range colours {
		h := uint32(i)*2654435761 + 12345
		colours[i] = rgb{0.35 + 0.5*float64(h&255)/255, 0.35 + 0.5*float64(h>>8&255)/255, 0.35 + 0.5*float64(h>>16&255)/255}
	}

	parallel(b.Dy(), func(lo, hi int) {
		for py := lo; py < hi; py++ {
			for px := range b.Dx() {
				l := cell(px, py)
				o := img.RGBAAt(px, py)
				c := rgb{float64(o.R) / 255, float64(o.G) / 255, float64(o.B) / 255}
				sea := m.Provinces[l].Sea
				if !sea {
					c = c.mix(colours[l], 0.35)
				}
				border := (px+1 < b.Dx() && cell(px+1, py) != l) || (py+1 < b.Dy() && cell(px, py+1) != l)
				if border {
					other := cell(px, py+1)
					if px+1 < b.Dx() && cell(px+1, py) != l {
						other = cell(px+1, py)
					}
					switch {
					case sea && m.Provinces[other].Sea:
						c = c.mix(rgb{1, 1, 1}, 0.35)
					case !sea && !m.Provinces[other].Sea:
						c = c.mix(rgb{0.15, 0.15, 0.15}, 0.55)
					}
				}
				img.SetRGBA(px, py, toRGBA(c))
			}
		}
	})

	for _, pr := range m.Provinces {
		x, y := (float64(pr.CenterX)+0.5)/k, (float64(pr.CenterY)+0.5)/k
		switch {
		case pr.FreeCity:
			dot(img, x, y, 3.5, rgb{0.75, 0.1, 0.1})
		case !pr.Sea:
			dot(img, x, y, 1.2, rgb{0.1, 0.1, 0.1})
		}
	}

	return img
}

func dot(img *image.RGBA, cx, cy, r float64, c rgb) {
	for y := int(cy - r - 1); y <= int(cy+r+1); y++ {
		for x := int(cx - r - 1); x <= int(cx+r+1); x++ {
			if !(image.Point{x, y}.In(img.Rect)) {
				continue
			}
			cover := clamp(r-math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)+0.5, 0, 1)
			o := img.RGBAAt(x, y)
			img.SetRGBA(x, y, toRGBA(rgb{float64(o.R) / 255, float64(o.G) / 255, float64(o.B) / 255}.mix(c, cover)))
		}
	}
}
