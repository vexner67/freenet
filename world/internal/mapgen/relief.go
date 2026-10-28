package mapgen

import (
	"math"
	"math/rand/v2"
	"slices"
)

type plate struct {
	x, y        float64
	vx, vy      float64
	continental bool
}

// tectonics splits the grid into plates and gives the plate of every cell.
func tectonics(p Params, g grid) ([]plate, []int8) {
	r := stream(p.Seed, streamPlates)
	count := p.Plates
	if count == 0 {
		count = 12 + r.IntN(7)
	}

	plates := make([]plate, 0, count)
	minDist := 0.5 * math.Sqrt(float64(g.n())/float64(count))
	for attempt := 0; len(plates) < count; attempt++ {
		x, y := r.Float64()*float64(g.w), r.Float64()*float64(g.h)
		if attempt < 10000 && slices.ContainsFunc(plates, func(q plate) bool { return math.Hypot(q.x-x, q.y-y) < minDist }) {
			continue
		}
		a := r.Float64() * 2 * math.Pi
		speed := 0.4 + 0.6*r.Float64()
		plates = append(plates, plate{x: x, y: y, vx: math.Cos(a) * speed, vy: math.Sin(a) * speed})
	}

	// The borders of plates wander: the distances are measured from
	// warped points, so the plates are not straight-edged cells.
	wx, wy := newNoise(r), newNoise(r)
	freq, amp := 5/float64(g.w), 0.05*float64(g.w)
	owner := make([]int8, g.n())
	parallel(g.h, func(lo, hi int) {
		for y := lo; y < hi; y++ {
			for x := range g.w {
				fx, fy := float64(x)*freq, float64(y)*freq
				px := float64(x) + amp*wx.fbm(fx, fy, 6)
				py := float64(y) + amp*wy.fbm(fx, fy, 6)
				best, bestD := 0, math.Inf(1)
				for k, q := range plates {
					if d := (q.x-px)*(q.x-px) + (q.y-py)*(q.y-py); d < bestD {
						best, bestD = k, d
					}
				}
				owner[y*g.w+x] = int8(best)
			}
		}
	})

	chooseContinents(p, r, g, plates, owner)

	return plates, owner
}

// chooseContinents makes plates continental until they cover a bit more
// than the land should, preferring the plates far from the edges of the
// map, which the sea takes anyway.
func chooseContinents(p Params, r *rand.Rand, g grid, plates []plate, owner []int8) {
	area := make([]int, len(plates))
	for _, o := range owner {
		area[o]++
	}

	order := make([]int, len(plates))
	score := make([]float64, len(plates))
	for k, q := range plates {
		order[k] = k
		cx := 1 - math.Abs(q.x/float64(g.w)-0.5)*2
		cy := 1 - math.Abs(q.y/float64(g.h)-0.5)*2
		score[k] = r.Float64() + 0.8*math.Min(cx, cy)
	}
	slices.SortStableFunc(order, func(a, b int) int {
		switch {
		case score[a] > score[b]:
			return -1
		case score[a] < score[b]:
			return 1
		}
		return 0
	})

	touch := touching(g, len(plates), owner)
	covered, target := 0, int(float64(g.n())*p.LandShare*1.15)
	chosen := make([]bool, len(plates))
	// Mostly apart, so the land makes several continents; now and then
	// touching, where their collision raises the great ranges.
	for pass := range 2 {
		for _, k := range order {
			if covered >= target {
				break
			}
			if chosen[k] {
				continue
			}
			near := slices.ContainsFunc(touch[k], func(o int) bool { return chosen[o] })
			if pass == 0 && near && r.Float64() > 0.3 {
				continue
			}
			chosen[k] = true
			plates[k].continental = true
			covered += area[k]
		}
	}
}

// touching gives the plates each plate borders.
func touching(g grid, count int, owner []int8) [][]int {
	seen := make([][]bool, count)
	for k := range seen {
		seen[k] = make([]bool, count)
	}
	for i, o := range owner {
		for _, k := range [2]int{0, 2} {
			if j := g.neighbour(i, k); j >= 0 && owner[j] != o {
				seen[o][owner[j]], seen[owner[j]][o] = true, true
			}
		}
	}

	out := make([][]int, count)
	for a := range seen {
		for b, s := range seen[a] {
			if s {
				out[a] = append(out[a], b)
			}
		}
	}

	return out
}

// Kinds of the meeting of plates and how wide their relief is, as a part
// of the width of the map.
const (
	widthCollision = 0.030
	widthCoastal   = 0.020
	widthArc       = 0.010
	widthRift      = 0.018
)

// boundaries gives the uplift of the meeting of plates in the cells along
// it: collisions raise ranges, a continent over an ocean a coastal range
// and an ocean over an ocean arcs of islands; parting plates sink a rift.
func boundaries(g grid, plates []plate, owner []int8) (uplift, width []float32, source []bool) {
	uplift = make([]float32, g.n())
	width = make([]float32, g.n())
	source = make([]bool, g.n())

	mark := func(i int, u, w float64) {
		if !source[i] || math.Abs(u) > math.Abs(float64(uplift[i])) {
			uplift[i], width[i], source[i] = float32(u), float32(w*float64(g.w)), true
		}
	}

	for i := range g.n() {
		// Right and down: each pair of cells once.
		for _, k := range [2]int{0, 2} {
			j := g.neighbour(i, k)
			if j < 0 || owner[j] == owner[i] {
				continue
			}
			a, b := plates[owner[i]], plates[owner[j]]
			nx, ny := b.x-a.x, b.y-a.y
			l := math.Hypot(nx, ny)
			nx, ny = nx/l, ny/l
			// Positive when the plates come together.
			c := (a.vx-b.vx)*nx + (a.vy-b.vy)*ny

			ua, ub, w := meeting(a.continental, b.continental, c)
			mark(i, ua, w)
			mark(j, ub, w)
		}
	}

	return uplift, width, source
}

func meeting(ac, bc bool, c float64) (ua, ub, width float64) {
	switch {
	case c < 0 && ac && bc:
		return 0.18 * c, 0.18 * c, widthRift
	case c < 0:
		u := func(cont bool) float64 {
			if cont {
				return 0.18 * c
			}
			return -0.08 * c
		}
		return u(ac), u(bc), widthRift
	case ac && bc:
		return c, c, widthCollision
	case ac:
		return 0.55 * c, -0.25 * c, widthCoastal
	case bc:
		return -0.25 * c, 0.55 * c, widthCoastal
	}

	return 0.32 * c, 0.32 * c, widthArc
}

// elevation gives the relief before erosion: the plates, smoothed, their
// meetings and warped fractal noise, with the sea around the edges, as a
// field with the sea level at 0, land up to 1 and the sea down to -1.
func elevation(p Params, g grid) []float32 {
	plates, owner := tectonics(p, g)
	r := stream(p.Seed, streamRelief)

	base := make([]float32, g.n())
	jitter := make([]float64, len(plates))
	for k := range plates {
		jitter[k] = 0.08 * (r.Float64() - 0.5)
	}
	for i, o := range owner {
		v := -0.45 + jitter[o]
		if plates[o].continental {
			v = 0.30 + jitter[o]
		}
		base[i] = float32(v)
	}
	base = blur(g, base, max(1, g.w/40), 3)

	uplift, width, source := boundaries(g, plates, owner)
	src, dist := nearest(g, source)

	n1, n2, n3, n4, ridges := newNoise(r), newNoise(r), newNoise(r), newNoise(r), newNoise(r)
	freq := 6 / float64(g.w)
	e := make([]float32, g.n())
	parallel(g.h, func(lo, hi int) {
		for y := lo; y < hi; y++ {
			for x := range g.w {
				i := y*g.w + x
				fx, fy := float64(x)*freq, float64(y)*freq
				// Domain warping: the noise is read at a point moved by
				// noise, which bends it into the shapes of real coasts.
				qx, qy := n2.fbm(fx, fy, 4), n3.fbm(fx+5.2, fy+1.3, 4)
				shape := n1.fbm(fx*0.5+qx, fy*0.5+qy, 4)
				detail := n4.fbm(fx*2+0.3*qx, fy*2+0.3*qy, 8)

				v := 0.65*float64(base[i]) + 0.3*shape
				u := 0.0
				if s := src[i]; s >= 0 {
					t := float64(dist[i] / width[s])
					u = float64(uplift[s]) * math.Exp(-t*t)
					if u > 0 {
						u *= 0.3 + 0.9*ridges.ridged(fx*3, fy*3, 5)
					}
				}
				// Mountains are rough, plains smooth: the fine relief grows
				// with the uplift.
				v += u + (0.05+0.4*math.Max(0, u)+0.1*smoothstep(0, 0.4, v))*detail
				v -= edgeSea(float64(x)/float64(g.w), float64(y)/float64(g.h))
				e[i] = float32(v)
			}
		}
	})

	h := toSeaLevel(e, p.LandShare)
	inlandSeas(g, h)

	return h
}

// inlandSeas raises the small pieces of sea closed in the land to low
// land: basins the rivers and lakes fill, not salt water.
func inlandSeas(g grid, h []float32) {
	limit := g.n() / 400
	seen := make([]bool, g.n())
	for i := range h {
		if h[i] > 0 || seen[i] {
			continue
		}
		part := []int{i}
		seen[i] = true
		for k := 0; k < len(part); k++ {
			for n := range 4 {
				j := g.neighbour(part[k], n)
				if j >= 0 && !seen[j] && h[j] <= 0 {
					seen[j] = true
					part = append(part, j)
				}
			}
		}
		if len(part) >= limit {
			continue
		}
		for _, c := range part {
			h[c] = float32(math.Max(0.0005, 0.004+0.01*float64(h[c])))
		}
	}
}

// edgeSea lowers the edges of the map, so that the world ends in sea.
func edgeSea(u, v float64) float64 {
	ex := math.Min(u, 1-u)
	ey := math.Min(v, 1-v)
	s := 0.0
	if ex < 0.1 {
		t := 1 - ex/0.1
		s += 0.8 * t * t
	}
	if ey < 0.12 {
		t := 1 - ey/0.12
		s += 0.8 * t * t
	}

	return s
}

// toSeaLevel puts the sea level where the land is the share it should be,
// and stretches the land to (0, 1] with most of it low, as on the Earth,
// and the sea to [-1, 0).
func toSeaLevel(e []float32, landShare float64) []float32 {
	sea := quantile(e, 1-landShare)
	top, bottom := slices.Max(e), slices.Min(e)
	out := make([]float32, len(e))
	for i, v := range e {
		if v > sea {
			t := float64(v-sea) / float64(top-sea)
			out[i] = float32(math.Max(1e-4, math.Pow(t, 1.5)))
			continue
		}
		t := float64(sea-v) / float64(sea-bottom)
		out[i] = -float32(math.Max(1e-4, math.Pow(t, 0.7)))
	}

	return out
}
