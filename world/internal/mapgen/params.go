// Package mapgen generates the map of a world: plates, relief, erosion,
// climate, rivers, provinces, their resources and names. The same seed
// gives the same map. It is pure computation, heavy but without storage:
// the world service keeps what it makes.
package mapgen

import (
	"errors"
	"math"
	"math/rand/v2"
)

// The full grid of a world: 4096×2048 cells of about 3 km.
const (
	fullWidth  = 4096
	fullHeight = 2048

	// worldKm is the width of the world, for the climate and the relief.
	worldKm = 12000
)

var ErrParams = errors.New("invalid map parameters")

// Params are what a map is made of. Droplets and province counts are for
// the grid they are given with; Scaled gives them for another one.
type Params struct {
	Seed   uint64
	Width  int
	Height int
	// Plates is the number of tectonic plates; 0 picks 12–18 by the seed.
	Plates int
	// LandShare is the part of the surface above the sea.
	LandShare float64
	// Droplets is the number of drops of the hydraulic erosion.
	Droplets      int
	LandProvinces int
	SeaZones      int
}

// DefaultParams gives the full map of a world.
func DefaultParams(seed uint64) Params {
	return Params{
		Seed:          seed,
		Width:         fullWidth,
		Height:        fullHeight,
		LandShare:     0.37,
		Droplets:      1_500_000,
		LandProvinces: 1500,
		SeaZones:      200,
	}
}

// Scaled gives the parameters for a grid width cells wide: the droplets
// scale with the area, the provinces stay as many, at least 1 per 400 cells
// of land, so that a small grid still has provinces of a few cells.
func (p Params) Scaled(width int) Params {
	k := float64(width) / float64(p.Width)
	p.Height = int(math.Round(float64(p.Height) * k))
	p.Width = width
	p.Droplets = int(float64(p.Droplets) * k * k)

	limit := int(float64(p.Width*p.Height) * p.LandShare / 400)
	p.LandProvinces = min(p.LandProvinces, limit)
	p.SeaZones = min(p.SeaZones, max(4, limit/6))

	return p
}

func (p Params) validate() error {
	switch {
	case p.Width < 128 || p.Height < 64 || p.Width > 16384 || p.Height > 8192:
		return ErrParams
	case p.Plates != 0 && (p.Plates < 4 || p.Plates > 40):
		return ErrParams
	case p.LandShare < 0.1 || p.LandShare > 0.8:
		return ErrParams
	case p.Droplets < 0 || p.LandProvinces < 1 || p.SeaZones < 1:
		return ErrParams
	}

	return nil
}

// cellKm is the size of a cell.
func (p Params) cellKm() float64 {
	return worldKm / float64(p.Width)
}

// scale is the size of the grid against the full one, for the distances
// counted in cells.
func (p Params) scale() float64 {
	return float64(p.Width) / fullWidth
}

// Each stage draws from its own stream of the seed, so that changing one
// stage does not change what the others draw.
const (
	streamPlates uint64 = iota + 1
	streamRelief
	streamErosion
	streamClimate
	streamProvinces
	streamResources
	streamNames
)

func stream(seed, stage uint64) *rand.Rand {
	return rand.New(rand.NewPCG(seed, stage))
}
