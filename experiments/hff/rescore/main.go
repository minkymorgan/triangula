// rescore: compute per-image PSNR/MSE between every rendered PNG in an output
// directory and its corresponding input PNG. Emits a CSV ranking all runs by
// the same yardstick regardless of what fitness drove training.
//
// Input filenames must follow the existing convention:
//   scalar_{img}[-s{seed}]_pts{N}_g{gens}.png
//   HFF_{img}[-s{seed}]_grid{WxH}_pts{N}_g{gens}_{method}[_sal].png
//
// Output: scores_rgb.csv (image, mode, config, seed, points, gens, mse, psnr_db, outfile)
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	scalarRe = regexp.MustCompile(`^scalar_([a-zA-Z0-9]+)(?:-s(\d+))?_pts(\d+)_g(\d+)\.png$`)
	hffRe    = regexp.MustCompile(`^HFF_([a-zA-Z0-9]+)(?:-s(\d+))?_grid(\d+)x(\d+)_pts(\d+)_g(\d+)_(.+)\.png$`)
)

type record struct {
	file   string
	image  string
	mode   string
	config string
	seed   int
	points int
	gens   int
	mse    float64
	psnr   float64
}

func main() {
	inputsDir := flag.String("inputs", "inputs", "directory containing the source PNGs")
	outputsDir := flag.String("outputs", "output", "directory with rendered PNGs")
	csvPath := flag.String("csv", "output/scores_rgb.csv", "output CSV path")
	flag.Parse()

	entries, err := os.ReadDir(*outputsDir)
	if err != nil {
		log.Fatal(err)
	}

	targetCache := map[string][][3]uint8{}
	targetSize := map[string][2]int{}

	var recs []record
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".png") {
			continue
		}
		r, ok := parseName(e.Name())
		if !ok {
			continue
		}
		// Load target.
		targetPath := filepath.Join(*inputsDir, r.image+".png")
		tKey := r.image
		if _, have := targetCache[tKey]; !have {
			px, w, h, err := loadPNG(targetPath)
			if err != nil {
				log.Printf("skip %s: cannot load target %s: %v", e.Name(), targetPath, err)
				continue
			}
			targetCache[tKey] = px
			targetSize[tKey] = [2]int{w, h}
		}
		// Load rendered.
		rPath := filepath.Join(*outputsDir, e.Name())
		rendered, rw, rh, err := loadPNG(rPath)
		if err != nil {
			log.Printf("skip %s: cannot load render: %v", e.Name(), err)
			continue
		}
		tw, th := targetSize[tKey][0], targetSize[tKey][1]
		if rw != tw || rh != th {
			log.Printf("skip %s: size mismatch (target %dx%d, render %dx%d)", e.Name(), tw, th, rw, rh)
			continue
		}
		r.mse, r.psnr = mseAndPSNR(targetCache[tKey], rendered)
		r.file = e.Name()
		recs = append(recs, r)
	}

	// Sort: image, points, gens, mse asc
	sort.Slice(recs, func(i, j int) bool {
		if recs[i].image != recs[j].image {
			return recs[i].image < recs[j].image
		}
		if recs[i].points != recs[j].points {
			return recs[i].points < recs[j].points
		}
		if recs[i].gens != recs[j].gens {
			return recs[i].gens < recs[j].gens
		}
		return recs[i].mse < recs[j].mse
	})

	f, err := os.Create(*csvPath)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{"image", "mode", "config", "seed", "points", "generations", "mse", "psnr_db", "file"})
	for _, r := range recs {
		_ = w.Write([]string{
			r.image, r.mode, r.config,
			strconv.Itoa(r.seed), strconv.Itoa(r.points), strconv.Itoa(r.gens),
			fmt.Sprintf("%.6f", r.mse), fmt.Sprintf("%.4f", r.psnr), r.file,
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		log.Fatal(err)
	}

	// Print a human-readable ranking.
	fmt.Println()
	fmt.Println("=== Ranked by PSNR (higher is better) — grouped by image+points ===")
	fmt.Println()
	var lastKey string
	for _, r := range recs {
		key := fmt.Sprintf("%s pts=%d", r.image, r.points)
		if key != lastKey {
			if lastKey != "" {
				fmt.Println()
			}
			fmt.Printf("## %s\n", key)
			lastKey = key
		}
		fmt.Printf("  %6s  gens=%6d  seed=%3d  MSE=%9.4f  PSNR=%6.2f dB  [%s]\n",
			r.mode, r.gens, r.seed, r.mse, r.psnr, r.config)
	}
}

func parseName(fn string) (record, bool) {
	if m := scalarRe.FindStringSubmatch(fn); m != nil {
		seed := 42
		if m[2] != "" {
			seed, _ = strconv.Atoi(m[2])
		}
		pts, _ := strconv.Atoi(m[3])
		gens, _ := strconv.Atoi(m[4])
		return record{image: m[1], mode: "scalar", config: "scalar", seed: seed, points: pts, gens: gens}, true
	}
	if m := hffRe.FindStringSubmatch(fn); m != nil {
		seed := 42
		if m[2] != "" {
			seed, _ = strconv.Atoi(m[2])
		}
		gx, _ := strconv.Atoi(m[3])
		gy, _ := strconv.Atoi(m[4])
		pts, _ := strconv.Atoi(m[5])
		gens, _ := strconv.Atoi(m[6])
		cfg := fmt.Sprintf("hff_%dx%d_%s", gx, gy, m[7])
		return record{image: m[1], mode: "hff", config: cfg, seed: seed, points: pts, gens: gens}, true
	}
	return record{}, false
}

func loadPNG(path string) ([][3]uint8, int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		// Try generic decode as a fallback.
		f.Seek(0, 0)
		img2, _, err2 := image.Decode(f)
		if err2 != nil {
			return nil, 0, 0, err
		}
		img = img2
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	px := make([][3]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b2, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			px[y*w+x] = [3]uint8{
				uint8(r >> 8),
				uint8(g >> 8),
				uint8(b2 >> 8),
			}
		}
	}
	return px, w, h, nil
}

func mseAndPSNR(a, b [][3]uint8) (float64, float64) {
	if len(a) != len(b) || len(a) == 0 {
		return math.NaN(), math.NaN()
	}
	var sumSq float64
	for i := range a {
		for c := 0; c < 3; c++ {
			d := float64(int(a[i][c]) - int(b[i][c]))
			sumSq += d * d
		}
	}
	mse := sumSq / float64(len(a)*3)
	if mse <= 0 {
		return 0, math.Inf(1)
	}
	psnr := 10 * math.Log10(255*255/mse)
	return mse, psnr
}
