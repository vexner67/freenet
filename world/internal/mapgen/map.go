package mapgen

import (
	"time"

	"github.com/vexner67/freenet/pkg/errs"
)

// Map is a generated world: the fields of its cells and its provinces.
type Map struct {
	Params Params
	Width  int
	Height int
	// Elevation is in metres, below 0 under the sea.
	Elevation   []float32
	Temperature []float32
	Moisture    []float32
	Biome       []Biome
	// Flow is the rain gathered by a cell, River its class (0 — none,
	// 1–3), Lake whether it is under a lake.
	Flow  []float32
	River []uint8
	Lake  []bool
	// Province is the index of a cell's province in Provinces.
	Province  []int32
	Provinces []Province
	Rivers    []River
	// Timings say how long each stage took.
	Timings []Timing
}

type Timing struct {
	Stage string
	Took  time.Duration
}

// Generate makes the map of the parameters. The same parameters give the
// same map.
func Generate(p Params) (*Map, error) {
	if err := p.validate(); err != nil {
		return nil, errs.Wrap("p.validate", err)
	}

	g := grid{w: p.Width, h: p.Height}
	m := &Map{Params: p, Width: g.w, Height: g.h}
	stage := func(name string, f func()) {
		start := time.Now()
		f()
		m.Timings = append(m.Timings, Timing{Stage: name, Took: time.Since(start)})
	}

	var relief []float32
	stage("relief", func() { relief = elevation(p, g) })
	stage("erosion", func() { erode(p, g, relief) })
	stage("climate", func() {
		m.Elevation = toMetres(relief)
		lats := climateLatitude(p, g)
		m.Temperature = temperature(p, g, m.Elevation, lats)
		m.Moisture = moisture(p, g, m.Elevation, m.Temperature, lats)
	})

	var hy hydrology
	stage("rivers", func() {
		hy = flood(p, g, m.Elevation, m.Moisture)
		m.Flow, m.River, m.Lake = hy.flow, hy.river, hy.lake
		m.Rivers = traceRivers(g, hy)
	})
	stage("biomes", func() {
		wet := wetCells(g, hy)
		m.Biome = make([]Biome, g.n())
		for i := range m.Biome {
			m.Biome[i] = biomeOf(float64(m.Temperature[i]), float64(m.Moisture[i]), m.Elevation[i], m.Lake[i], wet[i])
		}
	})
	stage("provinces", func() { divide(p, g, m) })
	stage("resources", func() { describe(p, g, m) })
	stage("names", func() { name(p, g, m) })

	return m, nil
}

// LandShare is the part of the cells above the sea.
func (m *Map) LandShare() float64 {
	land := 0
	for _, e := range m.Elevation {
		if e > 0 {
			land++
		}
	}

	return float64(land) / float64(len(m.Elevation))
}
