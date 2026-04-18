package fitness

import (
	"math"

	"github.com/RH12503/Triangula/geom"
	"github.com/RH12503/Triangula/image"
	"github.com/RH12503/Triangula/rasterize"
	"github.com/RH12503/Triangula/triangulation/incrdelaunay"
)

// trianglesQuadFunction is the HFF multi-objective fitness that uses a
// quadtree decomposition of the target image. Each leaf is one objective.
// Fitness = 1 - CDF(theta, m) where theta is HFF-TrueNorth angular distance
// and m = number of leaves.
//
// Intended use: outer evolution selects the quadtree (via its threshold T),
// so trianglesQuadFunction is constructed per-individual from a QuadDecomp
// computed for that individual's T. No cross-individual caching of triangles
// (caching is trickier when the leaf table changes); the experiment measures
// whether the decomposition is learnable, not peak throughput.
type trianglesQuadFunction struct {
	target    pixelData
	targetN   pixelDataN
	blockSize int

	decomp    QuadDecomp
	mode      string    // "cdf" or "raw"
	perLeaf   []float64 // scratch — length len(decomp.Leaves)+1 (last = blank-frac obj)
	triangulation *incrdelaunay.Delaunay
	Base          *incrdelaunay.Delaunay
}

func (t *trianglesQuadFunction) Calculate(data PointsData) float64 {
	points := data.Points
	w, h := t.target.Size()

	if t.triangulation == nil {
		t.triangulation = incrdelaunay.NewDelaunay(w, h)
		for _, p := range points {
			t.triangulation.Insert(createPoint(p.X, p.Y, w, h))
		}
	} else if t.Base != nil {
		t.triangulation.Set(t.Base)
		for _, m := range data.Mutations {
			t.triangulation.Remove(createPoint(m.Old.X, m.Old.Y, w, h))
		}
		for _, m := range data.Mutations {
			t.triangulation.Insert(createPoint(m.New.X, m.New.Y, w, h))
		}
	}
	t.Base = nil

	nLeaves := len(t.decomp.Leaves)
	if cap(t.perLeaf) < nLeaves {
		t.perLeaf = make([]float64, nLeaves)
	} else {
		t.perLeaf = t.perLeaf[:nLeaves]
		for i := range t.perLeaf {
			t.perLeaf[i] = 0
		}
	}

	pixels := t.target.pixels
	area := 0.0

	// Per triangle: accumulate (n, sum_sq_r+g+b+sq) per leaf touched, then
	// close out variance per leaf at end. Use a small scratch table keyed by
	// leaf index for the visited set, since most triangles touch few leaves.
	visited := make([]int32, 0, 32)
	// Dense per-leaf accumulators (reused across triangles; cleared via visited list).
	type leafAcc struct {
		n, sR, sG, sB int
		sSq           int
	}
	accs := make([]leafAcc, nLeaves)

	t.triangulation.IterTriangles(func(triangle incrdelaunay.Triangle) {
		a := triangle.A
		b := triangle.B
		c := triangle.C

		area += math.Abs(0.5 * (float64(b.X-a.X)*float64(c.Y-a.Y) - float64(c.X-a.X)*float64(b.Y-a.Y)))

		// Reset visited accumulators from the previous triangle.
		for _, idx := range visited {
			accs[idx] = leafAcc{}
		}
		visited = visited[:0]

		tri := geom.NewTriangle(int(a.X), int(a.Y), int(b.X), int(b.Y), int(c.X), int(c.Y))
		rasterize.DDATriangle(tri, func(x, y int) {
			if x < 0 || y < 0 || x >= w || y >= h {
				return
			}
			leaf := t.decomp.LeafOf[y*w+x]
			acc := &accs[leaf]
			if acc.n == 0 {
				visited = append(visited, leaf)
			}
			pixel := pixels[y][x]
			acc.sR += int(pixel.r)
			acc.sG += int(pixel.g)
			acc.sB += int(pixel.b)
			acc.sSq += int(pixel.sq)
			acc.n++
		})

		// For each leaf this triangle touched, add its Welford-variance
		// contribution to that leaf's running total.
		for _, idx := range visited {
			a := accs[idx]
			if a.n == 0 {
				continue
			}
			diff := float64(a.sSq) - float64(a.sR*a.sR+a.sG*a.sG+a.sB*a.sB)/float64(a.n)
			t.perLeaf[idx] += diff
		}
	})

	// Normalise each leaf's accumulated variance by its cap to get objectives in [0,1].
	// Include blank-fraction as one more objective so coverage still matters.
	blankFrac := (float64(w*h) - area) / float64(w*h)
	if blankFrac < 0 {
		blankFrac = 0
	}
	if blankFrac > 1 {
		blankFrac = 1
	}
	objectives := make([]float64, nLeaves+1)
	for i := 0; i < nLeaves; i++ {
		cap := t.decomp.Leaves[i].Cap
		if cap <= 0 {
			objectives[i] = 0
			continue
		}
		v := t.perLeaf[i] / cap
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		objectives[i] = v
	}
	objectives[nLeaves] = blankFrac

	theta := HFFSingleTrueNorth(objectives)
	m := len(objectives)

	// Fitness modes:
	//   - logcdfpm: fitness = −log_CDF(theta, m) / m. Per-objective rate;
	//     removes the linear-in-m bias of log_cdf in the small-theta regime,
	//     so evolution of T isn't automatically biased toward maximum m.
	//   - logcdf:   fitness = −log_CDF(theta, m). Scales as ~m, so rewards
	//     large m mechanically.
	//   - cdf:      fitness = 1 − CDF(theta, m); collapses to ~1.0 at large m.
	//   - raw:      fitness = 1 − theta/pi; preserves gradient but NOT
	//     comparable across different m.
	switch t.mode {
	case "logcdfpm":
		return -HFFLogCDF(theta, m) / float64(m)
	case "logcdf":
		return -HFFLogCDF(theta, m)
	case "cdf":
		return 1.0 - HFFCDF(theta, m)
	case "raw":
		return 1.0 - theta/math.Pi
	default:
		return -HFFLogCDF(theta, m) / float64(m)
	}
}

// implements CacheFunction interface minimally — no cross-call caching.
func (t *trianglesQuadFunction) SetBase(other CacheFunction) {
	if o, ok := other.(*trianglesQuadFunction); ok {
		t.Base = o.triangulation
	}
}
func (t *trianglesQuadFunction) Cache() []CacheData     { return nil }
func (t *trianglesQuadFunction) SetCache(c []CacheData) {}

// PrecomputedPixels bundles the per-pixel and per-block target tables so that
// many evaluators (one per population slot, or one per candidate T) can share
// the work of reading the image.
type PrecomputedPixels struct {
	P  pixelData
	PN pixelDataN
}

// NewPrecomputedPixels reads the target image into the formats the existing
// rasterisation pipeline expects.
func NewPrecomputedPixels(target image.Data, blockSize int) PrecomputedPixels {
	return PrecomputedPixels{
		P:  fromImage(target),
		PN: fromImageN(target, blockSize),
	}
}

// NewTrianglesQuadFunction constructs one evaluator against a fixed QuadDecomp.
// Callers evolving T build a fresh QuadDecomp per candidate and pass it here.
// mode = "logcdfpm" (default; log_CDF / m, removes m-linear bias in left tail) |
// "logcdf" (−log_CDF, biased toward large m in left tail) | "cdf" (f64 CDF,
// underflows in left tail) | "raw" (−theta, not cross-m comparable).
func NewTrianglesQuadFunction(pix PrecomputedPixels, blockSize int, decomp QuadDecomp, mode string) CacheFunction {
	switch mode {
	case "cdf", "raw", "logcdf", "logcdfpm":
	default:
		mode = "logcdfpm"
	}
	return &trianglesQuadFunction{
		target:    pix.P,
		targetN:   pix.PN,
		blockSize: blockSize,
		decomp:    decomp,
		mode:      mode,
	}
}
