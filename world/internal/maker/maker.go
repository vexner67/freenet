// Package maker makes the map of a world with mapgen and cuts it into
// what the service keeps: the provinces, the raster of cells and the tiles
// of the pictures.
package maker

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/png"

	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/world/internal/mapgen"
	"github.com/vexner67/freenet/world/internal/service"
	"github.com/vexner67/freenet/world/internal/world"
)

const (
	// TileSize is the side of a tile in pixels.
	TileSize = 256
	// BaseWidth is the width of the world at zoom 0: two tiles side by
	// side, as the world is twice as wide as high.
	BaseWidth = 2 * TileSize
)

var ErrTopWidth = errors.New("the top width is not a base width times a power of two")

// Maker makes maps whose pictures are top pixels wide at the closest zoom.
type Maker struct {
	top int
}

// New is a maker drawing the closest zoom top pixels wide: BaseWidth
// times a power of two.
func New(top int) (*Maker, error) {
	if top < BaseWidth || top%BaseWidth != 0 || (top/BaseWidth)&(top/BaseWidth-1) != 0 {
		return nil, ErrTopWidth
	}

	return &Maker{top: top}, nil
}

// Zooms is how many zooms the pictures have.
func (m *Maker) Zooms() int {
	n := 0
	for w := BaseWidth; w <= m.top; w *= 2 {
		n++
	}

	return n
}

// Make generates the map of seed on a grid grid cells wide. It cannot
// stop halfway: ctx is looked at between the stages.
func (m *Maker) Make(ctx context.Context, seed uint64, grid int) (service.MadeMap, error) {
	p := mapgen.DefaultParams(seed)
	if grid != p.Width {
		p = p.Scaled(grid)
	}

	g, err := mapgen.Generate(p)
	if err != nil {
		return service.MadeMap{}, errs.Wrap("mapgen.Generate", err)
	}

	if err = ctx.Err(); err != nil {
		return service.MadeMap{}, errs.Wrap("ctx.Err", err)
	}

	raster, err := rasterOf(g)
	if err != nil {
		return service.MadeMap{}, errs.Wrap("rasterOf", err)
	}

	tiles, err := m.tiles(ctx, mapgen.Render(g, m.top))
	if err != nil {
		return service.MadeMap{}, errs.Wrap("m.tiles", err)
	}

	return service.MadeMap{Atlas: atlasOf(g), Raster: raster, Tiles: tiles}, nil
}

func atlasOf(g *mapgen.Map) *world.Atlas {
	a := &world.Atlas{Width: g.Width, Height: g.Height, Provinces: make([]world.Province, len(g.Provinces))}
	for i, p := range g.Provinces {
		resources := make([]world.Resource, len(p.Resources))
		for k, r := range p.Resources {
			resources[k] = world.Resource(r)
		}
		links := make([]world.Link, len(p.Links))
		for k, l := range p.Links {
			links[k] = world.Link{To: l.To, River: l.River, Sea: l.Sea}
		}

		a.Provinces[i] = world.Province{
			ID: p.ID, Sea: p.Sea, Name: p.Name, Terrain: world.Terrain(p.Terrain.String()), Biome: p.Biome.String(),
			Resources: resources, CenterX: p.CenterX, CenterY: p.CenterY, Area: p.Area, Coastal: p.Coastal,
			Culture: p.Culture, Population: p.Population, FreeCity: p.FreeCity, Links: links,
		}
	}

	return a
}

// rasterOf is the province of every cell as little-endian uint16, gzipped:
// the site finds the province under a finger with it and draws its
// border.
func rasterOf(g *mapgen.Map) ([]byte, error) {
	cells := make([]byte, 2*len(g.Province))
	for i, p := range g.Province {
		binary.LittleEndian.PutUint16(cells[2*i:], uint16(p))
	}

	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, errs.Wrap("gzip.NewWriterLevel", err)
	}

	if _, err = zw.Write(cells); err != nil {
		return nil, errs.Wrap("zw.Write", err)
	}

	if err = zw.Close(); err != nil {
		return nil, errs.Wrap("zw.Close", err)
	}

	return buf.Bytes(), nil
}

// tiles cuts the picture of the closest zoom and every smaller one, each
// half the one before.
func (m *Maker) tiles(ctx context.Context, top *image.RGBA) ([]service.Tile, error) {
	var out []service.Tile
	img := top
	for z := m.Zooms() - 1; z >= 0; z-- {
		if err := ctx.Err(); err != nil {
			return nil, errs.Wrap("ctx.Err", err)
		}

		b := img.Bounds()
		for y := 0; y < b.Dy(); y += TileSize {
			for x := 0; x < b.Dx(); x += TileSize {
				data, err := encode(img.SubImage(image.Rect(x, y, x+TileSize, y+TileSize)))
				if err != nil {
					return nil, errs.Wrap("encode", err)
				}
				out = append(out, service.Tile{Z: z, X: x / TileSize, Y: y / TileSize, PNG: data})
			}
		}

		img = half(img)
	}

	return out, nil
}

func encode(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, errs.Wrap("png.Encode", err)
	}

	return buf.Bytes(), nil
}

// half is img at half the size, every pixel the mean of four.
func half(img *image.RGBA) *image.RGBA {
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx()/2, b.Dy()/2))
	for y := range b.Dy() / 2 {
		for x := range b.Dx() / 2 {
			var sum [4]int
			for _, d := range [4][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
				i := img.PixOffset(2*x+d[0], 2*y+d[1])
				for c := range 4 {
					sum[c] += int(img.Pix[i+c])
				}
			}
			o := out.PixOffset(x, y)
			for c := range 4 {
				out.Pix[o+c] = uint8((sum[c] + 2) / 4)
			}
		}
	}

	return out
}
