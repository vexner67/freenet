package main

import "math"

// The models of collectible gifts (user/internal/user/collectible.go): each
// gift drawn again in other metals and stones, alone on nothing, for the
// site to lay on the backdrop the gift rolled.

// model recasts a gift: its metals in metal and, unless hue is below zero,
// its stones, enamel and velvet turned to that hue, in degrees.
type model struct {
	name  string
	metal V
	hue   float64
}

var models = []model{
	{name: "classic", hue: -1},
	{name: "silver", metal: V{0.95, 0.93, 0.88}, hue: -1},
	{name: "rose", metal: V{0.98, 0.64, 0.55}, hue: 335},
	{name: "onyx", metal: V{0.2, 0.2, 0.23}, hue: 210},
	{name: "emerald", metal: V{0.42, 0.88, 0.5}, hue: 145},
	{name: "amethyst", metal: V{0.74, 0.5, 1.0}, hue: 275},
}

// recast is s as m: classic is the gift as it is.
func (m model) recast(s *Scene) *Scene {
	s.Name += "_" + m.name
	s.Bare = true
	if m.name == "classic" {
		return s
	}
	mats := make([]Mat, len(s.Mats))
	for i, mat := range s.Mats {
		switch {
		case mat.Kind == metal:
			mat.Color = m.metal
		case m.hue < 0:
		case mat.Kind == gem:
			// the colour of a stone is what it lets through
			seen := V{math.Exp(-mat.Color.X), math.Exp(-mat.Color.Y), math.Exp(-mat.Color.Z)}
			seen = withHue(seen, m.hue)
			absorb := func(x float64) float64 { return -math.Log(clamp(x, 1e-3, 1)) }
			mat.Color = V{absorb(seen.X), absorb(seen.Y), absorb(seen.Z)}
		default:
			mat.Color = withHue(mat.Color, m.hue)
		}
		mats[i] = mat
	}
	s.Mats = mats
	return s
}

// withHue is c turned to hue, its saturation and value kept; a grey stays
// grey.
func withHue(c V, hue float64) V {
	hi := math.Max(c.X, math.Max(c.Y, c.Z))
	lo := math.Min(c.X, math.Min(c.Y, c.Z))
	if hi <= 0 || hi-lo < 1e-6 {
		return c
	}
	sat := (hi - lo) / hi
	f := func(n float64) float64 {
		k := math.Mod(n+hue/60, 6)
		return hi - hi*sat*math.Max(0, math.Min(k, math.Min(4-k, 1)))
	}
	return V{f(5), f(3), f(1)}
}
