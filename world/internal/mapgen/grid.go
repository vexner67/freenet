package mapgen

import (
	"math"
	"runtime"
	"slices"
	"sync"
)

// grid is the size of the cell fields; cell (x, y) is y*w + x.
type grid struct {
	w, h int
}

func (g grid) n() int { return g.w * g.h }

// The eight neighbours, the first four sharing a side.
var (
	dx8 = [8]int{1, -1, 0, 0, 1, -1, 1, -1}
	dy8 = [8]int{0, 0, 1, -1, 1, 1, -1, -1}
	dd8 = [8]float64{1, 1, 1, 1, math.Sqrt2, math.Sqrt2, math.Sqrt2, math.Sqrt2}
)

// neighbour gives the k-th neighbour of cell i, or -1 off the grid.
func (g grid) neighbour(i, k int) int {
	x, y := i%g.w+dx8[k], i/g.w+dy8[k]
	if x < 0 || y < 0 || x >= g.w || y >= g.h {
		return -1
	}

	return y*g.w + x
}

// parallel runs f over [0, n) in chunks on all processors. f must only
// write what its own chunk owns, so the result does not depend on timing.
func parallel(n int, f func(lo, hi int)) {
	workers := min(runtime.GOMAXPROCS(0), n)
	if workers <= 1 {
		f(0, n)
		return
	}

	chunk := (n + workers - 1) / workers
	var wg sync.WaitGroup
	for lo := 0; lo < n; lo += chunk {
		wg.Go(func() { f(lo, min(lo+chunk, n)) })
	}
	wg.Wait()
}

// blur smooths a field with passes of a box blur of radius r, which
// together come close to a Gaussian one. The edges repeat their cells.
func blur(g grid, f []float32, r, passes int) []float32 {
	if r < 1 {
		return slices.Clone(f)
	}

	out := slices.Clone(f)
	tmp := make([]float32, len(f))
	for range passes {
		boxRows(g, out, tmp, r)
		boxColumns(g, tmp, out, r)
	}

	return out
}

func boxRows(g grid, src, dst []float32, r int) {
	norm := 1 / float32(2*r+1)
	parallel(g.h, func(lo, hi int) {
		for y := lo; y < hi; y++ {
			row := src[y*g.w : (y+1)*g.w]
			var sum float32
			for k := -r; k <= r; k++ {
				sum += row[clampInt(k, 0, g.w-1)]
			}
			for x := range g.w {
				dst[y*g.w+x] = sum * norm
				sum += row[clampInt(x+r+1, 0, g.w-1)] - row[clampInt(x-r, 0, g.w-1)]
			}
		}
	})
}

func boxColumns(g grid, src, dst []float32, r int) {
	norm := 1 / float32(2*r+1)
	parallel(g.w, func(lo, hi int) {
		for x := lo; x < hi; x++ {
			var sum float32
			for k := -r; k <= r; k++ {
				sum += src[clampInt(k, 0, g.h-1)*g.w+x]
			}
			for y := range g.h {
				dst[y*g.w+x] = sum * norm
				sum += src[clampInt(y+r+1, 0, g.h-1)*g.w+x] - src[clampInt(y-r, 0, g.h-1)*g.w+x]
			}
		}
	})
}

// nearest gives, for every cell, the nearest source cell and the distance
// to it, by the two passes of vector propagation (8SSEDT): close to the
// Euclidean distance in one sweep each way. Without sources it gives -1.
func nearest(g grid, source []bool) ([]int32, []float32) {
	src := make([]int32, g.n())
	dist := make([]float32, g.n())
	inf := float32(math.Inf(1))
	for i := range src {
		src[i] = -1
		dist[i] = inf
		if source[i] {
			src[i] = int32(i)
			dist[i] = 0
		}
	}

	try := func(i, x, y, nx, ny int) {
		if nx < 0 || ny < 0 || nx >= g.w || ny >= g.h {
			return
		}
		s := src[ny*g.w+nx]
		if s < 0 {
			return
		}
		sx, sy := float64(int(s)%g.w-x), float64(int(s)/g.w-y)
		if d := float32(math.Hypot(sx, sy)); d < dist[i] {
			dist[i], src[i] = d, s
		}
	}

	for y := range g.h {
		for x := range g.w {
			i := y*g.w + x
			try(i, x, y, x-1, y-1)
			try(i, x, y, x, y-1)
			try(i, x, y, x+1, y-1)
			try(i, x, y, x-1, y)
		}
		for x := g.w - 1; x >= 0; x-- {
			try(y*g.w+x, x, y, x+1, y)
		}
	}
	for y := g.h - 1; y >= 0; y-- {
		for x := g.w - 1; x >= 0; x-- {
			i := y*g.w + x
			try(i, x, y, x+1, y+1)
			try(i, x, y, x, y+1)
			try(i, x, y, x-1, y+1)
			try(i, x, y, x+1, y)
		}
		for x := range g.w {
			try(y*g.w+x, x, y, x-1, y)
		}
	}

	return src, dist
}

// quantile gives the value below which part q of the values lie.
func quantile(f []float32, q float64) float32 {
	sorted := slices.Clone(f)
	slices.Sort(sorted)

	return sorted[clampInt(int(q*float64(len(sorted))), 0, len(sorted)-1)]
}

// heap is a binary min-heap of cells by a priority, for the flooding and
// the growth of provinces: millions of cells, so no container/heap.
type heap struct {
	items []heapItem
}

type heapItem struct {
	p float64
	i int32
	// seq breaks the ties of equal priorities in the order of pushing, so
	// the result does not depend on the layout of the heap.
	seq int64
}

func (h *heap) less(a, b int) bool {
	x, y := h.items[a], h.items[b]
	if x.p != y.p {
		return x.p < y.p
	}

	return x.seq < y.seq
}

func (h *heap) push(p float64, i int, seq int64) {
	h.items = append(h.items, heapItem{p: p, i: int32(i), seq: seq})
	c := len(h.items) - 1
	for c > 0 {
		parent := (c - 1) / 2
		if !h.less(c, parent) {
			break
		}
		h.items[c], h.items[parent] = h.items[parent], h.items[c]
		c = parent
	}
}

func (h *heap) pop() heapItem {
	top := h.items[0]
	last := len(h.items) - 1
	h.items[0] = h.items[last]
	h.items = h.items[:last]

	c := 0
	for {
		l, r, m := 2*c+1, 2*c+2, c
		if l < last && h.less(l, m) {
			m = l
		}
		if r < last && h.less(r, m) {
			m = r
		}
		if m == c {
			break
		}
		h.items[c], h.items[m] = h.items[m], h.items[c]
		c = m
	}

	return top
}

func (h *heap) len() int { return len(h.items) }

func clampInt(v, lo, hi int) int {
	return max(lo, min(v, hi))
}

func clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(v, hi))
}

func smoothstep(a, b, v float64) float64 {
	t := clamp((v-a)/(b-a), 0, 1)
	return t * t * (3 - 2*t)
}
