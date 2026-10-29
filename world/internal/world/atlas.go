package world

import "errors"

var ErrProvinceNotFound = errors.New("province not found")

// Terrain is what most of a province is.
type Terrain string

const (
	TerrainSea       Terrain = "sea"
	TerrainPlains    Terrain = "plains"
	TerrainHills     Terrain = "hills"
	TerrainMountains Terrain = "mountains"
	TerrainForest    Terrain = "forest"
	TerrainDesert    Terrain = "desert"
	TerrainSwamp     Terrain = "swamp"
	TerrainTundra    Terrain = "tundra"
)

// Resource is a good a province yields.
type Resource string

const (
	ResourceGrain  Resource = "grain"
	ResourceWood   Resource = "wood"
	ResourceOre    Resource = "ore"
	ResourceOil    Resource = "oil"
	ResourceGold   Resource = "gold"
	ResourceHorses Resource = "horses"
)

// Link is a border of a province with a neighbour: along a river, or
// across a strait of sea between two lands.
type Link struct {
	To    int
	River bool
	Sea   bool
}

// Province is a province of land or a zone of sea as the map was made: it
// does not change while the world lives.
type Province struct {
	ID        int
	Sea       bool
	Name      string
	Terrain   Terrain
	Biome     string
	Resources []Resource
	// CenterX, CenterY is the cell of its name and army on the grid.
	CenterX, CenterY int
	Area             int
	Coastal          bool
	Culture          int
	// Population is the thousands living there.
	Population int
	// FreeCity belongs to nobody and cannot be founded on or settled.
	FreeCity bool
	Links    []Link
}

// Atlas is the map of a world: the grid and its provinces, numbered from 0.
type Atlas struct {
	Width     int
	Height    int
	Provinces []Province
	// Zooms is how many zooms of tiles the map is drawn at, from 0.
	Zooms int
}

// Province is the province id; false for none.
func (a *Atlas) Province(id int) (Province, bool) {
	if id < 0 || id >= len(a.Provinces) {
		return Province{}, false
	}

	return a.Provinces[id], true
}

// isLand says whether id is a province of land.
func (a *Atlas) isLand(id int) bool {
	p, ok := a.Province(id)
	return ok && !p.Sea
}

// landNeighbours are the provinces of land next to id, across a strait
// too, in the order of the links.
func (a *Atlas) landNeighbours(id int) []int {
	p, ok := a.Province(id)
	if !ok {
		return nil
	}

	var out []int
	for _, l := range p.Links {
		if a.isLand(l.To) {
			out = append(out, l.To)
		}
	}

	return out
}

// distances are the steps over land from id to every province of land
// within limit steps.
func (a *Atlas) distances(id, limit int) map[int]int {
	dist := map[int]int{id: 0}
	queue := []int{id}
	for k := 0; k < len(queue); k++ {
		c := queue[k]
		if dist[c] == limit {
			continue
		}
		for _, n := range a.landNeighbours(c) {
			if _, seen := dist[n]; !seen {
				dist[n] = dist[c] + 1
				queue = append(queue, n)
			}
		}
	}

	return dist
}
