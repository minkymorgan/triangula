package fitness

import (
	"github.com/RH12503/Triangula/image"
)

// QuadLeaf is one cell of a variance-subdivided decomposition of the target
// image. The leaf owns a rectangular region [x0, x1) x [y0, y1).
type QuadLeaf struct {
	X0, Y0, X1, Y1 int
	Pixels         int
	Cap            float64 // pixels * maxPixelDifference, used to normalise variance -> [0,1]
}

// QuadDecomp is the precomputed leaf table plus a per-pixel leaf index image.
// With LeafOf[y*W + x] a triangle's pixel iteration can dispatch into the
// correct leaf in O(1).
type QuadDecomp struct {
	W, H   int
	Leaves []QuadLeaf
	LeafOf []int32 // length W*H; leaf index for each pixel
}

// BuildQuadtreeT builds a variance-threshold quadtree from target image and
// threshold T (in luminance-squared-variance units, 0..255^2). A cell is
// split while its per-pixel luminance variance exceeds T AND its size > min.
// maxLeaves caps depth so runaway subdivisions can't blow up dimensions.
//
// T maps monotonically: small T -> many leaves, large T -> few leaves.
// maxLeaves clamps the outer cap.
func BuildQuadtreeT(target image.Data, T float64, minCell, maxLeaves int) QuadDecomp {
	w, h := target.Size()

	// Per-pixel luminance (Rec.601 Y), 0..255 range.
	lum := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := target.RGBAt(x, y)
			lum[y*w+x] = (0.299*c.R + 0.587*c.G + 0.114*c.B) * 255.0
		}
	}

	// Region variance via summed-area tables of lum and lum^2.
	satL := make([]float64, (w+1)*(h+1))
	satL2 := make([]float64, (w+1)*(h+1))
	for y := 1; y <= h; y++ {
		rowL := 0.0
		rowL2 := 0.0
		for x := 1; x <= w; x++ {
			v := lum[(y-1)*w+(x-1)]
			rowL += v
			rowL2 += v * v
			satL[y*(w+1)+x] = satL[(y-1)*(w+1)+x] + rowL
			satL2[y*(w+1)+x] = satL2[(y-1)*(w+1)+x] + rowL2
		}
	}
	sum := func(sat []float64, x0, y0, x1, y1 int) float64 {
		return sat[y1*(w+1)+x1] - sat[y0*(w+1)+x1] - sat[y1*(w+1)+x0] + sat[y0*(w+1)+x0]
	}
	variance := func(x0, y0, x1, y1 int) (float64, int) {
		n := (x1 - x0) * (y1 - y0)
		if n <= 0 {
			return 0, 0
		}
		s := sum(satL, x0, y0, x1, y1)
		sq := sum(satL2, x0, y0, x1, y1)
		mean := s / float64(n)
		v := sq/float64(n) - mean*mean
		if v < 0 {
			v = 0
		}
		return v, n
	}

	var leaves []QuadLeaf
	// Iterative stack to avoid Go recursion overhead and keep control over
	// the maxLeaves cap across sibling splits.
	type region struct{ x0, y0, x1, y1 int }
	stack := []region{{0, 0, w, h}}

	for len(stack) > 0 {
		// Pop last.
		r := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		v, n := variance(r.x0, r.y0, r.x1, r.y1)
		minSide := r.x1 - r.x0
		if (r.y1 - r.y0) < minSide {
			minSide = r.y1 - r.y0
		}

		// Leaf conditions:
		//  - variance below threshold, OR
		//  - cell too small to split further, OR
		//  - already at the leaf budget (remaining regions collapse into leaves)
		if v <= T || minSide <= minCell || len(leaves)+len(stack)+1 >= maxLeaves {
			leaves = append(leaves, QuadLeaf{
				X0: r.x0, Y0: r.y0, X1: r.x1, Y1: r.y1,
				Pixels: n,
				Cap:    float64(n) * float64(maxPixelDifference),
			})
			continue
		}

		// Split into 4 quadrants.
		xm := (r.x0 + r.x1) / 2
		ym := (r.y0 + r.y1) / 2
		stack = append(stack,
			region{r.x0, r.y0, xm, ym},
			region{xm, r.y0, r.x1, ym},
			region{r.x0, ym, xm, r.y1},
			region{xm, ym, r.x1, r.y1},
		)
	}

	// Pixel->leaf index map.
	leafOf := make([]int32, w*h)
	for i, L := range leaves {
		idx := int32(i)
		for y := L.Y0; y < L.Y1; y++ {
			base := y * w
			for x := L.X0; x < L.X1; x++ {
				leafOf[base+x] = idx
			}
		}
	}

	return QuadDecomp{
		W:      w,
		H:      h,
		Leaves: leaves,
		LeafOf: leafOf,
	}
}
