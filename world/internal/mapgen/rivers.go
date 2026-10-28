package mapgen

import (
	"math"
	"slices"
)

// hydrology is where the water of the land flows.
type hydrology struct {
	// filled is the land with its depressions filled to their brims.
	filled []float32
	// down is the cell the water flows to; -1 at the sea.
	down []int32
	// order is the land cells from the sea up, so that upstream comes last.
	order []int32
	flow  []float32
	lake  []bool
	river []uint8
}

// River classes by the water they gather, in cells of wet land on the
// full grid.
var riverFlow = [4]float64{0, 1200, 6000, 30000}

const (
	lakeDepth = 60 // metres of a depression that hold a lake
	lakeShare = 0.012
	fillStep  = 1e-3
)

// flood fills the depressions (priority-flood: the land is reached from
// the sea up, each cell no lower than the one it is reached from, plus a
// little, so that every cell has a way down), gives the flow of every cell
// to its steepest lower neighbour and gathers the rain down the flow.
func flood(p Params, g grid, metres, wetness []float32) hydrology {
	hy := hydrology{
		filled: make([]float32, g.n()),
		down:   make([]int32, g.n()),
		flow:   make([]float32, g.n()),
		lake:   make([]bool, g.n()),
		river:  make([]uint8, g.n()),
	}
	copy(hy.filled, metres)

	seen := make([]bool, g.n())
	var q heap
	var seq int64
	for i, m := range metres {
		hy.down[i] = -1
		x, y := i%g.w, i/g.w
		if m <= 0 || x == 0 || y == 0 || x == g.w-1 || y == g.h-1 {
			seen[i] = true
			if m > 0 {
				q.push(float64(m), i, seq)
				seq++
			}
		}
	}
	// The coast: land next to the sea starts the flood.
	for i, m := range metres {
		if m > 0 || !seen[i] {
			continue
		}
		for k := range 8 {
			j := g.neighbour(i, k)
			if j >= 0 && !seen[j] {
				seen[j] = true
				q.push(float64(metres[j]), j, seq)
				seq++
			}
		}
	}

	for q.len() > 0 {
		it := q.pop()
		i := int(it.i)
		hy.order = append(hy.order, it.i)
		for k := range 8 {
			j := g.neighbour(i, k)
			if j < 0 || seen[j] {
				continue
			}
			seen[j] = true
			hy.down[j] = int32(i)
			hy.filled[j] = max(metres[j], hy.filled[i]+fillStep)
			q.push(float64(hy.filled[j]), j, seq)
			seq++
		}
	}

	hy.findLakes(p, g, metres)
	hy.steepest(g)
	hy.gather(p, wetness)

	return hy
}

// findLakes keeps as lakes the largest filled depressions deep and wide
// enough, together no more than lakeShare of the land; the rest become the
// flat plains the rivers cross, as basins filled with silt are.
func (hy *hydrology) findLakes(p Params, g grid, metres []float32) {
	deep := make([]bool, g.n())
	landCells := 0
	for i, m := range metres {
		deep[i] = m > 0 && hy.filled[i]-m > 1
		if m > 0 {
			landCells++
		}
	}

	minCells := max(4, int(150*p.scale()*p.scale()))
	seen := make([]bool, g.n())
	var lakes [][]int
	for i := range deep {
		if !deep[i] || seen[i] {
			continue
		}
		part := []int{i}
		seen[i] = true
		depth := float32(0)
		for k := 0; k < len(part); k++ {
			c := part[k]
			depth = max(depth, hy.filled[c]-metres[c])
			for n := range 4 {
				j := g.neighbour(c, n)
				if j >= 0 && deep[j] && !seen[j] {
					seen[j] = true
					part = append(part, j)
				}
			}
		}
		if depth >= lakeDepth && len(part) >= minCells {
			lakes = append(lakes, part)
		}
	}

	slices.SortStableFunc(lakes, func(a, b []int) int { return len(b) - len(a) })
	room := int(lakeShare * float64(landCells))
	for _, part := range lakes {
		if len(part) > room {
			continue
		}
		room -= len(part)
		for _, c := range part {
			hy.lake[c] = true
		}
	}
	// The basins left and the hollows too shallow to count are filled, so
	// the land always falls along the rivers.
	for i, m := range metres {
		if m > 0 && !hy.lake[i] {
			metres[i] = hy.filled[i]
		}
	}
}

// steepest sends the water of a slope down its steepest way. On the flats
// of filled basins every way is as steep, so the water keeps the way the
// flood came from the outlet: it winds out of the basin instead of running
// in straight lines.
func (hy *hydrology) steepest(g grid) {
	for _, c := range hy.order {
		i := int(c)
		best, bestSlope := -1, 3*fillStep
		for k := range 8 {
			j := g.neighbour(i, k)
			if j < 0 {
				continue
			}
			if s := float64(hy.filled[i]-hy.filled[j]) / dd8[k]; s > bestSlope {
				best, bestSlope = j, s
			}
		}
		if best >= 0 {
			hy.down[i] = int32(best)
		}
	}
}

func (hy *hydrology) gather(p Params, wetness []float32) {
	for k := len(hy.order) - 1; k >= 0; k-- {
		i := hy.order[k]
		hy.flow[i] += 0.05 + wetness[i]
		if d := hy.down[i]; d >= 0 {
			hy.flow[d] += hy.flow[i]
		}
	}

	area := p.scale() * p.scale()
	for _, i := range hy.order {
		if hy.lake[i] {
			continue
		}
		f := float64(hy.flow[i]) / area
		for c := 3; c >= 1; c-- {
			if f >= riverFlow[c] {
				hy.river[i] = uint8(c)
				break
			}
		}
	}
}

// River is a river drawn on the map: points in cells, from the source to
// the mouth, with the flow at each.
type River struct {
	Name   string
	Points [][2]float32
	Flow   []float32
}

// traceRivers follows every river from its source down to the sea, a lake
// or the river it falls into.
func traceRivers(g grid, hy hydrology) []River {
	fed := make([]bool, g.n())
	for _, i := range hy.order {
		if d := hy.down[i]; hy.river[i] > 0 && d >= 0 {
			fed[d] = true
		}
	}

	drawn := make([]bool, g.n())
	var rivers []River
	// From the highest sources down; a river ends where it meets one
	// already traced.
	for k := len(hy.order) - 1; k >= 0; k-- {
		start := int(hy.order[k])
		if hy.river[start] == 0 || fed[start] {
			continue
		}
		var r River
		for i := start; ; {
			r.Points = append(r.Points, [2]float32{float32(i%g.w) + 0.5, float32(i/g.w) + 0.5})
			r.Flow = append(r.Flow, hy.flow[i])
			if drawn[i] {
				break
			}
			drawn[i] = true
			d := int(hy.down[i])
			if d < 0 || hy.lake[d] || hy.filled[d] <= 0 {
				if d >= 0 {
					r.Points = append(r.Points, [2]float32{float32(d%g.w) + 0.5, float32(d/g.w) + 0.5})
					r.Flow = append(r.Flow, hy.flow[i])
				}
				break
			}
			i = d
		}
		if len(r.Points) >= 3 {
			rivers = append(rivers, r)
		}
	}

	return rivers
}

// wetCells marks the land within two cells of a river, for the swamps.
func wetCells(g grid, hy hydrology) []bool {
	wet := make([]bool, g.n())
	for i, r := range hy.river {
		if r == 0 {
			continue
		}
		x, y := i%g.w, i/g.w
		for dy := -2; dy <= 2; dy++ {
			for dx := -2; dx <= 2; dx++ {
				nx, ny := x+dx, y+dy
				if nx >= 0 && ny >= 0 && nx < g.w && ny < g.h {
					wet[ny*g.w+nx] = true
				}
			}
		}
	}

	return wet
}

// toMetres turns the relief field into metres: up to 5 km of land, down
// to 5.5 km of sea.
func toMetres(h []float32) []float32 {
	m := make([]float32, len(h))
	for i, v := range h {
		if v > 0 {
			m[i] = float32(math.Max(1, 5000*float64(v)))
			continue
		}
		m[i] = float32(math.Min(-1, 5500*float64(v)))
	}

	return m
}
