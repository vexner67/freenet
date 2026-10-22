// Command giftgen renders the site's own gifts, the "Freenet" and "Freenet
// Prime" collections of the catalogue (user/internal/user/gift.go): each
// gift is a scene of signed distance functions — gold, gems and enamel lit
// by a photo studio — on a rounded backdrop with a pattern of the logo, as
// collectible gifts are shown. It needs nothing but the standard library:
//
//	cd scripts/giftgen && GOWORK=off go run . -out ../../web/static/gifts
//
// With -models it draws the models of collectible gifts instead, each gift
// in six metals and stones on nothing, into <out>/models:
//
//	cd scripts/giftgen && GOWORK=off go run . -models -size 256 -out ../../web/static/gifts
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

var gold = V{1.0, 0.6, 0.16}

func crystal() *Scene {
	body := func(p V) float64 {
		q := rotY(rotX(p, -0.32), 0.2)
		const k = 0.95
		return brilliant(q.Mul(1/k).Add(V{0, -0.22, 0})) * k
	}
	return &Scene{
		Name: "freenet_crystal",
		SDF:  func(p V) (float64, int) { return body(p), 0 },
		Mats: []Mat{{Kind: gem, Color: V{1.5, 0.55, 0.06}, IOR: 2.1, Disp: 0.035, Body: body}},
		Cam:  V{0, 0, 4.2}, FOV: 36, Light: 1.0, Bounds: 1.4,
		Inner: 0x3d7fe0, Outer: 0x0a1c45, Pattern: 0x8bbcff,
	}
}

func coin() *Scene {
	const R, h = 1.0, 0.11
	floor := h - 0.03
	sdf := func(p V) (float64, int) {
		q := rotY(rotX(p, 0.42), -0.5)
		r2 := math.Hypot(q.X, q.Y)
		ang := math.Atan2(q.Y, q.X)
		az := math.Abs(q.Z)
		reed := 0.004 * math.Cos(ang*140)
		base := sdRoundCyl(r2+reed*smoothstep(0.9, 1, r2), q.Z, R, h, 0.03)
		carve := math.Max(r2-0.86, floor-az)
		body := math.Max(base, -carve)
		mat := 0
		if r2 < 0.858 && az < floor+0.004 {
			mat = 1
		}
		sector := 2 * math.Pi / 64
		a := math.Mod(ang+10*math.Pi+sector/2, sector) - sector/2
		dots := V{r2*math.Cos(a) - 0.80, r2 * math.Sin(a), az - floor}.Len() - 0.013
		g := fN(q.X/0.52, q.Y/0.52) * 0.52
		relief := math.Max(g+0.004, math.Abs(az-(floor+0.012))-0.008) - 0.004
		if d := math.Min(dots, relief); d < body {
			return d, 2
		}
		return body, mat
	}
	return &Scene{
		Name: "freenet_coin",
		SDF:  sdf,
		Mats: []Mat{
			{Kind: metal, Color: gold, Rough: 0.08},
			{Kind: metal, Color: gold, Rough: 0.015},
			{Kind: metal, Color: gold, Rough: 0.32},
		},
		Cam: V{0, 0, 4.2}, FOV: 36, Light: 1.0, Bounds: 1.2,
		Inner: 0x2fa070, Outer: 0x0a3022, Pattern: 0x86e8b8,
	}
}

func ring() *Scene {
	const R = 0.6
	sdf := func(p V) (float64, int) {
		q := rotX(rotY(p, 0.5), -0.75).Mul(1 / 1.12).Add(V{0, 0.1, 0})
		// the band, in the xy plane, thicker under the head
		thick := 0.075 + 0.07*smoothstep(-0.2, R, q.Y)
		band := math.Hypot(math.Hypot(q.X, q.Y)-R, q.Z*1.25)/1.25 - thick
		// the head: an oval seal on top
		rr := math.Hypot(q.X*0.82, q.Z)
		head := sdRoundCyl(rr, q.Y-0.7, 0.33, 0.11, 0.05)
		body := smin(band, head, 0.12)
		body = math.Max(body, -math.Max(rr-0.265, 0.79-q.Y))
		onyx := math.Max(rr-0.267, math.Abs(q.Y-0.785)-0.012)
		g := fN(q.X/0.19, -q.Z/0.19) * 0.19
		f := math.Max(g+0.003, math.Abs(q.Y-0.8)-0.006) - 0.003
		switch {
		case f < body && f < onyx:
			return f, 2
		case onyx < body:
			return onyx, 1
		}
		return body, 0
	}
	return &Scene{
		Name: "freenet_ring",
		SDF:  sdf,
		Mats: []Mat{
			{Kind: metal, Color: gold, Rough: 0.06},
			{Kind: gloss, Color: V{0.004, 0.004, 0.005}, Rough: 0.02},
			{Kind: metal, Color: gold, Rough: 0.12},
		},
		Cam: V{0, 0, 4.2}, FOV: 36, Light: 1.0, Bounds: 1.3,
		Inner: 0x8f5fe6, Outer: 0x1c0a42, Pattern: 0xcbb0ff,
	}
}

func crown() *Scene {
	const R = 0.86
	points := 5
	gemsAt := func(p V, odd bool) float64 {
		d := math.Inf(1)
		for k := 0; k < points; k++ {
			if (k%2 == 1) != odd {
				continue
			}
			a := float64(k)*2*math.Pi/float64(points) + math.Pi/float64(points) + math.Pi/2
			c := V{(R - 0.02) * math.Cos(a), -0.2, (R - 0.02) * math.Sin(a)}
			d = math.Min(d, sdSphere(p.Sub(c), 0.12))
		}
		return d
	}
	local := func(p V) V { return rotY(rotX(p, 0.32), 0.25).Add(V{0, 0.12, 0}) }
	ruby := func(p V) float64 { return gemsAt(local(p), false) }
	sapphire := func(p V) float64 { return gemsAt(local(p), true) }
	sdf := func(p V) (float64, int) {
		q := local(p)
		r := math.Hypot(q.X, q.Z)
		a := math.Atan2(q.Z, q.X) - math.Pi/2
		sector := 2 * math.Pi / float64(points)
		f := math.Abs(math.Mod(a+20*math.Pi, sector)/sector - 0.5) // 0 at a point, 0.5 between
		top := 0.04 + 0.6*math.Pow(1-2*f, 1.7)
		wall := math.Abs(r-R) - 0.03
		band := math.Max(wall, math.Abs(q.Y+0.2)-0.2) - 0.012
		spikes := math.Max(wall, math.Max(q.Y-top, -q.Y)) * 0.7
		body := math.Min(band, spikes)
		body = math.Min(body, math.Hypot(r-R, q.Y+0.4)-0.045)
		body = math.Min(body, math.Hypot(r-R, q.Y)-0.04)
		for k := 0; k < points; k++ {
			pa := float64(k)*sector + sector/2 + math.Pi/2
			body = math.Min(body, sdSphere(q.Sub(V{R * math.Cos(pa), 0.68, R * math.Sin(pa)}), 0.075))
		}
		cap := math.Max(math.Hypot(r/0.8, (q.Y+0.05)/0.55)*0.55-0.55, -q.Y-0.1)
		cross := sdSphere(q.Sub(V{0, 0.5, 0}), 0.09)
		d, m := body, 0
		if cap < d {
			d, m = cap, 1
		}
		if cross < d {
			d, m = cross, 0
		}
		if g := gemsAt(q, false); g < d {
			d, m = g, 2
		}
		if g := gemsAt(q, true); g < d {
			d, m = g, 3
		}
		return d, m
	}
	return &Scene{
		Name: "freenet_crown",
		SDF:  sdf,
		Mats: []Mat{
			{Kind: metal, Color: gold, Rough: 0.07},
			{Kind: velvet, Color: V{0.55, 0.02, 0.05}},
			{Kind: gem, Color: V{0.6, 9, 7}, IOR: 1.76, Disp: 0.01, Body: ruby},
			{Kind: gem, Color: V{9, 4, 0.5}, IOR: 1.76, Disp: 0.01, Body: sapphire},
		},
		Cam: V{0, 0, 4.2}, FOV: 36, Light: 1.0, Bounds: 1.35,
		Inner: 0xc8405f, Outer: 0x3a0716, Pattern: 0xffaabb,
	}
}

// heartF is Taubin's heart, y up, about 1.2 across.
func heartF(p V) float64 {
	// a³ = y³(x² + 0.1125 z²), taken by its cube root: the same surface,
	// but its gradient does not vanish where y = 0.
	x, y, z := p.X, p.Y, p.Z
	return x*x + 2.25*z*z + y*y - 1 - y*math.Cbrt(x*x+0.1125*z*z)
}

func heartBody(p V) float64 {
	q := rotY(rotZ(p, 0.12), 0.45).Mul(1 / 0.82)
	q = V{q.X, q.Y + 0.08, q.Z * 1.25}
	if l := q.Len(); l > 1.6 {
		return (l - 1.5) * 0.82
	}
	const e = 1e-3
	f := heartF(q)
	g := V{
		heartF(q.Add(V{e, 0, 0})) - f,
		heartF(q.Add(V{0, e, 0})) - f,
		heartF(q.Add(V{0, 0, e})) - f,
	}.Mul(1 / e)
	return f / math.Max(g.Len(), 1e-3) * 0.7 * 0.82
}

func heart() *Scene {
	sdf := func(p V) (float64, int) {
		body := heartBody(p)
		q := rotY(rotZ(p, 0.12), 0.45)
		c := q.Sub(V{0, 0.78, 0})
		bail := math.Hypot(math.Hypot(c.X, c.Y)-0.12, c.Z) - 0.045
		cap := sdSphere(q.Sub(V{0, 0.62, 0}), 0.07)
		if g := math.Min(bail, cap); g < body {
			return g, 1
		}
		return body, 0
	}
	return &Scene{
		Name: "freenet_heart",
		SDF:  sdf,
		Mats: []Mat{
			{Kind: gem, Color: V{0.45, 6, 5}, IOR: 1.76, Disp: 0.012, Body: heartBody},
			{Kind: metal, Color: gold, Rough: 0.07},
		},
		Cam: V{0, 0, 4.2}, FOV: 36, Light: 1.0, Bounds: 1.35,
		Inner: 0x2aa0ad, Outer: 0x062a33, Pattern: 0x95edf2,
	}
}

func cup() *Scene {
	local := func(p V) V { return rotY(rotX(p, 0.22), 0.3).Mul(1 / 0.92).Add(V{0, 0.02, 0}) }
	outer := func(q V) float64 {
		r := math.Hypot(q.X, q.Z)
		low := math.Max((math.Hypot(r/0.6, (q.Y-0.3)/0.62)-1)*0.6, q.Y-0.3)
		wall := math.Max((r-(0.6+(q.Y-0.3)*0.18))*0.98, math.Abs(q.Y-0.58)-0.28)
		return math.Min(low, wall)
	}
	sdf := func(p V) (float64, int) {
		q := local(p)
		r := math.Hypot(q.X, q.Z)
		o := outer(q)
		bowl := math.Max(math.Abs(o+0.025)-0.025, q.Y-0.86)
		rim := math.Hypot(r-(0.6+0.56*0.18)+0.02, q.Y-0.86) - 0.035
		stem := math.Max(r-0.075, math.Abs(q.Y+0.42)-0.2)
		knot := math.Hypot(r/1.3, q.Y+0.4) - 0.1
		base := sdRoundCyl(r, q.Y+0.66, 0.4, 0.06, 0.03)
		step := sdRoundCyl(r, q.Y+0.57, 0.27, 0.05, 0.025)
		ax := math.Abs(q.X)
		hc := V{ax - 0.68, q.Y - 0.42, q.Z}
		handle := math.Max(math.Hypot(math.Hypot(hc.X, hc.Y)-0.22, q.Z)-0.045, 0.62-ax)
		body := smin(bowl, rim, 0.02)
		body = math.Min(body, smin(smin(stem, knot, 0.06), smin(base, step, 0.04), 0.05))
		body = smin(body, handle, 0.03)
		g := fN(q.X/0.26, (q.Y-0.4)/0.26) * 0.26
		emblem := math.Max(math.Max(g+0.004, math.Abs(o-0.014)-0.01)-0.004, -q.Z)
		if emblem < body {
			return emblem, 1
		}
		return body, 0
	}
	return &Scene{
		Name: "freenet_cup",
		SDF:  sdf,
		Mats: []Mat{
			{Kind: metal, Color: gold, Rough: 0.07},
			{Kind: gloss, Color: hex(0x2f7cf0), Rough: 0.04},
		},
		Cam: V{0, 0, 4.2}, FOV: 36, Light: 1.0, Bounds: 1.35,
		Inner: 0x5d6f88, Outer: 0x10161e, Pattern: 0xbfcde0,
	}
}

var scenes = map[string]func() *Scene{
	"crystal": crystal,
	"coin":    coin,
	"ring":    ring,
	"crown":   crown,
	"heart":   heart,
	"cup":     cup,

	"prime_star":    primeStar,
	"prime_diamond": primeDiamond,
	"prime_crown":   primeCrown,
	"prime_rocket":  primeRocket,
	"prime_trophy":  primeTrophy,
}

func main() {
	size := flag.Int("size", 192, "picture size")
	ss := flag.Int("ss", 4, "samples per side of a pixel")
	only := flag.String("only", "", "comma separated scenes")
	out := flag.String("out", "out", "directory")
	withModels := flag.Bool("models", false, "draw the models of collectible gifts into <out>/models")
	flag.Parse()

	if *withModels {
		if err := os.MkdirAll(filepath.Join(*out, "models"), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	for name, build := range scenes {
		if *only != "" && !strings.Contains(","+*only+",", ","+name+",") {
			continue
		}
		if !*withModels {
			draw(build(), *size, *ss, filepath.Join(*out, build().Name+".png"))
			continue
		}
		for _, m := range models {
			s := m.recast(build())
			draw(s, *size, *ss, filepath.Join(*out, "models", s.Name+".png"))
		}
	}
}

func draw(s *Scene, size, ss int, path string) {
	if err := Render(s, size, ss, path); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(path)
}
