package main

import "math"

// The gifts of Freenet Prime: gold and gems on violet, the colours of the
// subscription.

var platinum = V{0.86, 0.87, 0.92}

// star5 is the signed distance to a five-pointed star in the plane, a
// point up: r is the radius of its points, rf how deep its notches are.
func star5(x, y, r, rf float64) float64 {
	const k1x, k1y = 0.809016994375, -0.587785252292
	x = math.Abs(x)
	if d := k1x*x + k1y*y; d > 0 {
		x, y = x-2*d*k1x, y-2*d*k1y
	}
	if d := -k1x*x + k1y*y; d > 0 {
		x, y = x+2*d*k1x, y-2*d*k1y
	}
	x = math.Abs(x)
	y -= r
	bx, by := rf*-k1y, rf*k1x-1
	h := clamp((x*bx+y*by)/(bx*bx+by*by), 0, r)
	dx, dy := x-bx*h, y-by*h
	d := math.Hypot(dx, dy)
	if y*bx-x*by < 0 {
		return -d
	}
	return d
}

// facetedStar is a star cut like a gem: each arm ridged along its middle,
// rising from a thin rim; z is its thickness.
func facetedStar(p V, r, rf, rise, rim float64) float64 {
	d := star5(p.X, p.Y, r, rf)
	return math.Max(d, math.Abs(p.Z)+rise*d-rim) / math.Sqrt(1+rise*rise)
}

func primeStar() *Scene {
	sdf := func(p V) (float64, int) {
		q := rotX(rotY(p, 0.42), -0.18)
		star := facetedStar(q, 1.0, 0.45, 0.62, 0.05)
		gem := sdSphere(q.Sub(V{0, 0, 0.26}), 0.13)
		if gem < star {
			return gem, 1
		}
		return star, 0
	}
	return &Scene{
		Name: "prime_star",
		SDF:  sdf,
		Mats: []Mat{
			{Kind: metal, Color: gold, Rough: 0.05},
			{Kind: gem, Color: V{0.03, 0.03, 0.02}, IOR: 2.42, Disp: 0.05, Body: func(p V) float64 {
				return sdSphere(rotX(rotY(p, 0.42), -0.18).Sub(V{0, 0, 0.26}), 0.13)
			}},
		},
		Cam: V{0, 0, 4.2}, FOV: 36, Light: 1.0, Bounds: 1.3,
		Inner: 0x6a44c8, Outer: 0x170a3a, Pattern: 0xc6b0ff,
	}
}

func primeDiamond() *Scene {
	body := func(p V) float64 {
		q := rotY(rotX(p, 0.5), -0.35)
		const k = 1.05
		return brilliant(q.Mul(1/k).Add(V{0, -0.2, 0})) * k
	}
	return &Scene{
		Name: "prime_diamond",
		SDF:  func(p V) (float64, int) { return body(p), 0 },
		Mats: []Mat{{Kind: gem, Color: V{0.04, 0.035, 0.02}, IOR: 2.42, Disp: 0.06, Body: body}},
		Cam:  V{0, 0, 4.2}, FOV: 36, Light: 1.05, Bounds: 1.45,
		Inner: 0x8a52d8, Outer: 0x200b44, Pattern: 0xe0c8ff,
	}
}

func primeCrown() *Scene {
	s := crown()
	s.Name = "prime_crown"
	s.Mats = []Mat{
		{Kind: metal, Color: platinum, Rough: 0.06},
		{Kind: velvet, Color: V{0.22, 0.04, 0.42}},
		{Kind: gem, Color: V{2.2, 7, 1.2}, IOR: 1.55, Disp: 0.01, Body: s.Mats[2].Body},
		{Kind: gem, Color: V{0.04, 0.04, 0.03}, IOR: 2.42, Disp: 0.05, Body: s.Mats[3].Body},
	}
	s.Inner, s.Outer, s.Pattern = 0x5a3fb0, 0x120830, 0xb9a6f5
	return s
}

func primeRocket() *Scene {
	local := func(p V) V { return rotY(rotZ(p.Add(V{-0.06, 0.04, 0}), 0.7), -0.35).Mul(1 / 1.05).Add(V{0, 0.08, 0}) }
	window := func(q V) float64 {
		return math.Max(math.Hypot(q.X, q.Y-0.18)-0.13, math.Abs(q.Z-0.29)-0.05)
	}
	sdf := func(p V) (float64, int) {
		q := local(p)
		r := math.Hypot(q.X, q.Z)
		// the hull: a cylinder with an ogive nose, the profile revolved
		const R, top, bottom = 0.3, 0.25, -0.62
		profile := R
		if q.Y > top {
			t := clamp((q.Y-top)/0.72, 0, 1)
			profile = R * math.Sqrt(1-t*t)
		}
		hull := math.Max((r-profile)*0.75, math.Max(bottom-q.Y, q.Y-(top+0.72)))
		band := math.Max(math.Abs(r-R)-0.012, math.Abs(q.Y-top+0.02)-0.025)
		nozzle := math.Max(r-(0.2-0.12*(q.Y-bottom)), math.Abs(q.Y-(bottom-0.08))-0.08)
		// three fins, each a plate swept back from the hull
		a := math.Atan2(q.Z, q.X) + math.Pi/2
		sector := 2 * math.Pi / 3
		fa := math.Mod(a+10*math.Pi+sector/2, sector) - sector/2
		u, v := r*math.Cos(fa), r*math.Sin(fa)
		fin := math.Max(math.Abs(v)-0.03, math.Max(math.Max(R-0.05-u, u-0.6), math.Max(-0.78-q.Y, (q.Y+0.12)+(u-R)*0.9)))
		frame := math.Max(math.Abs(math.Hypot(q.X, q.Y-0.18)-0.15)-0.03, math.Abs(q.Z-0.28)-0.04)
		body := smin(hull, band, 0.01)
		d, m := body, 0
		for _, part := range []float64{nozzle, fin, frame} {
			if part < d {
				d, m = part, 1
			}
		}
		if g := window(q); g < d {
			d, m = g, 2
		}
		return d, m
	}
	return &Scene{
		Name: "prime_rocket",
		SDF:  sdf,
		Mats: []Mat{
			{Kind: gloss, Color: hex(0x6a3cc8), Rough: 0.03},
			{Kind: metal, Color: gold, Rough: 0.07},
			{Kind: gloss, Color: hex(0x3fa8ff), Rough: 0.02},
		},
		Cam: V{0, 0, 4.2}, FOV: 36, Light: 1.0, Bounds: 1.5,
		Inner: 0x4a5ad0, Outer: 0x0c0f36, Pattern: 0xaab8ff,
	}
}

// sdBox is the signed distance to a box with rounded edges.
func sdBox(p, half V, round float64) float64 {
	q := V{math.Abs(p.X) - half.X + round, math.Abs(p.Y) - half.Y + round, math.Abs(p.Z) - half.Z + round}
	out := V{math.Max(q.X, 0), math.Max(q.Y, 0), math.Max(q.Z, 0)}
	return out.Len() + math.Min(math.Max(q.X, math.Max(q.Y, q.Z)), 0) - round
}

func primeTrophy() *Scene {
	local := func(p V) V { return rotY(rotX(p, 0.24), 0.2).Mul(1 / 0.9).Add(V{0, 0.08, 0}) }
	sdf := func(p V) (float64, int) {
		q := local(p)
		r := math.Hypot(q.X, q.Z)
		// the bowl: a tulip, round below and flaring to the rim
		low := math.Max((math.Hypot(r/0.5, (q.Y-0.4)/0.45)-1)*0.45, q.Y-0.4)
		wall := math.Max((r-(0.5+(q.Y-0.4)*0.35))*0.94, math.Abs(q.Y-0.65)-0.25)
		outer := math.Min(low, wall)
		bowl := math.Max(math.Abs(outer+0.022)-0.022, q.Y-0.9)
		rim := math.Hypot(r-(0.5+0.5*0.35)+0.02, q.Y-0.9) - 0.03
		stem := math.Max(r-(0.06+0.05*smoothstep(-0.05, -0.35, q.Y)), math.Abs(q.Y+0.18)-0.2)
		knot := math.Hypot(r/1.4, q.Y+0.12) - 0.08
		ax := math.Abs(q.X)
		hc := V{ax - 0.6, q.Y - 0.55, q.Z}
		handle := math.Max(math.Hypot(math.Hypot(hc.X, hc.Y*0.8)-0.2, q.Z)-0.04, 0.55-ax)
		cup := smin(bowl, rim, 0.02)
		cup = smin(cup, smin(stem, knot, 0.05), 0.04)
		cup = smin(cup, handle, 0.03)
		plinth := math.Min(sdBox(q.Sub(V{0, -0.47, 0}), V{0.36, 0.09, 0.36}, 0.02), sdBox(q.Sub(V{0, -0.66, 0}), V{0.46, 0.11, 0.46}, 0.025))
		plate := sdBox(q.Sub(V{0, -0.66, 0.46}), V{0.26, 0.07, 0.012}, 0.008)
		star := facetedStar(V{q.X, q.Y - 0.62, q.Z - 0.58}.Mul(1/0.2), 1, 0.45, 0.6, 0.06) * 0.2
		d, m := cup, 0
		if plinth < d {
			d, m = plinth, 1
		}
		if plate < d {
			d, m = plate, 0
		}
		if star < d {
			d, m = star, 2
		}
		return d, m
	}
	return &Scene{
		Name: "prime_trophy",
		SDF:  sdf,
		Mats: []Mat{
			{Kind: metal, Color: gold, Rough: 0.06},
			{Kind: gloss, Color: V{0.006, 0.004, 0.012}, Rough: 0.02},
			{Kind: gloss, Color: hex(0x6a3cc8), Rough: 0.03},
		},
		Cam: V{0, 0, 4.2}, FOV: 36, Light: 1.0, Bounds: 1.35,
		Inner: 0x7a48b8, Outer: 0x1a0832, Pattern: 0xd6b8ff,
	}
}
