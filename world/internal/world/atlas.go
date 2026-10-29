package world

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
	// CenterX, CenterY is the cell of its name on the grid.
	CenterX, CenterY int
	Area             int
	Coastal          bool
	Culture          int
	// Population is the thousands living there.
	Population int
	// FreeCity is a city of its own, drawn apart on the map.
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
