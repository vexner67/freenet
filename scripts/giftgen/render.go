package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"runtime"
	"sync"
)

// ---- vectors ----

type V struct{ X, Y, Z float64 }

func v3(x, y, z float64) V            { return V{x, y, z} }
func (a V) Add(b V) V                 { return V{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func (a V) Sub(b V) V                 { return V{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func (a V) Mul(s float64) V           { return V{a.X * s, a.Y * s, a.Z * s} }
func (a V) MulV(b V) V                { return V{a.X * b.X, a.Y * b.Y, a.Z * b.Z} }
func (a V) Dot(b V) float64           { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func (a V) Len() float64              { return math.Sqrt(a.Dot(a)) }
func (a V) Norm() V                   { return a.Mul(1 / a.Len()) }
func (a V) Get(i int) float64         { return [3]float64{a.X, a.Y, a.Z}[i] }
func (a V) Cross(b V) V               { return V{a.Y*b.Z - a.Z*b.Y, a.Z*b.X - a.X*b.Z, a.X*b.Y - a.Y*b.X} }
func lerp(a, b V, t float64) V        { return a.Mul(1 - t).Add(b.Mul(t)) }
func clamp(x, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, x)) }
func smoothstep(a, b, x float64) float64 {
	t := clamp((x-a)/(b-a), 0, 1)
	return t * t * (3 - 2*t)
}

func rotX(p V, a float64) V {
	c, s := math.Cos(a), math.Sin(a)
	return V{p.X, c*p.Y - s*p.Z, s*p.Y + c*p.Z}
}

func rotY(p V, a float64) V {
	c, s := math.Cos(a), math.Sin(a)
	return V{c*p.X + s*p.Z, p.Y, -s*p.X + c*p.Z}
}

func rotZ(p V, a float64) V {
	c, s := math.Cos(a), math.Sin(a)
	return V{c*p.X - s*p.Y, s*p.X + c*p.Y, p.Z}
}

func reflect(d, n V) V { return d.Sub(n.Mul(2 * d.Dot(n))) }

// hex is an sRGB colour turned linear.
func hex(c uint32) V {
	f := func(b uint32) float64 { return math.Pow(float64(b)/255, 2.2) }
	return V{f(c >> 16 & 255), f(c >> 8 & 255), f(c & 255)}
}

// ---- random ----

type rng struct{ s uint64 }

func (r *rng) f() float64 {
	r.s ^= r.s << 13
	r.s ^= r.s >> 7
	r.s ^= r.s << 17
	return float64(r.s>>11) / float64(1<<53)
}

func (r *rng) sphere() V {
	for {
		p := V{r.f()*2 - 1, r.f()*2 - 1, r.f()*2 - 1}
		if p.Dot(p) <= 1 {
			return p
		}
	}
}

// ---- 2D shapes ----

func sdBox2(x, y, cx, cy, hx, hy float64) float64 {
	dx, dy := math.Abs(x-cx)-hx, math.Abs(y-cy)-hy
	return math.Hypot(math.Max(dx, 0), math.Max(dy, 0)) + math.Min(math.Max(dx, dy), 0)
}

// glyph is the "f" of the logo (favicon.svg, a 64 box, y down).
func glyph(x, y float64) float64 {
	d := sdBox2(x, y, 42, 15.5, 2, 4.5)
	dx, dy := x-40, y-24.5
	arc := math.Max(math.Abs(math.Hypot(dx, dy)-9)-4.5, math.Max(dx, dy))
	d = math.Min(d, arc)
	d = math.Min(d, sdBox2(x, y, 31, 38.25, 4.5, 13.75))
	d = math.Min(d, sdBox2(x, y, 31, 31, 12, 4))
	return d
}

// fN is the glyph in [-1, 1] (y up), its distance in those units.
func fN(u, v float64) float64 {
	return glyph(31.5+u*21, 31.5-v*21) / 21
}

// ---- 3D shapes ----

func sdSphere(p V, r float64) float64 { return p.Len() - r }

func sdTorus(p V, R, r float64) float64 { // ring in the xz plane
	return math.Hypot(math.Hypot(p.X, p.Z)-R, p.Y) - r
}

func sdRoundCyl(r2, y, R, h, round float64) float64 { // axis y
	dx, dy := r2-R+round, math.Abs(y)-h+round
	return math.Min(math.Max(dx, dy), 0) + math.Hypot(math.Max(dx, 0), math.Max(dy, 0)) - round
}

func smin(a, b, k float64) float64 {
	h := clamp(0.5+0.5*(b-a)/k, 0, 1)
	return b*(1-h) + a*h - k*h*(1-h)
}

// brilliant is a round brilliant cut, its table up, girdle radius 1.
func brilliant(p V) float64 {
	d := math.Hypot(p.X, p.Z) - 1
	d = math.Max(d, p.Y-0.36)
	plane := func(az, tilt, through float64, down bool) float64 {
		s, c := math.Sin(tilt), math.Cos(tilt)
		n := V{math.Cos(az) * s, c, math.Sin(az) * s}
		pt := V{math.Cos(az) * through, 0.03, math.Sin(az) * through}
		if down {
			n.Y = -c
			pt.Y = -0.03
		}
		return p.Dot(n) - pt.Dot(n)
	}
	for k := 0; k < 8; k++ {
		a := float64(k) * math.Pi / 4
		d = math.Max(d, plane(a, (90-34.5)*math.Pi/180, 1, false))
		d = math.Max(d, plane(a+math.Pi/8, (90-42)*math.Pi/180, 1.0, false))
		d = math.Max(d, plane(a, (90-40.8)*math.Pi/180, 1, true))
		d = math.Max(d, plane(a+math.Pi/8, (90-43)*math.Pi/180, 1.0, true))
	}
	return d
}

// ---- materials ----

const (
	metal = iota
	gem
	gloss
	velvet
)

type Mat struct {
	Kind  int
	Color V // metal: F0; gem: absorption; gloss, velvet: albedo
	Rough float64
	IOR   float64
	Disp  float64
	Body  func(V) float64 // a gem's own shape, for the paths inside it
}

type Scene struct {
	Name    string
	SDF     func(V) (float64, int)
	Mats    []Mat
	Cam     V
	FOV     float64
	Inner   uint32 // backdrop at the centre
	Outer   uint32 // backdrop at the edge
	Pattern uint32
	Light   float64 // exposure
	Bounds  float64 // radius of a sphere holding the object
	// Bare draws the object alone, with its shadow, on nothing: a model
	// of a collectible gift, which the site lays on its backdrop.
	Bare bool
}

// ---- lighting ----

var (
	keyDir  = V{-0.55, 0.75, 0.55}.Norm()
	rimDir  = V{0.85, 0.35, -0.45}.Norm()
	fillDir = V{0.7, -0.05, 0.75}.Norm()
	topDir  = V{0.1, 1, 0.1}.Norm()
	backDir = V{-0.3, 0.2, -1}.Norm()
	kickDir = V{-0.9, -0.2, 0.3}.Norm()
)

// env is a photo studio: dark floor, soft walls and big softboxes, so
// metal and stones mirror bright shapes against dark.
func env(d V) V {
	t := 0.5 * (d.Y + 1)
	c := lerp(V{0.006, 0.005, 0.007}, V{0.045, 0.045, 0.05}, smoothstep(0.35, 1, t))
	lobe := func(dir V, k, power float64) float64 { return power * math.Exp(-(1-d.Dot(dir))*k) }
	c = c.Add(V{1, 0.96, 0.9}.Mul(lobe(keyDir, 30, 11)))
	c = c.Add(V{0.9, 0.95, 1}.Mul(lobe(rimDir, 45, 7)))
	c = c.Add(V{1, 1, 1}.Mul(lobe(fillDir, 16, 1.4)))
	c = c.Add(V{1, 0.98, 0.95}.Mul(lobe(topDir, 24, 3.2)))
	c = c.Add(V{0.9, 0.9, 1}.Mul(lobe(backDir, 20, 2.6)))
	c = c.Add(V{1, 0.9, 0.8}.Mul(lobe(kickDir, 26, 2.0)))
	// strip lights left and right, long and thin
	strip := func(az, power float64) float64 {
		x := math.Sin(az)*d.Z - math.Cos(az)*d.X
		return power * math.Exp(-x*x*120) * smoothstep(-0.6, 0.2, d.Y) * smoothstep(0.95, 0.6, d.Y)
	}
	c = c.Add(V{1, 1, 1}.Mul(strip(0.9, 2.2) + strip(-2.3, 1.6)))
	// a horizon line, as the edge of a studio table
	c = c.Add(V{0.5, 0.5, 0.55}.Mul(math.Exp(-math.Pow((d.Y+0.05)*12, 2)) * 0.5))
	return c
}

func envBlur(d V, rough float64, r *rng, n int) V {
	if rough < 0.005 {
		return env(d)
	}
	var sum V
	for i := 0; i < n; i++ {
		sum = sum.Add(env(d.Add(r.sphere().Mul(rough)).Norm()))
	}
	return sum.Mul(1 / float64(n))
}

func schlick(cos float64, f0 V) V {
	m := math.Pow(1-clamp(cos, 0, 1), 5)
	return f0.Add(V{1, 1, 1}.Sub(f0).Mul(m))
}

func fresnel(cosi, eta float64) float64 { // dielectric, eta = n1/n2
	sint2 := eta * eta * (1 - cosi*cosi)
	if sint2 >= 1 {
		return 1
	}
	cost := math.Sqrt(1 - sint2)
	rs := (eta*cosi - cost) / (eta*cosi + cost)
	rp := (cosi - eta*cost) / (cosi + eta*cost)
	return (rs*rs + rp*rp) / 2
}

// ---- marching ----

func (s *Scene) normal(p V) V {
	const e = 2e-4
	k := []V{{1, -1, -1}, {-1, -1, 1}, {-1, 1, -1}, {1, 1, 1}}
	var n V
	for _, d := range k {
		v, _ := s.SDF(p.Add(d.Mul(e)))
		n = n.Add(d.Mul(v))
	}
	return n.Norm()
}

func bodyNormal(f func(V) float64, p V) V {
	const e = 2e-4
	k := []V{{1, -1, -1}, {-1, -1, 1}, {-1, 1, -1}, {1, 1, 1}}
	var n V
	for _, d := range k {
		n = n.Add(d.Mul(f(p.Add(d.Mul(e)))))
	}
	return n.Norm()
}

func (s *Scene) march(ro, rd V) (float64, int, bool) {
	// start at the bounding sphere
	b := ro.Dot(rd)
	c := ro.Dot(ro) - s.Bounds*s.Bounds
	h := b*b - c
	if h < 0 {
		return 0, 0, false
	}
	t := math.Max(-b-math.Sqrt(h), 0)
	tmax := -b + math.Sqrt(h)
	for i := 0; i < 400 && t < tmax; i++ {
		d, m := s.SDF(ro.Add(rd.Mul(t)))
		if d < 1e-4 {
			return t, m, true
		}
		t += d * 0.8
	}
	return 0, 0, false
}

func (s *Scene) ao(p, n V) float64 {
	occ, w := 0.0, 1.0
	for i := 1; i <= 5; i++ {
		h := 0.02 * float64(i)
		d, _ := s.SDF(p.Add(n.Mul(h)))
		occ += (h - d) * w
		w *= 0.6
	}
	return clamp(1-3*occ, 0, 1)
}

func (s *Scene) shade(p, rd V, m Mat, r *rng) V {
	n := s.normal(p)
	cos := -rd.Dot(n)
	refl := reflect(rd, n)
	ao := s.ao(p, n)
	switch m.Kind {
	case metal:
		return schlick(cos, m.Color).MulV(envBlur(refl, m.Rough, r, 4+int(m.Rough*40))).Mul(0.15 + 0.85*ao)
	case gloss:
		diffuse := m.Color.MulV(envBlur(n, 0.9, r, 16)).Mul(0.7)
		f := fresnel(cos, 1/1.5)
		return diffuse.Mul(ao).Add(envBlur(refl, m.Rough, r, 16).Mul(f))
	case velvet:
		diffuse := m.Color.MulV(envBlur(n, 0.9, r, 4)).Mul(0.3)
		sheen := m.Color.Add(V{0.2, 0.2, 0.2}).Mul(math.Pow(1-clamp(cos, 0, 1), 3) * 0.5)
		return diffuse.Add(sheen).Mul(ao)
	case gem:
		return gemShade(p, rd, n, m, r)
	}
	return V{}
}

func gemShade(p, rd, n V, m Mat, r *rng) V {
	cos := -rd.Dot(n)
	f := fresnel(cos, 1/m.IOR)
	out := env(reflect(rd, n)).Mul(f)
	var in [3]float64
	for ch := 0; ch < 3; ch++ {
		ior := m.IOR + float64(ch-1)*m.Disp
		eta := 1 / ior
		k := 1 - eta*eta*(1-cos*cos)
		d := rd.Mul(eta).Add(n.Mul(eta*cos - math.Sqrt(k))).Norm()
		in[ch] = inside(p.Sub(n.Mul(1e-3)), d, ior, m, ch)
	}
	return out.Add(V{in[0], in[1], in[2]}.Mul(1 - f))
}

func inside(q, d V, ior float64, m Mat, ch int) float64 {
	through, got := 1.0, 0.0
	for bounce := 0; bounce < 10 && through > 0.01; bounce++ {
		t := 0.0
		for i := 0; i < 200; i++ {
			s := -m.Body(q.Add(d.Mul(t)))
			if s < 2e-4 {
				break
			}
			t += math.Max(s*0.9, 2e-4)
		}
		through *= math.Exp(-m.Color.Get(ch) * t)
		hp := q.Add(d.Mul(t))
		nn := bodyNormal(m.Body, hp)
		cosi := d.Dot(nn)
		if cosi < 0 {
			nn, cosi = nn.Mul(-1), -cosi
		}
		eta := ior
		k := 1 - eta*eta*(1-cosi*cosi)
		if k < 0 {
			d = reflect(d, nn)
			q = hp.Sub(nn.Mul(1e-3))
			continue
		}
		fr := fresnel(cosi, eta)
		outDir := d.Mul(eta).Add(nn.Mul(-eta*cosi + math.Sqrt(k))).Norm()
		got += through * (1 - fr) * env(outDir).Get(ch)
		through *= fr
		d = reflect(d, nn)
		q = hp.Sub(nn.Mul(1e-3))
	}
	return got
}

// ---- picture ----

func aces(x float64) float64 {
	return clamp(x*(2.51*x+0.03)/(x*(2.43*x+0.59)+0.14), 0, 1)
}

func toSRGB(x float64) float64 { return math.Pow(clamp(x, 0, 1), 1/2.2) }

type rgba struct{ r, g, b, a float64 }

// Render draws the scene on its backdrop, size×size.
func Render(s *Scene, size, ss int, path string) error {
	obj := make([]rgba, size*size)
	var wg sync.WaitGroup
	rows := make(chan int, size)
	for y := 0; y < size; y++ {
		rows <- y
	}
	close(rows)
	fwd := s.Cam.Mul(-1).Norm()
	right := fwd.Cross(V{0, 1, 0}).Norm()
	up := right.Cross(fwd)
	tanH := math.Tan(s.FOV * math.Pi / 360)
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for y := range rows {
				for x := 0; x < size; x++ {
					r := &rng{s: uint64(y*size+x)*2654435761 + 12345}
					var acc rgba
					for sy := 0; sy < ss; sy++ {
						for sx := 0; sx < ss; sx++ {
							u := (float64(x)+(float64(sx)+r.f())/float64(ss))/float64(size)*2 - 1
							v := 1 - (float64(y)+(float64(sy)+r.f())/float64(ss))/float64(size)*2
							rd := fwd.Add(right.Mul(u * tanH)).Add(up.Mul(v * tanH)).Norm()
							t, mi, ok := s.march(s.Cam, rd)
							if !ok {
								continue
							}
							c := s.shade(s.Cam.Add(rd.Mul(t)), rd, s.Mats[mi], r).Mul(s.Light)
							acc.r += toSRGB(aces(c.X))
							acc.g += toSRGB(aces(c.Y))
							acc.b += toSRGB(aces(c.Z))
							acc.a++
						}
					}
					n := float64(ss * ss)
					if acc.a > 0 {
						obj[y*size+x] = rgba{acc.r / acc.a, acc.g / acc.a, acc.b / acc.a, acc.a / n}
					}
				}
			}
		}()
	}
	wg.Wait()

	shadow := blurMask(obj, size)
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	inner, outer, pat := srgb(s.Inner), srgb(s.Outer), srgb(s.Pattern)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if s.Bare {
				o := obj[y*size+x]
				// the shadow is black under the object, half as dark as
				// on the backdrop
				a := o.a + 0.45*shadow[y*size+x]*(1-o.a)
				if a > 0 {
					c := V{o.r, o.g, o.b}.Mul(o.a / a)
					img.SetNRGBA(x, y, color.NRGBA{to8(c.X), to8(c.Y), to8(c.Z), to8(a)})
				}
				continue
			}
			bg, a := backdrop(x, y, size, inner, outer, pat)
			sh := shadow[y*size+x]
			bg = bg.Mul(1 - 0.55*sh)
			o := obj[y*size+x]
			c := V{o.r, o.g, o.b}.Mul(o.a).Add(bg.Mul(1 - o.a))
			img.SetNRGBA(x, y, color.NRGBA{to8(c.X), to8(c.Y), to8(c.Z), to8(a)})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(f, img)
}

func to8(x float64) uint8 { return uint8(clamp(x*255+0.5, 0, 255)) }

func srgb(c uint32) V {
	return V{float64(c>>16&255) / 255, float64(c>>8&255) / 255, float64(c&255) / 255}
}

// blurMask is the object's shadow on the backdrop: its coverage, moved
// down and blurred.
func blurMask(obj []rgba, size int) []float64 {
	m := make([]float64, size*size)
	off := size * 5 / 100
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			sy := y - off
			if sy >= 0 {
				m[y*size+x] = obj[sy*size+x].a
			}
		}
	}
	rad := size * 4 / 100
	for pass := 0; pass < 3; pass++ {
		m = boxBlur(m, size, rad, true)
		m = boxBlur(m, size, rad, false)
	}
	return m
}

func boxBlur(m []float64, size, rad int, horizontal bool) []float64 {
	out := make([]float64, len(m))
	for a := 0; a < size; a++ {
		for b := 0; b < size; b++ {
			sum, n := 0.0, 0
			for k := -rad; k <= rad; k++ {
				c := b + k
				if c < 0 || c >= size {
					continue
				}
				if horizontal {
					sum += m[a*size+c]
				} else {
					sum += m[c*size+a]
				}
				n++
			}
			if horizontal {
				out[a*size+b] = sum / float64(n)
			} else {
				out[b*size+a] = sum / float64(n)
			}
		}
	}
	return out
}

// backdrop is a rounded tile: a glow in the middle and a pattern of small
// "f" in rings around it, as the backdrops of collectible gifts.
func backdrop(x, y, size int, inner, outer, pat V) (V, float64) {
	const ss = 4
	var col V
	alpha := 0.0
	for sy := 0; sy < ss; sy++ {
		for sx := 0; sx < ss; sx++ {
			u := (float64(x)+(float64(sx)+0.5)/ss)/float64(size)*2 - 1
			v := (float64(y)+(float64(sy)+0.5)/ss)/float64(size)*2 - 1
			// the tile: a rounded square
			const rr = 0.36
			dx, dy := math.Abs(u)-(1-rr), math.Abs(v)-(1-rr)
			edge := math.Hypot(math.Max(dx, 0), math.Max(dy, 0)) + math.Min(math.Max(dx, dy), 0) - rr
			if edge > 0 {
				continue
			}
			alpha++
			r := math.Hypot(u, v+0.05)
			c := lerp(inner, outer, smoothstep(0, 1.25, r))
			c = c.Add(V{1, 1, 1}.Mul(0.10 * math.Exp(-r*r*4)))
			col = col.Add(lerp(c, pat, patternAt(u, v)*0.13))
		}
	}
	if alpha == 0 {
		return V{}, 0
	}
	return col.Mul(1 / alpha), alpha / (ss * ss)
}

// patternAt: rings of small glyphs, fewer and smaller towards the middle,
// where the gift covers them.
func patternAt(u, v float64) float64 {
	r := math.Hypot(u, v)
	best := 0.0
	rings := []struct {
		radius, size float64
		count        int
		phase        float64
	}{
		{0.66, 0.05, 10, 0.31},
		{0.92, 0.06, 14, 0},
		{1.18, 0.065, 18, 0.17},
		{1.42, 0.07, 22, 0.05},
	}
	for _, ring := range rings {
		if math.Abs(r-ring.radius) > ring.size*1.5 {
			continue
		}
		a := math.Atan2(v, u) + ring.phase
		sector := 2 * math.Pi / float64(ring.count)
		k := math.Floor(a/sector + 0.5)
		ca := k*sector - ring.phase
		cx, cy := ring.radius*math.Cos(ca), ring.radius*math.Sin(ca)
		d := fN((u-cx)/ring.size, -(v-cy)/ring.size) * ring.size
		cover := 1 - smoothstep(-0.004, 0.004, d)
		best = math.Max(best, cover)
	}
	return best
}
