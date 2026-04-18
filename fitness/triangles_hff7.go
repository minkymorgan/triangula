package fitness

import (
	"math"

	"github.com/RH12503/Triangula/geom"
	"github.com/RH12503/Triangula/image"
	"github.com/RH12503/Triangula/rasterize"
	"github.com/RH12503/Triangula/triangulation/incrdelaunay"
)

// trianglesHFF7Function: 7-objective HFF fitness.
//   o0: varR — total R-channel variance across all triangles
//   o1: varG — total G-channel variance
//   o2: varB — total B-channel variance
//   o3: wasteEdge — sum over triangle edge pixels of max(0, threshold − sobel[x,y]);
//                   measures triangle edges that don't lie on image edges
//   o4: hfR — R high-pass residual variance (texture fit, R)
//   o5: hfG — G high-pass residual variance
//   o6: hfB — B high-pass residual variance
//
// Objectives are normalised to [0, 1] by precomputed caps, then HFF-TrueNorth
// aggregates. m = 7 is small enough that raw angular distance has plenty of
// resolution — no CDF correction, no log-space needed.
type trianglesHFF7Function struct {
	target HFF7Target

	// Per-channel max-per-pixel: cap = pixels * 255^2 = pixels * maxCh2.
	maxCh2 float64 // 255*255

	// Edge-waste threshold (0..255). Sobel pixels above this are "real edges"
	// (zero waste); below, each unit under contributes to waste.
	edgeThreshold float64
	// Max wasted-edge total used to normalise o3.
	maxWasteEdge float64

	// Triangulation state (identical plumbing to trianglesImageFunction).
	Triangulation *incrdelaunay.Delaunay
	Base          *incrdelaunay.Delaunay
}

// NewTrianglesHFF7Function: one evaluator per population slot.
func NewTrianglesHFF7Function(t HFF7Target) *trianglesHFF7Function {
	f := &trianglesHFF7Function{
		target:        t,
		maxCh2:        255 * 255,
		edgeThreshold: 64, // out of 255; 64 ≈ quarter-strength — empirically reasonable
	}
	// Rough cap for waste: assume avg triangle edge length ~ sqrt(area/ntri);
	// safest is to use image diagonal × some constant. We'll just use
	// W*H*edgeThreshold as a generous upper bound — same order as image pixels
	// times threshold, so typical normalised waste sits well below 1.
	f.maxWasteEdge = float64(t.W*t.H) * f.edgeThreshold
	return f
}

func (f *trianglesHFF7Function) SetBase(other CacheFunction) {
	if o, ok := other.(*trianglesHFF7Function); ok {
		f.Base = o.Triangulation
	}
}
func (f *trianglesHFF7Function) Cache() []CacheData     { return nil }
func (f *trianglesHFF7Function) SetCache(c []CacheData) {}

// Calculate: evaluate the individual's fitness.
func (f *trianglesHFF7Function) Calculate(data PointsData) float64 {
	points := data.Points
	w, h := f.target.W, f.target.H

	if f.Triangulation == nil {
		f.Triangulation = incrdelaunay.NewDelaunay(w, h)
		for _, p := range points {
			f.Triangulation.Insert(createPoint(p.X, p.Y, w, h))
		}
	} else if f.Base != nil {
		f.Triangulation.Set(f.Base)
		for _, m := range data.Mutations {
			f.Triangulation.Remove(createPoint(m.Old.X, m.Old.Y, w, h))
		}
		for _, m := range data.Mutations {
			f.Triangulation.Insert(createPoint(m.New.X, m.New.Y, w, h))
		}
	}
	f.Base = nil

	// Global accumulators. We sum per-triangle Welford terms into these.
	var varR, varG, varB float64
	var hfR, hfG, hfB float64
	var wasteEdge float64
	var totalPixels int

	f.Triangulation.IterTriangles(func(triangle incrdelaunay.Triangle) {
		a := triangle.A
		b := triangle.B
		c := triangle.C

		// Pass 1: iterate pixels inside the triangle, accumulate per-channel
		// sums + sum-of-squares for both target and high-pass target. Close
		// out variance at end of triangle.
		var sR, sG, sB float64
		var sR2, sG2, sB2 float64
		var sHR, sHG, sHB float64
		var sHR2, sHG2, sHB2 float64
		n := 0

		tri := geom.NewTriangle(int(a.X), int(a.Y), int(b.X), int(b.Y), int(c.X), int(c.Y))
		rasterize.DDATriangle(tri, func(x, y int) {
			if x < 0 || y < 0 || x >= w || y >= h {
				return
			}
			i := y*w + x
			sR += f.target.R[i]
			sG += f.target.G[i]
			sB += f.target.B[i]
			sR2 += f.target.R2[i]
			sG2 += f.target.G2[i]
			sB2 += f.target.B2[i]
			sHR += f.target.HPR[i]
			sHG += f.target.HPG[i]
			sHB += f.target.HPB[i]
			sHR2 += f.target.HPR2[i]
			sHG2 += f.target.HPG2[i]
			sHB2 += f.target.HPB2[i]
			n++
		})
		if n > 0 {
			fn := float64(n)
			varR += sR2 - (sR*sR)/fn
			varG += sG2 - (sG*sG)/fn
			varB += sB2 - (sB*sB)/fn
			hfR += sHR2 - (sHR*sHR)/fn
			hfG += sHG2 - (sHG*sHG)/fn
			hfB += sHB2 - (sHB*sHB)/fn
			totalPixels += n
		}

		// Pass 2: walk the three triangle edges, sum waste per edge pixel.
		wasteEdge += edgeWaste(&f.target, f.edgeThreshold, int(a.X), int(a.Y), int(b.X), int(b.Y))
		wasteEdge += edgeWaste(&f.target, f.edgeThreshold, int(b.X), int(b.Y), int(c.X), int(c.Y))
		wasteEdge += edgeWaste(&f.target, f.edgeThreshold, int(c.X), int(c.Y), int(a.X), int(a.Y))
	})

	// Normalise each objective to [0, 1] by a precomputed cap.
	// Colour variance cap: (total covered pixels) × 255² is the max possible
	// per-channel variance; we use image_pixels × 255² to stay scale-consistent
	// across triangulations.
	cap := float64(w*h) * f.maxCh2
	o := [7]float64{
		clamp01(varR / cap),
		clamp01(varG / cap),
		clamp01(varB / cap),
		clamp01(wasteEdge / f.maxWasteEdge),
		clamp01(hfR / cap),
		clamp01(hfG / cap),
		clamp01(hfB / cap),
	}

	// HFF-TrueNorth at m=7. Energy E = Σ o²; e = max(0, 1 − E/m); θ = acos(e).
	// For m=7 all values sit in [0, π/2], with plenty of gradient.
	var E float64
	for _, v := range o {
		E += v * v
	}
	m := float64(len(o))
	e := 1 - E/m
	if e < 0 {
		e = 0
	}
	if e > 1 {
		e = 1
	}
	theta := math.Acos(e)

	// Higher = better.
	return 1 - theta/math.Pi
}

// edgeWaste walks the pixels along line (x0,y0)→(x1,y1) using a simple
// Bresenham stepper and accumulates (threshold − sobel) where sobel<threshold.
// Returns the total waste contribution for this one edge.
func edgeWaste(t *HFF7Target, threshold float64, x0, y0, x1, y1 int) float64 {
	dx := absi(x1 - x0)
	dy := -absi(y1 - y0)
	sx := 1
	if x0 >= x1 {
		sx = -1
	}
	sy := 1
	if y0 >= y1 {
		sy = -1
	}
	err := dx + dy
	w, h := t.W, t.H
	var waste float64
	for {
		if x0 >= 0 && x0 < w && y0 >= 0 && y0 < h {
			s := float64(t.Sobel[y0*w+x0])
			if s < threshold {
				waste += threshold - s
			}
		}
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
	return waste
}

func absi(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// TrianglesHFF7Functions: one evaluator per population slot, sharing the
// precomputed target. Matches the factory signature expected by
// algorithm.NewModifiedGenetic → evaluator.NewParallel.
func TrianglesHFF7Functions(target image.Data, n int) []CacheFunction {
	t := BuildHFF7Target(target)
	out := make([]CacheFunction, n)
	for i := range out {
		out[i] = NewTrianglesHFF7Function(t)
	}
	return out
}
