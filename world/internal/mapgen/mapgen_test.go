package mapgen

import (
	"errors"
	"hash/fnv"
	"math"
	"sync"
	"testing"
)

// A small map, made once: the generation takes a while under -race.
var (
	smallOnce sync.Once
	smallMap  *Map
	smallErr  error
)

func small(t *testing.T) *Map {
	t.Helper()
	smallOnce.Do(func() { smallMap, smallErr = Generate(DefaultParams(42).Scaled(512)) })
	if smallErr != nil {
		t.Fatalf("Generate: %v", smallErr)
	}

	return smallMap
}

func fingerprint(m *Map) uint64 {
	h := fnv.New64a()
	for i := range m.Elevation {
		v := math.Float32bits(m.Elevation[i])
		_, _ = h.Write([]byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24), byte(m.Province[i]), byte(m.Province[i] >> 8), m.River[i], byte(m.Biome[i])})
	}
	for _, pr := range m.Provinces {
		_, _ = h.Write([]byte(pr.Name))
	}

	return h.Sum64()
}

func TestGenerateIsDeterministic(t *testing.T) {
	t.Parallel()
	m := small(t)
	again, err := Generate(DefaultParams(42).Scaled(512))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if fingerprint(m) != fingerprint(again) {
		t.Error("the same seed gave another map")
	}

	other, err := Generate(DefaultParams(43).Scaled(512))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if fingerprint(m) == fingerprint(other) {
		t.Error("another seed gave the same map")
	}
}

func TestGenerateRejectsParams(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		edit func(*Params)
	}{
		{"tiny grid", func(p *Params) { p.Width, p.Height = 64, 32 }},
		{"few plates", func(p *Params) { p.Plates = 2 }},
		{"no land", func(p *Params) { p.LandShare = 0.01 }},
		{"no provinces", func(p *Params) { p.LandProvinces = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := DefaultParams(1)
			tt.edit(&p)
			if _, err := Generate(p); !errors.Is(err, ErrParams) {
				t.Errorf("Generate: %v, want ErrParams", err)
			}
		})
	}
}

// TestFieldsAreFinite: a NaN once cut the rivers wherever it flowed.
func TestFieldsAreFinite(t *testing.T) {
	t.Parallel()
	m := small(t)
	for name, f := range map[string][]float32{"elevation": m.Elevation, "temperature": m.Temperature, "moisture": m.Moisture, "flow": m.Flow} {
		for i, v := range f {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				t.Errorf("%s of cell %d is %v", name, i, v)
				break
			}
		}
	}
}

func TestLandShare(t *testing.T) {
	t.Parallel()
	if share := small(t).LandShare(); share < 0.33 || share > 0.42 {
		t.Errorf("land is %.3f of the map, want 0.33–0.42", share)
	}
}

func TestProvincesHaveNeighbours(t *testing.T) {
	t.Parallel()
	m := small(t)
	land, sea := 0, 0
	for _, pr := range m.Provinces {
		if pr.Sea {
			sea++
		} else {
			land++
		}
		if len(pr.Links) == 0 {
			t.Errorf("province %d %q has no neighbours", pr.ID, pr.Name)
		}
		if !pr.Sea && (len(pr.Resources) < 1 || len(pr.Resources) > 2) {
			t.Errorf("province %d has %d resources, want 1–2", pr.ID, len(pr.Resources))
		}
		if pr.Name == "" {
			t.Errorf("province %d has no name", pr.ID)
		}
		if m.Province[pr.CenterY*m.Width+pr.CenterX] != int32(pr.ID) {
			t.Errorf("the centre of province %d lies outside it", pr.ID)
		}
	}
	if land < m.Params.LandProvinces/2 || sea < 1 {
		t.Errorf("%d provinces and %d zones, want about %d and %d", land, sea, m.Params.LandProvinces, m.Params.SeaZones)
	}
}

// TestNoTinyProvinces: the small pieces join their neighbours.
func TestNoTinyProvinces(t *testing.T) {
	t.Parallel()
	m := small(t)
	landCells, seaCells, land, sea := 0, 0, 0, 0
	for _, pr := range m.Provinces {
		if pr.Sea {
			seaCells += pr.Area
			sea++
		} else {
			landCells += pr.Area
			land++
		}
	}
	for _, pr := range m.Provinces {
		mean := float64(landCells) / float64(land)
		if pr.Sea {
			mean = float64(seaCells) / float64(sea)
		}
		if float64(pr.Area) < 0.2*mean {
			t.Errorf("province %d has %d cells, the mean is %.0f", pr.ID, pr.Area, mean)
		}
	}
}

// TestLandmassesAreConnected: the land provinces of one landmass are linked
// to each other by land, so an army can walk between any two of them.
func TestLandmassesAreConnected(t *testing.T) {
	t.Parallel()
	m := small(t)
	g := grid{w: m.Width, h: m.Height}
	seen := make([]bool, g.n())
	for i := range m.Elevation {
		if m.Elevation[i] <= 0 || seen[i] {
			continue
		}
		mass := map[int32]bool{}
		queue := []int{i}
		seen[i] = true
		for k := 0; k < len(queue); k++ {
			c := queue[k]
			if !m.Provinces[m.Province[c]].Sea {
				mass[m.Province[c]] = true
			}
			for n := range 4 {
				j := g.neighbour(c, n)
				if j >= 0 && !seen[j] && m.Elevation[j] > 0 {
					seen[j] = true
					queue = append(queue, j)
				}
			}
		}
		if len(mass) < 2 {
			continue
		}

		var start int32
		for p := range mass {
			start = p
			break
		}
		reached := map[int32]bool{start: true}
		walk := []int32{start}
		for k := 0; k < len(walk); k++ {
			for _, l := range m.Provinces[walk[k]].Links {
				to := int32(l.To)
				if !l.Sea && mass[to] && !reached[to] {
					reached[to] = true
					walk = append(walk, to)
				}
			}
		}
		if len(reached) != len(mass) {
			t.Errorf("a landmass of %d provinces splits: %d reached from one", len(mass), len(reached))
		}
	}
}

// TestRiversRunDown: every river flows downhill and ends in the sea, a lake
// or another river.
func TestRiversRunDown(t *testing.T) {
	t.Parallel()
	m := small(t)
	if len(m.Rivers) == 0 {
		t.Fatal("no rivers")
	}

	// The cells of every river, to see where one falls into another.
	owners := map[[2]int][]int{}
	for n, rv := range m.Rivers {
		for _, p := range rv.Points {
			c := [2]int{int(p[0]), int(p[1])}
			owners[c] = append(owners[c], n)
		}
	}
	for n, rv := range m.Rivers {
		prev := float32(math.Inf(1))
		for _, p := range rv.Points {
			e := m.Elevation[int(p[1])*m.Width+int(p[0])]
			if e > prev+0.01 {
				t.Errorf("river %d climbs from %.2f to %.2f m", n, prev, e)
				break
			}
			prev = e
		}

		end := rv.Points[len(rv.Points)-1]
		i := int(end[1])*m.Width + int(end[0])
		joins := len(owners[[2]int{int(end[0]), int(end[1])}]) > 1
		if m.Elevation[i] > 0 && !m.Lake[i] && !joins {
			t.Errorf("river %d ends on dry land at %v", n, end)
		}
	}
}
