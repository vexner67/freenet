package mapgen

import (
	"math"
	"math/rand/v2"
)

// noise is 2D simplex noise over a permutation drawn from the seed.
type noise struct {
	perm [512]uint8
}

func newNoise(r *rand.Rand) *noise {
	n := &noise{}
	for i, v := range r.Perm(256) {
		n.perm[i] = uint8(v)
		n.perm[i+256] = uint8(v)
	}

	return n
}

var grad2 = [8][2]float64{{1, 1}, {-1, 1}, {1, -1}, {-1, -1}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}}

const (
	skewF2   = 0.36602540378443865 // (sqrt(3)-1)/2
	unskewG2 = 0.21132486540518713 // (3-sqrt(3))/6
)

// at gives the noise at a point, in about [-1, 1].
func (n *noise) at(x, y float64) float64 {
	s := (x + y) * skewF2
	i, j := math.Floor(x+s), math.Floor(y+s)
	t := (i + j) * unskewG2
	x0, y0 := x-(i-t), y-(j-t)

	i1, j1 := 0.0, 1.0
	if x0 > y0 {
		i1, j1 = 1, 0
	}
	x1, y1 := x0-i1+unskewG2, y0-j1+unskewG2
	x2, y2 := x0-1+2*unskewG2, y0-1+2*unskewG2

	ii, jj := int(i)&255, int(j)&255
	g0 := n.perm[ii+int(n.perm[jj])] & 7
	g1 := n.perm[ii+int(i1)+int(n.perm[jj+int(j1)])] & 7
	g2 := n.perm[ii+1+int(n.perm[jj+1])] & 7

	return 70 * (corner(x0, y0, g0) + corner(x1, y1, g1) + corner(x2, y2, g2))
}

func corner(x, y float64, g uint8) float64 {
	t := 0.5 - x*x - y*y
	if t <= 0 {
		return 0
	}
	t *= t

	return t * t * (grad2[g][0]*x + grad2[g][1]*y)
}

// fbm sums octaves of the noise, each twice as fine and half as strong,
// in about [-1, 1].
func (n *noise) fbm(x, y float64, octaves int) float64 {
	var sum, amp, norm = 0.0, 1.0, 0.0
	for range octaves {
		sum += amp * n.at(x, y)
		norm += amp
		amp *= 0.5
		x, y = x*2+17.3, y*2-9.1
	}

	return sum / norm
}

// ridged is noise with sharp crests, for the chains of peaks, in [0, 1].
func (n *noise) ridged(x, y float64, octaves int) float64 {
	var sum, amp, norm = 0.0, 1.0, 0.0
	for range octaves {
		v := 1 - math.Abs(n.at(x, y))
		sum += amp * v * v
		norm += amp
		amp *= 0.5
		x, y = x*2+5.7, y*2+3.9
	}

	return sum / norm
}
