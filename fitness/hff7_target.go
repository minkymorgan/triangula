package fitness

import (
	"math"

	"github.com/RH12503/Triangula/image"
)

// HFF7Target is the precomputed target-side data for the m=7 fitness:
//   - per-channel pixel values + per-channel squared values for exact
//     colour-variance accumulation
//   - luminance Sobel magnitude map (uint8, 0..255 after normalisation)
//     used to score wasted triangle edges
//   - per-channel high-pass target (pixel − 3×3-box-mean), used for
//     HF-residual variance (accumulated the same way as colour variance)
type HFF7Target struct {
	W, H int
	// Per-pixel channel values as float64 to avoid repeated uint→float conv.
	R, G, B []float64 // length W*H
	// Per-pixel squared values (for Welford-style variance inside a triangle).
	R2, G2, B2 []float64
	// Sobel magnitude on luminance; normalised to [0, 255].
	Sobel []uint8
	// High-pass per channel: HP_c = c − box3(c). Stored as float64.
	HPR, HPG, HPB    []float64
	HPR2, HPG2, HPB2 []float64
}

func (t HFF7Target) idx(x, y int) int { return y*t.W + x }

// BuildHFF7Target precomputes everything needed by the m=7 fitness from the
// target image. Runs once per experiment. Cheap — O(W*H).
func BuildHFF7Target(img image.Data) HFF7Target {
	w, h := img.Size()
	n := w * h
	t := HFF7Target{
		W: w, H: h,
		R: make([]float64, n), G: make([]float64, n), B: make([]float64, n),
		R2: make([]float64, n), G2: make([]float64, n), B2: make([]float64, n),
		Sobel: make([]uint8, n),
		HPR:   make([]float64, n), HPG: make([]float64, n), HPB: make([]float64, n),
		HPR2: make([]float64, n), HPG2: make([]float64, n), HPB2: make([]float64, n),
	}

	// Luminance for Sobel.
	lum := make([]float64, n)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := img.RGBAt(x, y)
			rv := c.R * 255
			gv := c.G * 255
			bv := c.B * 255
			i := t.idx(x, y)
			t.R[i] = rv
			t.G[i] = gv
			t.B[i] = bv
			t.R2[i] = rv * rv
			t.G2[i] = gv * gv
			t.B2[i] = bv * bv
			lum[i] = 0.299*rv + 0.587*gv + 0.114*bv
		}
	}

	// 3×3 Sobel on luminance. Zero padding at the border.
	// Gx = [-1 0 1; -2 0 2; -1 0 1], Gy = transpose.
	var sobelMax float64
	tmp := make([]float64, n)
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			tl := lum[(y-1)*w+(x-1)]
			tc := lum[(y-1)*w+x]
			tr := lum[(y-1)*w+(x+1)]
			ml := lum[y*w+(x-1)]
			mr := lum[y*w+(x+1)]
			bl := lum[(y+1)*w+(x-1)]
			bc := lum[(y+1)*w+x]
			br := lum[(y+1)*w+(x+1)]
			gx := -tl + tr - 2*ml + 2*mr - bl + br
			gy := -tl - 2*tc - tr + bl + 2*bc + br
			m := math.Sqrt(gx*gx + gy*gy)
			tmp[y*w+x] = m
			if m > sobelMax {
				sobelMax = m
			}
		}
	}
	// Normalise to [0,255]. sobelMax will be <= ~1444 (4*255); scale linearly.
	if sobelMax > 0 {
		for i, v := range tmp {
			s := v / sobelMax * 255.0
			if s < 0 {
				s = 0
			}
			if s > 255 {
				s = 255
			}
			t.Sobel[i] = uint8(s)
		}
	}

	// High-pass per channel: c - box3(c). Border pixels use asymmetric kernels
	// (only count available neighbours); produces HP_c at every pixel.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			y0 := y - 1
			y1 := y + 1
			if y0 < 0 {
				y0 = 0
			}
			if y1 >= h {
				y1 = h - 1
			}
			x0 := x - 1
			x1 := x + 1
			if x0 < 0 {
				x0 = 0
			}
			if x1 >= w {
				x1 = w - 1
			}
			var sR, sG, sB float64
			var count float64
			for yy := y0; yy <= y1; yy++ {
				for xx := x0; xx <= x1; xx++ {
					j := yy*w + xx
					sR += t.R[j]
					sG += t.G[j]
					sB += t.B[j]
					count++
				}
			}
			i := y*w + x
			hpr := t.R[i] - sR/count
			hpg := t.G[i] - sG/count
			hpb := t.B[i] - sB/count
			t.HPR[i] = hpr
			t.HPG[i] = hpg
			t.HPB[i] = hpb
			t.HPR2[i] = hpr * hpr
			t.HPG2[i] = hpg * hpg
			t.HPB2[i] = hpb * hpb
		}
	}

	return t
}
