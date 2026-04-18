package fitness

import (
	"math"

	"github.com/RH12503/Triangula/geom"
	"github.com/RH12503/Triangula/image"
	"github.com/RH12503/Triangula/rasterize"
	"github.com/RH12503/Triangula/triangulation/incrdelaunay"
)

// trianglesHFFFunction is the HFF multi-objective twin of trianglesImageFunction.
// It partitions the image into a CellsX x CellsY grid and emits one variance
// objective per cell, plus one coverage-deficit objective. The per-cell objectives
// are passed to HFF (angular distance to balanced north pole) to produce a
// single scalar fitness; the point count (n_objectives) is CellsX*CellsY+1.
//
// Objectives are normalised to [0,1] using a per-cell maximum = cell_w*cell_h *
// maxPixelDifference so that HFF can work without population stats. Each
// triangle is binned into exactly one cell by centroid.
type trianglesHFFFunction struct {
	target  pixelData
	targetN pixelDataN

	blockSize int

	cellsX, cellsY int
	cellW, cellH   int
	cellMax        []float64 // per-cell cap = cell_pixels * maxPixelDifference
	imgW, imgH     int

	// Cache: per-triangle, stores cell index + variance (so generations with
	// stable triangles skip rasterisation).
	TriangleCache []CacheData
	nextCache     []CacheData

	Triangulation *incrdelaunay.Delaunay
	Base          *incrdelaunay.Delaunay
}

// hffTriangleCacheData caches a triangle's cell index, area, and variance diff
// contribution so cache hits can replay into the per-cell buckets.
type hffTriangleCacheData struct {
	aX, aY int16
	bX, bY int16
	cX, cY int16
	cell   int32
	area   float64
	diff   float64
	hash   uint32
}

func (t hffTriangleCacheData) Data() float64 { return t.diff }
func (t hffTriangleCacheData) Equals(other CacheData) bool {
	o, ok := other.(*hffTriangleCacheData)
	if !ok {
		return false
	}
	return t.aX == o.aX && t.aY == o.aY && t.bX == o.bX && t.bY == o.bY && t.cX == o.cX && t.cY == o.cY
}
func (t hffTriangleCacheData) Hash() uint64 {
	x := int(t.aX) + int(t.bX) + int(t.cX)
	y := int(t.aY) + int(t.bY) + int(t.cY)
	return uint64((97+x)*97 + y)
}
func (t hffTriangleCacheData) CachedHash() uint32      { return t.hash }
func (t *hffTriangleCacheData) SetCachedHash(h uint32) { t.hash = h }

func (t *trianglesHFFFunction) Calculate(data PointsData) float64 {
	points := data.Points
	w, h := t.target.Size()

	if t.Triangulation == nil {
		t.Triangulation = incrdelaunay.NewDelaunay(w, h)
		for _, p := range points {
			t.Triangulation.Insert(createPoint(p.X, p.Y, w, h))
		}
	} else if t.Base != nil {
		t.Triangulation.Set(t.Base)
		for _, m := range data.Mutations {
			t.Triangulation.Remove(createPoint(m.Old.X, m.Old.Y, w, h))
		}
		for _, m := range data.Mutations {
			t.Triangulation.Insert(createPoint(m.New.X, m.New.Y, w, h))
		}
	}
	t.Base = nil
	t.nextCache = t.nextCache[:0]

	pixels := t.target.pixels
	pixelsN := t.targetN.pixels

	nCells := t.cellsX * t.cellsY
	// perCell[i] accumulates total pixel-variance contributions for cell i.
	perCell := make([]float64, nCells)

	cacheMask := uint64(len(t.TriangleCache)) - 1
	tris := t.TriangleCache
	area := 0.0

	t.Triangulation.IterTriangles(func(triangle incrdelaunay.Triangle) {
		a := triangle.A
		b := triangle.B
		c := triangle.C

		triArea := math.Abs(0.5 * (float64(b.X-a.X)*float64(c.Y-a.Y) - float64(c.X-a.X)*float64(b.Y-a.Y)))
		area += triArea

		triData := &hffTriangleCacheData{
			aX: a.X, aY: a.Y, bX: b.X, bY: b.Y, cX: c.X, cY: c.Y,
		}
		hash := triData.Hash()
		index0 := uint32(hash & cacheMask)

		entry := tris[index0]
		if entry != nil {
			if cached, ok := entry.(*hffTriangleCacheData); ok && cached.Equals(triData) {
				if int(cached.cell) >= 0 && int(cached.cell) < nCells {
					perCell[cached.cell] += cached.diff
				}
				t.nextCache = append(t.nextCache, cached)
				return
			}
		}

		var sR0, sG0, sB0 int
		var sSq int
		n := 0

		tri := geom.NewTriangle(int(a.X), int(a.Y), int(b.X), int(b.Y), int(c.X), int(c.Y))
		rasterize.DDATriangleBlocks(tri, t.blockSize, func(x0, x1, y int) {
			if y < 0 || y >= len(pixels) {
				return
			}
			row := pixels[y]
			if x0 >= 0 && x1 <= len(row) {
				for x := x0; x < x1; x++ {
					pixel := row[x]
					sR0 += int(pixel.r)
					sG0 += int(pixel.g)
					sB0 += int(pixel.b)
					sSq += int(pixel.sq)
				}
				n += x1 - x0
			}
		}, func(x, y int) {
			if y < 0 || y >= len(pixelsN) || x < 0 || x >= len(pixelsN[0]) {
				return
			}
			pixel := pixelsN[y][x]
			sR0 += int(pixel.r)
			sG0 += int(pixel.g)
			sB0 += int(pixel.b)
			sSq += int(pixel.sq)
			n += t.blockSize * t.blockSize
		})

		diff := 0.0
		if n != 0 {
			diff = float64(sSq) - float64(sR0*sR0+sG0*sG0+sB0*sB0)/float64(n)
		}

		cx := (int(a.X) + int(b.X) + int(c.X)) / 3
		cy := (int(a.Y) + int(b.Y) + int(c.Y)) / 3
		if cx < 0 {
			cx = 0
		}
		if cy < 0 {
			cy = 0
		}
		if cx >= t.imgW {
			cx = t.imgW - 1
		}
		if cy >= t.imgH {
			cy = t.imgH - 1
		}
		ci := cx / t.cellW
		cj := cy / t.cellH
		if ci >= t.cellsX {
			ci = t.cellsX - 1
		}
		if cj >= t.cellsY {
			cj = t.cellsY - 1
		}
		cellIdx := cj*t.cellsX + ci

		perCell[cellIdx] += diff

		triData.cell = int32(cellIdx)
		triData.area = triArea
		triData.diff = diff
		triData.SetCachedHash(index0)
		t.nextCache = append(t.nextCache, triData)
	})

	t.TriangleCache = t.nextCache

	// Build the objective vector: per-cell normalised variance + coverage deficit.
	// Normalise each cell by its cap so objectives live in [0,1].
	objectives := make([]float64, nCells+1)
	for i := 0; i < nCells; i++ {
		cap := t.cellMax[i]
		if cap <= 0 {
			objectives[i] = 0
			continue
		}
		v := perCell[i] / cap
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		objectives[i] = v
	}
	blankFrac := (float64(w*h) - area) / float64(w*h)
	if blankFrac < 0 {
		blankFrac = 0
	}
	if blankFrac > 1 {
		blankFrac = 1
	}
	objectives[nCells] = blankFrac

	theta := HFFSingle(objectives)
	// Algorithm maximises fitness; HFF outputs angular distance (lower is better).
	// Map to [0,1]-ish where higher is better: 1 - theta/pi.
	return 1.0 - theta/math.Pi
}

func (t *trianglesHFFFunction) SetBase(other CacheFunction) {
	if o, ok := other.(*trianglesHFFFunction); ok {
		t.Base = o.Triangulation
	}
}
func (t *trianglesHFFFunction) Cache() []CacheData        { return t.TriangleCache }
func (t *trianglesHFFFunction) SetCache(c []CacheData)    { t.TriangleCache = c }

// TrianglesHFFFunctions returns n fitness functions using HFF aggregation over
// a (cellsX x cellsY)+1 objective vector.
func TrianglesHFFFunctions(target image.Data, blockSize, cellsX, cellsY, n int) []CacheFunction {
	w, h := target.Size()
	cellW := w / cellsX
	cellH := h / cellsY
	if cellW < 1 {
		cellW = 1
	}
	if cellH < 1 {
		cellH = 1
	}
	nCells := cellsX * cellsY
	cellMax := make([]float64, nCells)
	for j := 0; j < cellsY; j++ {
		for i := 0; i < cellsX; i++ {
			w0 := i * cellW
			h0 := j * cellH
			w1 := w0 + cellW
			h1 := h0 + cellH
			if i == cellsX-1 {
				w1 = w
			}
			if j == cellsY-1 {
				h1 = h
			}
			cellMax[j*cellsX+i] = float64((w1 - w0) * (h1 - h0) * maxPixelDifference)
		}
	}

	pixels := fromImage(target)
	pixelsN := fromImageN(target, blockSize)

	funcs := make([]CacheFunction, n)
	for i := 0; i < n; i++ {
		f := &trianglesHFFFunction{
			target:        pixels,
			targetN:       pixelsN,
			blockSize:     blockSize,
			cellsX:        cellsX,
			cellsY:        cellsY,
			cellW:         cellW,
			cellH:         cellH,
			cellMax:       cellMax,
			imgW:          w,
			imgH:          h,
			TriangleCache: make([]CacheData, 1<<22),
		}
		funcs[i] = f
	}
	return funcs
}
