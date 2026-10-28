package mapgen

import (
	"math"
	"math/rand/v2"
	"slices"
)

// Terrain is what a province is like to cross and to fight in.
type Terrain uint8

const (
	TerrainSea Terrain = iota
	TerrainPlains
	TerrainHills
	TerrainMountains
	TerrainForest
	TerrainDesert
	TerrainSwamp
	TerrainTundra
	terrainCount
)

var terrainNames = [terrainCount]string{"sea", "plains", "hills", "mountains", "forest", "desert", "swamp", "tundra"}

func (t Terrain) String() string { return terrainNames[t] }

// Province is a province of land or a zone of sea.
type Province struct {
	ID      int
	Sea     bool
	Name    string
	Terrain Terrain
	Biome   Biome
	// Resources are the 1–2 goods of the land; none at sea.
	Resources []Resource
	// CenterX, CenterY is the cell farthest inside it: for its army and name.
	CenterX, CenterY int
	Area             int
	Coastal          bool
	Culture          int
	// Population is the thousands living there at the start.
	Population int
	FreeCity   bool
	Links      []Link
}

// Link is a border with a neighbour.
type Link struct {
	To int
	// River says the border runs along a river; Sea that one side is sea.
	River bool
	Sea   bool
}

// divider cuts the land into provinces and the sea into zones.
type divider struct {
	p     Params
	g     grid
	m     *Map
	land  []bool
	label []int32
	r     *rand.Rand
	// radius is that of the mean land province, in cells.
	radius float64
}

func divide(p Params, g grid, m *Map) {
	d := &divider{p: p, g: g, m: m, land: make([]bool, g.n()), label: make([]int32, g.n()), r: stream(p.Seed, streamProvinces)}
	landCells := 0
	for i, e := range m.Elevation {
		d.land[i] = e > 0
		d.label[i] = -1
		if d.land[i] {
			landCells++
		}
	}
	landArea := float64(landCells) / float64(p.LandProvinces)
	seaArea := float64(g.n()-landCells) / float64(p.SeaZones)
	d.radius = math.Sqrt(landArea / math.Pi)

	seeds := d.scatter(true, p.LandProvinces, landArea)
	seaSeeds := d.scatter(false, p.SeaZones, seaArea)
	d.grow(seeds, 0)
	d.grow(seaSeeds, int32(len(seeds)))

	count := len(seeds) + len(seaSeeds)
	count = d.unreached(count, landArea, seaArea)
	d.smooth()
	d.mend(count)
	d.mergeSmall(count, 0.3*landArea, 0.3*seaArea)
	d.finish()
}

// suitability is how much people would settle a land cell: plains by
// rivers and coasts most, mountains, deserts and the cold least. Provinces
// are smaller where it is higher.
func (d *divider) suitability(i int) float64 {
	m := d.m
	s := 1.0
	switch e := m.Elevation[i]; {
	case e > 1600:
		s = 0.25
	case e > 600:
		s = 0.6
	}
	switch m.Biome[i] {
	case BiomeDesert, BiomeGlacier:
		s *= 0.3
	case BiomeTundra:
		s *= 0.4
	case BiomeSwamp, BiomeTaiga:
		s *= 0.7
	}
	if m.River[i] > 0 {
		s *= 1.6
	}

	return clamp(s, 0.15, 2)
}

// scatter throws the seeds of the provinces as a Poisson disk whose
// spacing follows the suitability, closing the spacing until all fit.
func (d *divider) scatter(land bool, count int, area float64) []int {
	var cells []int
	for i, l := range d.land {
		// No province starts in a lake: the lake goes to those around it.
		if l == land && !d.m.Lake[i] {
			cells = append(cells, i)
		}
	}
	if len(cells) == 0 {
		return nil
	}
	count = min(count, len(cells))

	r0 := 0.85 * math.Sqrt(area)
	spacing := func(i int) float64 {
		if land {
			return r0 / math.Sqrt(d.suitability(i))
		}
		if d.m.Elevation[i] > -800 {
			return r0 * 0.8
		}
		return r0 * 1.2
	}

	bucket := math.Max(1, r0)
	bw := int(float64(d.g.w)/bucket) + 1
	buckets := map[int][]int{}
	var seeds []int
	var radii []float64
	near := func(i int, r float64) bool {
		x, y := i%d.g.w, i/d.g.w
		bx, by := int(float64(x)/bucket), int(float64(y)/bucket)
		reach := int(math.Ceil(3*r/bucket)) + 1
		for yy := by - reach; yy <= by+reach; yy++ {
			for xx := bx - reach; xx <= bx+reach; xx++ {
				for _, k := range buckets[yy*bw+xx] {
					s := seeds[k]
					limit := (r + radii[k]) / 2
					if math.Hypot(float64(s%d.g.w-x), float64(s/d.g.w-y)) < limit {
						return true
					}
				}
			}
		}
		return false
	}

	for len(seeds) < count {
		for attempt := 0; attempt < 40*count && len(seeds) < count; attempt++ {
			i := cells[d.r.IntN(len(cells))]
			if land && d.r.Float64()*2 > d.suitability(i) {
				continue
			}
			r := spacing(i)
			if near(i, r) {
				continue
			}
			x, y := i%d.g.w, i/d.g.w
			b := int(float64(y)/bucket)*bw + int(float64(x)/bucket)
			buckets[b] = append(buckets[b], len(seeds))
			seeds = append(seeds, i)
			radii = append(radii, r)
		}
		r0 *= 0.93
	}

	return seeds
}

// riverCrossing is what stepping onto a river costs, in radii of a
// province: a great river is a border almost always, a stream sometimes.
var riverCrossing = [4]float64{0, 0.3, 0.6, 1.0}

// grow spreads the provinces from their seeds by the cheapest way
// (Dijkstra): climbing, crossing a big river or a lake costs much, so the
// borders run along ridges and rivers, as real ones do.
func (d *divider) grow(seeds []int, first int32) {
	if len(seeds) == 0 {
		return
	}
	dist := make([]float64, d.g.n())
	for i := range dist {
		dist[i] = math.Inf(1)
	}

	var q heap
	var seq int64
	for k, s := range seeds {
		dist[s] = 0
		d.label[s] = first + int32(k)
		q.push(0, s, seq)
		seq++
	}

	land := d.land[seeds[0]]
	for q.len() > 0 {
		it := q.pop()
		i := int(it.i)
		if it.p > dist[i] {
			continue
		}
		for k := range 8 {
			j := d.g.neighbour(i, k)
			if j < 0 || d.land[j] != land {
				continue
			}
			c := it.p + d.cost(i, j, k, land)
			if c < dist[j] {
				dist[j] = c
				d.label[j] = d.label[i]
				q.push(c, j, seq)
				seq++
			}
		}
	}
}

func (d *divider) cost(i, j, k int, land bool) float64 {
	if !land {
		return dd8[k]
	}
	m := d.m
	climb := math.Abs(float64(m.Elevation[j] - m.Elevation[i]))
	c := dd8[k] * (1 + climb/(13*d.p.cellKm())) * (1 + float64(m.Elevation[j])/1500)
	if r := m.River[j]; r > 0 && m.River[i] == 0 {
		c += riverCrossing[r] * d.radius
	}
	if m.Lake[j] {
		c += 0.5 * d.radius
	}

	return c
}

// unreached gives provinces to the pieces no seed reached: big islands and
// seas get their own, small ones join the nearest province of land (a
// small closed sea becomes its lake).
func (d *divider) unreached(count int, landArea, seaArea float64) int {
	g := d.g
	seen := make([]bool, g.n())
	var small []int
	for i := range d.label {
		if d.label[i] >= 0 || seen[i] {
			continue
		}
		part := d.component(i, seen, func(j int) bool { return d.label[j] < 0 && d.land[j] == d.land[i] })
		limit := 0.3 * landArea
		if !d.land[i] {
			limit = 0.3 * seaArea
		}
		if float64(len(part)) < limit {
			small = append(small, part...)
			continue
		}
		for _, c := range part {
			d.label[c] = int32(count)
		}
		count++
	}

	if len(small) > 0 {
		owner := d.nearestLand()
		for _, c := range small {
			d.label[c] = owner[c]
			if !d.land[c] {
				d.land[c] = true
				d.m.Lake[c] = true
			}
		}
	}

	return count
}

// component gives the cells joined to i by their sides for which in holds.
func (d *divider) component(i int, seen []bool, in func(int) bool) []int {
	part := []int{i}
	seen[i] = true
	for k := 0; k < len(part); k++ {
		for n := range 4 {
			j := d.g.neighbour(part[k], n)
			if j >= 0 && !seen[j] && in(j) {
				seen[j] = true
				part = append(part, j)
			}
		}
	}

	return part
}

// nearestLand gives every cell the label of the nearest labelled land.
func (d *divider) nearestLand() []int32 {
	owner := make([]int32, d.g.n())
	var queue []int
	for i := range owner {
		owner[i] = -1
		if d.land[i] && d.label[i] >= 0 {
			owner[i] = d.label[i]
			queue = append(queue, i)
		}
	}
	for k := 0; k < len(queue); k++ {
		for n := range 4 {
			j := d.g.neighbour(queue[k], n)
			if j >= 0 && owner[j] < 0 {
				owner[j] = owner[queue[k]]
				queue = append(queue, j)
			}
		}
	}

	return owner
}

// smooth lets a cell join the province that surrounds it, which rounds off
// the teeth of the borders.
func (d *divider) smooth() {
	next := slices.Clone(d.label)
	for range 2 {
		for i := range d.label {
			var labels [8]int32
			var counts [8]int
			n := 0
			for k := range 8 {
				j := d.g.neighbour(i, k)
				if j < 0 || d.land[j] != d.land[i] {
					continue
				}
				at := slices.Index(labels[:n], d.label[j])
				if at < 0 {
					labels[n], at = d.label[j], n
					n++
				}
				counts[at]++
			}
			for k := range n {
				if counts[k] >= 6 {
					next[i] = labels[k]
				}
			}
		}
		copy(d.label, next)
	}
}

// mend gives the stray pieces of a province, cut off from its main part,
// to the neighbour they touch most.
func (d *divider) mend(count int) {
	for range 3 {
		seen := make([]bool, d.g.n())
		parts := make([][][]int, count)
		for i, l := range d.label {
			if l < 0 || seen[i] {
				continue
			}
			parts[l] = append(parts[l], d.component(i, seen, func(j int) bool { return d.label[j] == l }))
		}

		changed := false
		for l, ps := range parts {
			if len(ps) < 2 {
				continue
			}
			main := 0
			for k, part := range ps {
				if len(part) > len(ps[main]) {
					main = k
				}
			}
			for k, part := range ps {
				if k == main {
					continue
				}
				if to, ok := d.touching(part, int32(l)); ok {
					for _, c := range part {
						d.label[c] = to
					}
					changed = true
				}
			}
		}
		if !changed {
			return
		}
	}
}

// touching gives the label of the same kind the cells border most.
func (d *divider) touching(cells []int, own int32) (int32, bool) {
	counts := map[int32]int{}
	for _, c := range cells {
		for n := range 4 {
			j := d.g.neighbour(c, n)
			if j >= 0 && d.label[j] != own && d.label[j] >= 0 && d.land[j] == d.land[c] {
				counts[d.label[j]]++
			}
		}
	}

	best, bestCount := int32(-1), 0
	for l, n := range counts {
		if n > bestCount || (n == bestCount && l < best) {
			best, bestCount = l, n
		}
	}

	return best, best >= 0
}

// mergeSmall joins the provinces smaller than a limit to the neighbour
// they border most, the smallest first; a small island joins the nearest
// province across the sea.
func (d *divider) mergeSmall(count int, landLimit, seaLimit float64) {
	cells := make([][]int, count)
	for i, l := range d.label {
		if l >= 0 {
			cells[l] = append(cells[l], i)
		}
	}

	order := make([]int, count)
	for k := range order {
		order[k] = k
	}
	slices.SortStableFunc(order, func(a, b int) int { return len(cells[a]) - len(cells[b]) })

	for _, l := range order {
		part := cells[l]
		if len(part) == 0 {
			continue
		}
		limit := seaLimit
		if d.land[part[0]] {
			limit = landLimit
		}
		if float64(len(part)) >= limit {
			continue
		}
		to, ok := d.touching(part, int32(l))
		if !ok && d.land[part[0]] {
			to, ok = d.acrossSea(part, int32(l))
		}
		if !ok {
			continue
		}
		for _, c := range part {
			d.label[c] = to
		}
		cells[to] = append(cells[to], part...)
		cells[l] = nil
	}
}

// acrossSea finds the nearest land of another province from an island.
func (d *divider) acrossSea(cells []int, own int32) (int32, bool) {
	seen := make(map[int]bool, len(cells))
	queue := slices.Clone(cells)
	for _, c := range cells {
		seen[c] = true
	}
	for k := 0; k < len(queue); k++ {
		for n := range 4 {
			j := d.g.neighbour(queue[k], n)
			if j < 0 || seen[j] {
				continue
			}
			if d.land[j] && d.label[j] >= 0 && d.label[j] != own {
				return d.label[j], true
			}
			seen[j] = true
			queue = append(queue, j)
		}
	}

	return -1, false
}

// finish numbers the provinces left from 0, land first, and fills in what
// the cells say of them.
func (d *divider) finish() {
	g, m := d.g, d.m
	var used []int32
	isUsed := map[int32]bool{}
	for _, l := range d.label {
		if l >= 0 && !isUsed[l] {
			isUsed[l] = true
			used = append(used, l)
		}
	}
	slices.Sort(used)
	landOf := map[int32]bool{}
	for i, l := range d.label {
		if l >= 0 && d.land[i] {
			landOf[l] = true
		}
	}
	slices.SortStableFunc(used, func(a, b int32) int {
		switch {
		case landOf[a] && !landOf[b]:
			return -1
		case !landOf[a] && landOf[b]:
			return 1
		}
		return 0
	})

	index := map[int32]int32{}
	for k, l := range used {
		index[l] = int32(k)
	}
	m.Province = make([]int32, g.n())
	for i, l := range d.label {
		m.Province[i] = index[l]
	}

	m.Provinces = make([]Province, len(used))
	var counts [][terrainCount]int
	var biomes [][biomeCount]int
	counts = make([][terrainCount]int, len(used))
	biomes = make([][biomeCount]int, len(used))
	for i, l := range m.Province {
		pr := &m.Provinces[l]
		pr.Area++
		pr.Sea = !d.land[i]
		counts[l][d.terrainOf(i)]++
		biomes[l][m.Biome[i]]++
	}
	for k := range m.Provinces {
		pr := &m.Provinces[k]
		pr.ID = k
		pr.Terrain = Terrain(argmax(counts[k][:]))
		pr.Biome = Biome(argmax(biomes[k][:]))
		if !pr.Sea && pr.Terrain == TerrainSea {
			pr.Terrain = TerrainPlains
		}
	}

	d.centers()
	d.links()
}

func argmax(counts []int) int {
	best := 0
	for k, n := range counts {
		if n > counts[best] {
			best = k
		}
	}

	return best
}

// terrainOf is the terrain of a cell: by height first, then by the cover.
func (d *divider) terrainOf(i int) Terrain {
	m := d.m
	if !d.land[i] {
		return TerrainSea
	}
	switch e := m.Elevation[i]; {
	case e > 1500:
		return TerrainMountains
	case e > 550:
		return TerrainHills
	}
	switch b := m.Biome[i]; {
	case b.isForest():
		return TerrainForest
	case b == BiomeDesert:
		return TerrainDesert
	case b == BiomeSwamp:
		return TerrainSwamp
	case b == BiomeTundra || b == BiomeGlacier:
		return TerrainTundra
	case b == BiomeLake || b == BiomeSea:
		return TerrainSea
	}

	return TerrainPlains
}

// centers puts the centre of every province at its cell farthest from its
// borders, where its name and armies fit best.
func (d *divider) centers() {
	g, m := d.g, d.m
	dist := make([]int32, g.n())
	var queue []int
	for i := range dist {
		dist[i] = -1
		border := false
		for n := range 4 {
			j := g.neighbour(i, n)
			if j < 0 || m.Province[j] != m.Province[i] {
				border = true
				break
			}
		}
		if border {
			dist[i] = 0
			queue = append(queue, i)
		}
	}
	for k := 0; k < len(queue); k++ {
		c := queue[k]
		for n := range 4 {
			j := g.neighbour(c, n)
			if j >= 0 && dist[j] < 0 && m.Province[j] == m.Province[c] {
				dist[j] = dist[c] + 1
				queue = append(queue, j)
			}
		}
	}

	best := make([]int32, len(m.Provinces))
	for k := range best {
		best[k] = -1
	}
	for i, l := range m.Province {
		if best[l] < 0 || dist[i] > dist[best[l]] {
			best[l] = int32(i)
		}
	}
	for k, i := range best {
		m.Provinces[k].CenterX, m.Provinces[k].CenterY = int(i)%g.w, int(i)/g.w
	}
}

// links finds the neighbours of every province, whether the border runs
// along a river and whether the province touches the sea.
func (d *divider) links() {
	g, m := d.g, d.m
	type pair struct{ a, b int32 }
	borders := map[pair][2]int{}
	var pairs []pair
	for i := range m.Province {
		for _, n := range [2]int{0, 2} {
			j := g.neighbour(i, n)
			if j < 0 || m.Province[j] == m.Province[i] {
				continue
			}
			a, b := m.Province[i], m.Province[j]
			if a > b {
				a, b = b, a
			}
			k := pair{a, b}
			v, ok := borders[k]
			if !ok {
				pairs = append(pairs, k)
			}
			v[0]++
			if m.River[i] >= 2 || m.River[j] >= 2 {
				v[1]++
			}
			borders[k] = v
		}
	}

	slices.SortFunc(pairs, func(x, y pair) int {
		if x.a != y.a {
			return int(x.a - y.a)
		}
		return int(x.b - y.b)
	})
	for _, k := range pairs {
		v := borders[k]
		a, b := &m.Provinces[k.a], &m.Provinces[k.b]
		sea := a.Sea || b.Sea
		river := !sea && float64(v[1]) >= 0.4*float64(v[0])
		a.Links = append(a.Links, Link{To: int(k.b), River: river, Sea: sea})
		b.Links = append(b.Links, Link{To: int(k.a), River: river, Sea: sea})
		if sea && !a.Sea {
			a.Coastal = true
		}
		if sea && !b.Sea {
			b.Coastal = true
		}
	}
}
