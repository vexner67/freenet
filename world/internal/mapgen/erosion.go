package mapgen

import (
	"math"
)

// The droplet erosion, as in the classic algorithm.
const (
	dropInertia    = 0.05
	dropCapacity   = 4.0
	dropMinSlope   = 0.01
	dropDeposit    = 0.3
	dropErode      = 0.3
	dropEvaporate  = 0.01
	dropGravity    = 4.0
	dropLifetime   = 64
	dropBrush      = 3
	thermalTalus   = 0.7 // tan(35°): steeper slopes crumble
	thermalShare   = 0.25
	thermalPasses  = 6
	erosionLateral = 256.0 // the grid the erosion constants are made for
)

// erode runs drops of rain down the relief, which carry away the slopes
// and leave it in valleys and at the sea, then lets the steepest slopes
// crumble. The relief is in units with the sea at 0 and the peaks near 1;
// erosion works on it stretched to slopes of the grid it was made for.
func erode(p Params, g grid, h []float32) {
	k := float32(float64(g.w) / erosionLateral)
	for i := range h {
		h[i] *= k
	}

	droplets(p, g, h)
	thermal(g, h)

	for i := range h {
		h[i] /= k
	}
}

type brushCell struct {
	dx, dy int
	w      float32
}

func brush() []brushCell {
	var cells []brushCell
	var sum float32
	for dy := -dropBrush; dy <= dropBrush; dy++ {
		for dx := -dropBrush; dx <= dropBrush; dx++ {
			d := math.Hypot(float64(dx), float64(dy))
			if d >= dropBrush {
				continue
			}
			w := float32(1 - d/dropBrush)
			cells = append(cells, brushCell{dx, dy, w})
			sum += w
		}
	}
	for i := range cells {
		cells[i].w /= sum
	}

	return cells
}

func droplets(p Params, g grid, h []float32) {
	r := stream(p.Seed, streamErosion)
	b := brush()

	for range p.Droplets {
		x, y := r.Float64()*float64(g.w-1), r.Float64()*float64(g.h-1)
		if sample(g, h, x, y) <= 0 {
			continue
		}

		var dirX, dirY, sediment float64
		speed, water := 1.0, 1.0
		for range dropLifetime {
			cx, cy := int(x), int(y)
			height, gx, gy := gradient(g, h, x, y)

			dirX = dirX*dropInertia - gx*(1-dropInertia)
			dirY = dirY*dropInertia - gy*(1-dropInertia)
			l := math.Hypot(dirX, dirY)
			if l == 0 {
				break
			}
			dirX, dirY = dirX/l, dirY/l
			nx, ny := x+dirX, y+dirY
			if nx < 0 || ny < 0 || nx >= float64(g.w-1) || ny >= float64(g.h-1) {
				break
			}

			next := sample(g, h, nx, ny)
			dh := next - height
			capacity := math.Max(-dh*speed*water*dropCapacity, dropMinSlope)

			if sediment > capacity || dh > 0 {
				amount := (sediment - capacity) * dropDeposit
				if dh > 0 {
					amount = math.Min(dh, sediment)
				}
				sediment -= amount
				deposit(g, h, x, y, amount)
			} else {
				amount := math.Min((capacity-sediment)*dropErode, -dh)
				sediment += carve(g, h, b, cx, cy, amount)
			}

			speed = math.Sqrt(math.Max(0, speed*speed-dh*dropGravity))
			water *= 1 - dropEvaporate
			x, y = nx, ny

			// At the sea the drop leaves all it carries: deltas.
			if next <= 0 {
				deposit(g, h, x, y, sediment)
				break
			}
		}
	}
}

// carve takes amount from the cells around (cx, cy), never below the sea,
// and gives what it took.
func carve(g grid, h []float32, b []brushCell, cx, cy int, amount float64) float64 {
	var taken float64
	for _, c := range b {
		x, y := cx+c.dx, cy+c.dy
		if x < 0 || y < 0 || x >= g.w || y >= g.h {
			continue
		}
		i := y*g.w + x
		take := math.Min(float64(c.w)*amount, math.Max(0, float64(h[i])))
		h[i] -= float32(take)
		taken += take
	}

	return taken
}

func deposit(g grid, h []float32, x, y, amount float64) {
	cx, cy := int(x), int(y)
	u, v := x-float64(cx), y-float64(cy)
	i := cy*g.w + cx
	h[i] += float32(amount * (1 - u) * (1 - v))
	h[i+1] += float32(amount * u * (1 - v))
	h[i+g.w] += float32(amount * (1 - u) * v)
	h[i+g.w+1] += float32(amount * u * v)
}

// sample is the bilinear height between cells.
func sample(g grid, h []float32, x, y float64) float64 {
	height, _, _ := gradient(g, h, x, y)
	return height
}

func gradient(g grid, h []float32, x, y float64) (height, gx, gy float64) {
	cx, cy := int(x), int(y)
	cx, cy = min(cx, g.w-2), min(cy, g.h-2)
	u, v := x-float64(cx), y-float64(cy)
	i := cy*g.w + cx
	nw, ne := float64(h[i]), float64(h[i+1])
	sw, se := float64(h[i+g.w]), float64(h[i+g.w+1])

	gx = (ne-nw)*(1-v) + (se-sw)*v
	gy = (sw-nw)*(1-u) + (se-ne)*u
	height = nw*(1-u)*(1-v) + ne*u*(1-v) + sw*(1-u)*v + se*u*v

	return height, gx, gy
}

// thermal lets the land slopes steeper than the talus crumble down to it.
func thermal(g grid, h []float32) {
	delta := make([]float32, len(h))
	for range thermalPasses {
		clear(delta)
		for i, v := range h {
			if v <= 0 {
				continue
			}
			for k := range 8 {
				j := g.neighbour(i, k)
				if j < 0 {
					continue
				}
				if d := float64(v-h[j]) / dd8[k]; d > thermalTalus {
					move := float32(thermalShare * (d - thermalTalus) * dd8[k] / 8)
					delta[i] -= move
					delta[j] += move
				}
			}
		}
		for i := range h {
			h[i] += delta[i]
		}
	}
}
