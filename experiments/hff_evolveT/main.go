// Evolve-T experiment: the quadtree variance threshold T is a gene, so each
// individual has its own objective-space decomposition. Selection uses
// CDF-corrected HFF-TrueNorth angular distance, which is what makes comparison
// across individuals with different objective counts meaningful.
//
// No PSNR anchor, no outer/inner loops. One population, joint genome
// (points, T), one fitness function (CDF-corrected angular, which IS the
// measure of visualisation quality given the cell decomposition).
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"image"
	imgcolor "image/color"
	"image/png"
	_ "image/jpeg"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/RH12503/Triangula/fitness"
	"github.com/RH12503/Triangula/generator"
	"github.com/RH12503/Triangula/geom"
	imageData "github.com/RH12503/Triangula/image"
	"github.com/RH12503/Triangula/normgeom"
	"github.com/RH12503/Triangula/rasterize"
	"github.com/RH12503/Triangula/render"
	"github.com/RH12503/Triangula/triangulation/incrdelaunay"
)

type individual struct {
	points   normgeom.NormPointGroup
	T        float64
	decomp   fitness.QuadDecomp
	evaluator fitness.CacheFunction
	fitness  float64 // higher = better (1 - CDF)
	m        int     // objective count (leaves + 1)
}

type config struct {
	inputPath   string
	outDir      string
	tag         string
	points      int
	population  int
	elites      int
	generations int
	tMin, tMax  float64
	tMutStd     float64
	pMutRate    float64
	pMutStd     float64
	minCell     int
	maxLeaves   int
	blockSize   int
	seed        int64
	fitnessMode string
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.inputPath, "in", "", "input image path")
	flag.StringVar(&cfg.outDir, "out", "output", "output directory")
	flag.StringVar(&cfg.tag, "tag", "run", "tag used in output filenames")
	flag.IntVar(&cfg.points, "points", 600, "number of triangulation points")
	flag.IntVar(&cfg.population, "pop", 60, "population size")
	flag.IntVar(&cfg.elites, "elites", 6, "elitism count")
	flag.IntVar(&cfg.generations, "gens", 2000, "generations to run")
	flag.Float64Var(&cfg.tMin, "tmin", 1.0, "min quadtree variance threshold")
	flag.Float64Var(&cfg.tMax, "tmax", 400.0, "max quadtree variance threshold")
	flag.Float64Var(&cfg.tMutStd, "tmut", 25.0, "gaussian sigma for T mutation")
	flag.Float64Var(&cfg.pMutRate, "pmutrate", 0.01, "per-point mutation rate")
	flag.Float64Var(&cfg.pMutStd, "pmutstd", 0.25, "gaussian sigma for point position mutation (in normalised units)")
	flag.IntVar(&cfg.minCell, "mincell", 4, "min cell side in pixels before forced leaf")
	flag.IntVar(&cfg.maxLeaves, "maxleaves", 4096, "hard cap on objective count")
	flag.IntVar(&cfg.blockSize, "block", 5, "rasterisation block size (unused for pixel-accurate leaf binning, but kept for api)")
	flag.Int64Var(&cfg.seed, "seed", 42, "rng seed")
	flag.StringVar(&cfg.fitnessMode, "fitmode", "cdf", "fitness: cdf | raw")
	flag.Parse()

	if cfg.inputPath == "" {
		log.Fatal("-in required")
	}
	if err := os.MkdirAll(cfg.outDir, 0o755); err != nil {
		log.Fatal(err)
	}
	rng := rand.New(rand.NewSource(cfg.seed))
	rand.Seed(cfg.seed) // some downstream generators read global

	// Load target.
	f, err := os.Open(cfg.inputPath)
	if err != nil {
		log.Fatal(err)
	}
	dec, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		log.Fatal(err)
	}
	target := imageData.ToData(dec)
	w, h := target.Size()

	// Share precomputed pixel tables across individuals.
	pix := fitness.NewPrecomputedPixels(target, cfg.blockSize)

	// Cache quadtree decompositions by rounded T so similar individuals share them.
	// Rounded to integer T for the cache key — T ranges in tens/hundreds, granularity
	// finer than that isn't worth caching.
	decompCache := map[int]fitness.QuadDecomp{}
	getDecomp := func(T float64) fitness.QuadDecomp {
		k := int(T + 0.5)
		if d, ok := decompCache[k]; ok {
			return d
		}
		d := fitness.BuildQuadtreeT(target, T, cfg.minCell, cfg.maxLeaves)
		decompCache[k] = d
		return d
	}

	// Init population.
	pop := make([]*individual, cfg.population)
	for i := range pop {
		pts := (generator.RandomGenerator{}).Generate(cfg.points)
		T := cfg.tMin + rng.Float64()*(cfg.tMax-cfg.tMin)
		pop[i] = &individual{points: pts, T: T}
	}

	evaluate := func(ind *individual) {
		ind.decomp = getDecomp(ind.T)
		ind.m = len(ind.decomp.Leaves) + 1
		ind.evaluator = fitness.NewTrianglesQuadFunction(pix, cfg.blockSize, ind.decomp, cfg.fitnessMode)
		ind.fitness = ind.evaluator.Calculate(fitness.PointsData{Points: ind.points})
	}

	for _, ind := range pop {
		evaluate(ind)
	}

	// CSV logging per generation.
	csvPath := filepath.Join(cfg.outDir, fmt.Sprintf("evolveT_%s_pts%d_g%d.csv", cfg.tag, cfg.points, cfg.generations))
	csvF, err := os.Create(csvPath)
	if err != nil {
		log.Fatal(err)
	}
	csvW := csv.NewWriter(csvF)
	_ = csvW.Write([]string{"gen", "best_fit", "best_T", "best_m", "mean_T", "stddev_T", "mean_m", "stddev_m", "elapsed_s"})

	start := time.Now()
	logEvery := cfg.generations / 40
	if logEvery < 1 {
		logEvery = 1
	}

	for g := 0; g < cfg.generations; g++ {
		// Sort descending by fitness.
		sort.Slice(pop, func(i, j int) bool { return pop[i].fitness > pop[j].fitness })

		// Stats.
		best := pop[0]
		var sumT, sumT2, sumM, sumM2 float64
		for _, ind := range pop {
			sumT += ind.T
			sumT2 += ind.T * ind.T
			sumM += float64(ind.m)
			sumM2 += float64(ind.m) * float64(ind.m)
		}
		N := float64(len(pop))
		meanT := sumT / N
		stdT := sqrt0(sumT2/N - meanT*meanT)
		meanM := sumM / N
		stdM := sqrt0(sumM2/N - meanM*meanM)

		if g%logEvery == 0 || g == cfg.generations-1 {
			fmt.Printf("[%s] gen=%5d best_fit=%.6f best_T=%.2f best_m=%4d  meanT=%.2f±%.2f  meanM=%.1f±%.1f  dt=%s\n",
				cfg.tag, g, best.fitness, best.T, best.m, meanT, stdT, meanM, stdM,
				time.Since(start).Round(time.Millisecond))
		}
		_ = csvW.Write([]string{
			strconv.Itoa(g),
			fmt.Sprintf("%.8f", best.fitness),
			fmt.Sprintf("%.4f", best.T),
			strconv.Itoa(best.m),
			fmt.Sprintf("%.4f", meanT),
			fmt.Sprintf("%.4f", stdT),
			fmt.Sprintf("%.4f", meanM),
			fmt.Sprintf("%.4f", stdM),
			fmt.Sprintf("%.2f", time.Since(start).Seconds()),
		})
		csvW.Flush()

		// Build next generation.
		next := make([]*individual, 0, cfg.population)
		// Carry elites.
		for i := 0; i < cfg.elites && i < len(pop); i++ {
			e := *pop[i]
			next = append(next, &e)
		}
		// Fill rest by mutating a tournament winner from the top half.
		topHalf := len(pop) / 2
		if topHalf < cfg.elites {
			topHalf = cfg.elites
		}
		for len(next) < cfg.population {
			parent := pop[rng.Intn(topHalf)]
			child := &individual{
				points: parent.points.Copy(),
				T:      parent.T,
			}
			// Mutate T with some probability.
			if rng.Float64() < 0.5 {
				child.T += rng.NormFloat64() * cfg.tMutStd
				if child.T < cfg.tMin {
					child.T = cfg.tMin
				}
				if child.T > cfg.tMax {
					child.T = cfg.tMax
				}
			}
			// Mutate points.
			for pi := range child.points {
				if rng.Float64() < cfg.pMutRate {
					child.points[pi].X += rng.NormFloat64() * cfg.pMutStd
					child.points[pi].Y += rng.NormFloat64() * cfg.pMutStd
					child.points[pi].X = clamp01(child.points[pi].X)
					child.points[pi].Y = clamp01(child.points[pi].Y)
				}
			}
			evaluate(child)
			next = append(next, child)
		}
		pop = next
	}
	csvF.Close()

	// Final render of best individual.
	sort.Slice(pop, func(i, j int) bool { return pop[i].fitness > pop[j].fitness })
	best := pop[0]
	fmt.Printf("[%s] DONE best_fit=%.6f best_T=%.2f best_m=%d wall=%s\n",
		cfg.tag, best.fitness, best.T, best.m, time.Since(start).Round(time.Second))

	tris := triangulate(best.points, w, h)
	coloured := render.TrianglesOnImage(tris, target)
	outPath := filepath.Join(cfg.outDir, fmt.Sprintf("HFF_evolveT_%s_pts%d_g%d_bestT%.1f_m%d.png",
		cfg.tag, cfg.points, cfg.generations, best.T, best.m))
	if err := rasterToPNG(coloured, w, h, outPath); err != nil {
		log.Fatal(err)
	}

	// Also dump the final-generation population summary so we can see T
	// distribution at convergence.
	popPath := filepath.Join(cfg.outDir, fmt.Sprintf("evolveT_%s_pts%d_g%d_final_pop.csv", cfg.tag, cfg.points, cfg.generations))
	pf, _ := os.Create(popPath)
	pw := csv.NewWriter(pf)
	_ = pw.Write([]string{"rank", "fitness", "T", "m"})
	for i, ind := range pop {
		_ = pw.Write([]string{strconv.Itoa(i), fmt.Sprintf("%.8f", ind.fitness),
			fmt.Sprintf("%.4f", ind.T), strconv.Itoa(ind.m)})
	}
	pw.Flush()
	pf.Close()

	fmt.Printf("[%s] wrote %s and %s\n", cfg.tag, outPath, csvPath)
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
func sqrt0(v float64) float64 {
	if v <= 0 {
		return 0
	}
	return math.Sqrt(v)
}

func triangulate(points normgeom.NormPointGroup, w, h int) []geom.Triangle {
	tri := incrdelaunay.NewDelaunay(w, h)
	for _, p := range points {
		x := int(p.X*float64(w-1) + 0.5)
		y := int(p.Y*float64(h-1) + 0.5)
		if x < 0 {
			x = 0
		}
		if y < 0 {
			y = 0
		}
		if x >= w {
			x = w - 1
		}
		if y >= h {
			y = h - 1
		}
		tri.Insert(incrdelaunay.Point{X: int16(x), Y: int16(y)})
	}
	var out []geom.Triangle
	tri.IterTriangles(func(t incrdelaunay.Triangle) {
		out = append(out, geom.NewTriangle(
			int(t.A.X), int(t.A.Y),
			int(t.B.X), int(t.B.Y),
			int(t.C.X), int(t.C.Y),
		))
	})
	return out
}

func rasterToPNG(coloured []render.TriangleData, w, h int, path string) error {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, imgcolor.RGBA{255, 255, 255, 255})
		}
	}
	for _, td := range coloured {
		r := uint8(clampUnit(td.Color.R) * 255)
		g := uint8(clampUnit(td.Color.G) * 255)
		b := uint8(clampUnit(td.Color.B) * 255)
		nt := td.Triangle
		tri := geom.NewTriangle(
			int(nt.Points[0].X*float64(w-1)+0.5), int(nt.Points[0].Y*float64(h-1)+0.5),
			int(nt.Points[1].X*float64(w-1)+0.5), int(nt.Points[1].Y*float64(h-1)+0.5),
			int(nt.Points[2].X*float64(w-1)+0.5), int(nt.Points[2].Y*float64(h-1)+0.5),
		)
		rasterize.DDATriangle(tri, func(x, y int) {
			if x >= 0 && x < w && y >= 0 && y < h {
				img.Set(x, y, imgcolor.RGBA{r, g, b, 255})
			}
		})
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func clampUnit(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
