package mapgen

import (
	"math"
)

// Biome is the living cover of a cell, by the Whittaker diagram.
type Biome uint8

const (
	BiomeSea Biome = iota
	BiomeLake
	BiomeGlacier
	BiomeTundra
	BiomeTaiga
	BiomeMixedForest
	BiomeSteppe
	BiomeMeadow
	BiomeDesert
	BiomeSavanna
	BiomeJungle
	BiomeSwamp
	biomeCount
)

var biomeNames = [biomeCount]string{"sea", "lake", "glacier", "tundra", "taiga", "mixed_forest", "steppe", "meadow", "desert", "savanna", "jungle", "swamp"}

func (b Biome) String() string { return biomeNames[b] }

// isForest says whether the biome is woodland.
func (b Biome) isForest() bool {
	return b == BiomeTaiga || b == BiomeMixedForest || b == BiomeJungle
}

// The map spans these latitudes: no poles.
const maxLatitude = 72.0

func latitude(g grid, y int) float64 {
	return ((float64(y)+0.5)/float64(g.h)*2 - 1) * maxLatitude
}

// climateLatitude is the latitude as the climate feels it: the belts of
// warmth and rain wave by up to 10°, as the real ones do, instead of
// running along the parallels as straight bands.
func climateLatitude(p Params, g grid) []float32 {
	wave := newNoise(stream(p.Seed, streamClimate+100))
	freq := 4 / float64(g.w)
	lat := make([]float32, g.n())
	parallel(g.h, func(lo, hi int) {
		for y := lo; y < hi; y++ {
			for x := range g.w {
				lat[y*g.w+x] = float32(latitude(g, y) + 10*wave.fbm(float64(x)*freq, float64(y)*freq, 3))
			}
		}
	})

	return lat
}

// temperature is the mean of a year in °C: warm at the equator, cold at
// the edges, 6.5° colder for every kilometre up.
func temperature(p Params, g grid, metres, lats []float32) []float32 {
	n := newNoise(stream(p.Seed, streamClimate))
	freq := 8 / float64(g.w)
	t := make([]float32, g.n())
	parallel(g.h, func(lo, hi int) {
		for y := lo; y < hi; y++ {
			for x := range g.w {
				i := y*g.w + x
				lat := float64(lats[i])
				v := 29 - 0.0072*lat*lat + 2*n.fbm(float64(x)*freq, float64(y)*freq, 3)
				if metres[i] > 0 {
					v -= 6.5 * float64(metres[i]) / 1000
				}
				t[i] = float32(v)
			}
		}
	})

	return t
}

// moisture carries the wet air of the seas with the prevailing winds of
// the belts of latitude: the trade winds to the west near the equator, the
// westerlies to the east in the middle latitudes, the polar easterlies to
// the west beyond. The air rains out over land, most of all where it has to
// climb, so behind mountains lies a rain shadow. The belts of rising and
// sinking air make the equator wet and the tropics dry. It gives 0 to 1.
func moisture(p Params, g grid, metres, temp, lats []float32) []float32 {
	east, west := sweep(p, g, metres, temp, true), sweep(p, g, metres, temp, false)
	// The belts of winds meet gradually: near 30° and 60° the rain is a
	// mix of both winds, so their borders leave no straight edge.
	rain := make([]float32, g.n())
	for i := range rain {
		a := math.Abs(float64(lats[i]))
		w := smoothstep(24, 36, a) * (1 - smoothstep(54, 66, a))
		rain[i] = float32(w*float64(east[i]) + (1-w)*float64(west[i]))
	}
	// The rows carry their air alone; the wide blur mixes the air of rows
	// near each other, or a bay on one row would leave a wet stripe.
	rain = blur(g, rain, max(1, int(20*p.scale()+0.5)), 2)

	var land []float32
	for i, m := range metres {
		if m > 0 {
			land = append(land, rain[i])
		}
	}
	top := float32(1)
	if len(land) > 0 {
		top = max(quantile(land, 0.8), 1e-6)
	}

	out := make([]float32, g.n())
	for y := range g.h {
		for x := range g.w {
			i := y*g.w + x
			belts := beltWetness(float64(lats[i]))
			// The blur leaves the dry cells a hair below 0.
			out[i] = float32(clamp(math.Sqrt(math.Max(0, float64(rain[i]/top)))*belts*0.8, 0, 1))
		}
	}

	return out
}

// sweep carries the air along every row, to the east or to the west, and
// gives the rain it leaves on the land.
func sweep(p Params, g grid, metres, temp []float32, east bool) []float32 {
	rain := make([]float32, g.n())
	km := p.cellKm()
	parallel(g.h, func(lo, hi int) {
		for y := lo; y < hi; y++ {
			air := 0.6
			for k := range g.w {
				x := g.w - 1 - k
				prev := y*g.w + x + 1
				if east {
					x, prev = k, y*g.w+k-1
				}
				i := y*g.w + x
				if metres[i] <= 0 {
					evaporation := 0.3 + 0.7*clamp((float64(temp[i])+5)/35, 0, 1)
					air += (1 - air) * 0.004 * km * evaporation
					continue
				}

				rise := 0.0
				if x > 0 && x < g.w-1 {
					rise = math.Max(0, float64(metres[i]-metres[prev]))
				}
				fall := air*0.0008*km + air*math.Min(1, rise/(100*km))*0.6
				fall = math.Min(fall, air)
				air -= fall * 0.85 // some of the rain evaporates back
				rain[i] = float32(fall)
			}
		}
	})

	return rain
}

func beltWetness(lat float64) float64 {
	a := math.Abs(lat)
	return 0.75 + 0.55*math.Exp(-(lat/10)*(lat/10)) -
		0.5*math.Exp(-((a-25)/9)*((a-25)/9)) +
		0.2*math.Exp(-((a-55)/12)*((a-55)/12))
}

// biomeOf reads the Whittaker diagram: temperature against wetness; low
// wet land by rivers is swamp.
func biomeOf(t, m float64, metres float32, lake, wet bool) Biome {
	switch {
	case lake:
		return BiomeLake
	case metres <= 0:
		return BiomeSea
	case t < -9:
		return BiomeGlacier
	case t < -2:
		return BiomeTundra
	case wet && metres < 200 && m > 0.6 && t > 2:
		return BiomeSwamp
	case t < 6:
		if m < 0.2 {
			return BiomeSteppe
		}
		return BiomeTaiga
	case t < 19:
		switch {
		case m < 0.14:
			return BiomeDesert
		case m < 0.32:
			return BiomeSteppe
		case m < 0.55:
			return BiomeMeadow
		}
		return BiomeMixedForest
	}

	switch {
	case m < 0.2:
		return BiomeDesert
	case m < 0.45:
		return BiomeSavanna
	}

	return BiomeJungle
}
