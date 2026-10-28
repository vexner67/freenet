// Command mapgen generates the map of a world and draws it into PNG files,
// to see how a seed looks before a world is made of it:
//
//	go run ./world/cmd/mapgen -seed 42 -out /tmp/map.png -provinces /tmp/provinces.png
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"time"

	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/world/internal/mapgen"
)

func main() {
	seed := flag.Uint64("seed", 42, "seed of the world")
	grid := flag.Int("grid", 4096, "width of the grid of cells (the full map is 4096)")
	width := flag.Int("width", 0, "width of the pictures in pixels (0 — as the grid)")
	out := flag.String("out", "map.png", "the physical map")
	provinces := flag.String("provinces", "", "the provinces over it, if given")
	flag.Parse()

	if err := run(*seed, *grid, *width, *out, *provinces); err != nil {
		fmt.Fprintln(os.Stderr, "mapgen:", err)
		os.Exit(1)
	}
}

func run(seed uint64, grid, width int, out, provinces string) error {
	p := mapgen.DefaultParams(seed)
	if grid != p.Width {
		p = p.Scaled(grid)
	}
	if width == 0 {
		width = p.Width
	}

	start := time.Now()
	m, err := mapgen.Generate(p)
	if err != nil {
		return errs.Wrap("mapgen.Generate", err)
	}
	for _, t := range m.Timings {
		fmt.Printf("%-10s %v\n", t.Stage, t.Took.Round(time.Millisecond))
	}

	land, sea, free := 0, 0, 0
	for _, pr := range m.Provinces {
		switch {
		case pr.Sea:
			sea++
		case pr.FreeCity:
			free++
			land++
		default:
			land++
		}
	}
	fmt.Printf("grid %d×%d, land %.1f%%, %d provinces, %d sea zones, %d free cities, %d rivers, in %v\n",
		m.Width, m.Height, 100*m.LandShare(), land, sea, free, len(m.Rivers), time.Since(start).Round(time.Millisecond))

	if err := save(out, mapgen.Render(m, width)); err != nil {
		return errs.Wrap("save", err)
	}
	if provinces != "" {
		if err := save(provinces, mapgen.RenderProvinces(m, width)); err != nil {
			return errs.Wrap("save", err)
		}
	}

	return nil
}

func save(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return errs.Wrap("os.Create", err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close() // the encoding error is the one to report
		return errs.Wrap("png.Encode", err)
	}

	return errs.Wrap("f.Close", f.Close())
}
