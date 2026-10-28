package mapgen

import (
	"math"
	"math/rand/v2"
	"slices"
)

// Resource is a good a province gives. The first four are the raw goods
// of the business, which an enterprise of the same good in the province
// makes more of.
type Resource string

const (
	ResourceGrain  Resource = "grain"
	ResourceWood   Resource = "wood"
	ResourceOre    Resource = "ore"
	ResourceOil    Resource = "oil"
	ResourceGold   Resource = "gold"
	ResourceHorses Resource = "horses"
)

// describe gives the provinces their resources, people and cultures and
// picks the free cities.
func describe(p Params, g grid, m *Map) {
	r := stream(p.Seed, streamResources)
	rivers := riverProvinces(m)
	for k := range m.Provinces {
		pr := &m.Provinces[k]
		if pr.Sea {
			continue
		}
		pr.Resources = resources(r, pr)
		pr.Population = population(r, pr, rivers[k])
	}

	cultures(r, m)
	freeCities(r, g, m)
}

func riverProvinces(m *Map) []bool {
	has := make([]bool, len(m.Provinces))
	for i, c := range m.River {
		if c >= 2 {
			has[m.Province[i]] = true
		}
	}

	return has
}

// resources draws the 1–2 goods of a province by its terrain and cover.
func resources(r *rand.Rand, pr *Province) []Resource {
	var odds []struct {
		res    Resource
		chance float64
	}
	add := func(res Resource, chance float64) {
		odds = append(odds, struct {
			res    Resource
			chance float64
		}{res, chance})
	}

	switch pr.Terrain {
	case TerrainPlains:
		add(ResourceGrain, 0.85)
	case TerrainForest:
		add(ResourceWood, 0.85)
		add(ResourceGrain, 0.15)
	case TerrainHills:
		add(ResourceOre, 0.5)
		add(ResourceGrain, 0.3)
		add(ResourceWood, 0.2)
	case TerrainMountains:
		add(ResourceOre, 0.8)
		add(ResourceGold, 0.06)
	case TerrainDesert:
		add(ResourceOil, 0.15)
	case TerrainSwamp:
		add(ResourceOil, 0.2)
		add(ResourceWood, 0.4)
	case TerrainTundra:
		add(ResourceWood, 0.2)
		add(ResourceOre, 0.2)
	}
	if pr.Biome == BiomeSteppe {
		add(ResourceHorses, 0.35)
	}
	if pr.Coastal {
		add(ResourceOil, 0.05)
	}

	var got []Resource
	for _, o := range odds {
		if len(got) < 2 && !slices.Contains(got, o.res) && r.Float64() < o.chance {
			got = append(got, o.res)
		}
	}
	if len(got) == 0 {
		got = append(got, primary(pr.Terrain))
	}

	return got
}

func primary(t Terrain) Resource {
	switch t {
	case TerrainForest, TerrainSwamp, TerrainTundra:
		return ResourceWood
	case TerrainHills, TerrainMountains:
		return ResourceOre
	case TerrainDesert:
		return ResourceOil
	}

	return ResourceGrain
}

// population gives the thousands living in a province at the start.
func population(r *rand.Rand, pr *Province, river bool) int {
	lo, hi := 50, 90
	switch pr.Terrain {
	case TerrainPlains:
		if river {
			lo, hi = 80, 150
		}
	case TerrainHills:
		lo, hi = 30, 60
	case TerrainForest:
		lo, hi = 25, 50
	case TerrainMountains:
		lo, hi = 10, 30
	case TerrainDesert:
		lo, hi = 5, 15
	case TerrainSwamp:
		lo, hi = 10, 25
	case TerrainTundra:
		lo, hi = 3, 10
	}
	n := lo + r.IntN(hi-lo+1)
	if pr.Coastal {
		n += n / 10
	}

	return n
}

// cultures groups the land provinces into 4–6 regions by k-means of their
// centres; a culture names its places in its own syllables.
func cultures(r *rand.Rand, m *Map) {
	var land []int
	for k, pr := range m.Provinces {
		if !pr.Sea {
			land = append(land, k)
		}
	}
	if len(land) == 0 {
		return
	}
	count := min(4+r.IntN(3), len(land))

	pos := func(k int) (float64, float64) {
		return float64(m.Provinces[k].CenterX), float64(m.Provinces[k].CenterY)
	}
	// The first centre at random, each next the province farthest from
	// those chosen: well spread starts.
	cx, cy := make([]float64, 0, count), make([]float64, 0, count)
	x, y := pos(land[r.IntN(len(land))])
	cx, cy = append(cx, x), append(cy, y)
	for len(cx) < count {
		far, farD := land[0], -1.0
		for _, k := range land {
			x, y := pos(k)
			d := math.Inf(1)
			for c := range cx {
				d = math.Min(d, math.Hypot(x-cx[c], y-cy[c]))
			}
			if d > farD {
				far, farD = k, d
			}
		}
		x, y := pos(far)
		cx, cy = append(cx, x), append(cy, y)
	}

	for range 12 {
		sx, sy, n := make([]float64, count), make([]float64, count), make([]int, count)
		for _, k := range land {
			x, y := pos(k)
			c := closest(cx, cy, x, y)
			m.Provinces[k].Culture = c
			sx[c] += x
			sy[c] += y
			n[c]++
		}
		for c := range count {
			if n[c] > 0 {
				cx[c], cy[c] = sx[c]/float64(n[c]), sy[c]/float64(n[c])
			}
		}
	}
	for k := range m.Provinces {
		if m.Provinces[k].Sea {
			x, y := pos(k)
			m.Provinces[k].Culture = closest(cx, cy, x, y)
		}
	}
}

func closest(cx, cy []float64, x, y float64) int {
	best, bestD := 0, math.Inf(1)
	for c := range cx {
		if d := math.Hypot(x-cx[c], y-cy[c]); d < bestD {
			best, bestD = c, d
		}
	}

	return best
}

// freeCities picks 5–10 of the most peopled land provinces, far from each
// other, as the free cities nobody owns at the start.
func freeCities(r *rand.Rand, g grid, m *Map) {
	var land []int
	for k, pr := range m.Provinces {
		if !pr.Sea {
			land = append(land, k)
		}
	}
	slices.SortStableFunc(land, func(a, b int) int { return m.Provinces[b].Population - m.Provinces[a].Population })

	want := 5 + r.IntN(6)
	spacing := 0.12 * float64(g.w)
	var chosen []int
	for _, k := range land {
		if len(chosen) == want {
			break
		}
		a := m.Provinces[k]
		if slices.ContainsFunc(chosen, func(c int) bool {
			b := m.Provinces[c]
			return math.Hypot(float64(a.CenterX-b.CenterX), float64(a.CenterY-b.CenterY)) < spacing
		}) {
			continue
		}
		chosen = append(chosen, k)
		m.Provinces[k].FreeCity = true
	}
}
