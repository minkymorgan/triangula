// Experiment: compare Triangula scalar fitness against HFF many-objective fitness.
//
// Runs the same genetic algorithm twice — once with Triangula's built-in scalar
// fitness, once with the HFF cell-grid multi-objective fitness piped through
// libhff_core.dylib via cgo — at matched point/population/generation budgets.
// Saves the reconstructed PNG for each mode into the output directory.
package main

import (
	"flag"
	"fmt"
	"image"
	imgcolor "image/color"
	"image/png"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	// PNG/JPEG registered via blank imports
	_ "image/jpeg"

	"github.com/RH12503/Triangula/algorithm"
	"github.com/RH12503/Triangula/algorithm/evaluator"
	"github.com/RH12503/Triangula/fitness"
	"github.com/RH12503/Triangula/generator"
	"github.com/RH12503/Triangula/geom"
	imageData "github.com/RH12503/Triangula/image"
	"github.com/RH12503/Triangula/mutation"
	"github.com/RH12503/Triangula/normgeom"
	"github.com/RH12503/Triangula/rasterize"
	"github.com/RH12503/Triangula/render"
	"github.com/RH12503/Triangula/triangulation/incrdelaunay"
)

type config struct {
	inputPath   string
	outDir      string
	tag         string
	mode        string
	points      int
	generations int
	population  int
	cutoff      int
	cellsX      int
	cellsY      int
	method      string
	salience    bool
	blockSize   int
	mutations   int
	variation   float64
	seed        int64
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.inputPath, "in", "", "input image path (PNG/JPEG)")
	flag.StringVar(&cfg.outDir, "out", "output", "output directory")
	flag.StringVar(&cfg.tag, "tag", "run", "tag used in output filenames")
	flag.StringVar(&cfg.mode, "mode", "scalar", "fitness mode: scalar | hff")
	flag.IntVar(&cfg.points, "points", 300, "number of triangulation points")
	flag.IntVar(&cfg.generations, "gens", 2000, "generations to run")
	flag.IntVar(&cfg.population, "pop", 400, "population size")
	flag.IntVar(&cfg.cutoff, "cutoff", 5, "cutoff")
	flag.IntVar(&cfg.cellsX, "cellsX", 16, "HFF grid cells (X)")
	flag.IntVar(&cfg.cellsY, "cellsY", 16, "HFF grid cells (Y)")
	flag.StringVar(&cfg.method, "method", "balanced", "HFF north-pole method: balanced | truenorth")
	flag.BoolVar(&cfg.salience, "salience", false, "weight HFF cells by target luminance variance")
	flag.IntVar(&cfg.blockSize, "block", 5, "rasterisation block size")
	flag.IntVar(&cfg.mutations, "mutations", 2, "mutations per step (not wired into gaussian mutator)")
	flag.Float64Var(&cfg.variation, "variation", 0.3, "gaussian variation")
	flag.Int64Var(&cfg.seed, "seed", 42, "random seed for point generation")
	flag.Parse()

	if cfg.inputPath == "" {
		log.Fatal("-in required")
	}
	if err := os.MkdirAll(cfg.outDir, 0o755); err != nil {
		log.Fatal(err)
	}

	rand.Seed(cfg.seed)

	imgFile, err := os.Open(cfg.inputPath)
	if err != nil {
		log.Fatal(err)
	}
	decoded, _, err := image.Decode(imgFile)
	imgFile.Close()
	if err != nil {
		log.Fatal(err)
	}

	tData := imageData.ToData(decoded)
	w, h := tData.Size()

	var evalFactory func(n int) evaluator.Evaluator
	switch cfg.mode {
	case "scalar":
		evalFactory = func(n int) evaluator.Evaluator {
			return evaluator.NewParallel(fitness.TrianglesImageFunctions(tData, cfg.blockSize, n), 22)
		}
	case "hff":
		var weights []float64
		if cfg.salience {
			weights = fitness.TargetSalienceWeights(tData, cfg.cellsX, cfg.cellsY)
		}
		evalFactory = func(n int) evaluator.Evaluator {
			return evaluator.NewParallel(
				fitness.TrianglesHFFFunctions(tData, cfg.blockSize, cfg.cellsX, cfg.cellsY, n, cfg.method, weights),
				22,
			)
		}
	default:
		log.Fatalf("unknown mode: %s", cfg.mode)
	}

	pointFactory := func() normgeom.NormPointGroup {
		return (generator.RandomGenerator{}).Generate(cfg.points)
	}

	mutator := mutation.NewGaussianMethod(float64(cfg.mutations)/float64(cfg.points), cfg.variation)

	start := time.Now()
	algo := algorithm.NewModifiedGenetic(pointFactory, cfg.population, cfg.cutoff, evalFactory, mutator)

	logEvery := cfg.generations / 20
	if logEvery < 1 {
		logEvery = 1
	}
	for g := 0; g < cfg.generations; g++ {
		algo.Step()
		if g%logEvery == 0 || g == cfg.generations-1 {
			st := algo.Stats()
			fmt.Printf("[%s %s] gen=%5d best=%.6f dt=%s total=%s\n",
				cfg.tag, cfg.mode, g, st.BestFitness, st.TimeForGen.Round(time.Millisecond),
				time.Since(start).Round(time.Second))
		}
	}

	best := algo.Best()
	tris := triangulate(best, w, h)
	coloured := render.TrianglesOnImage(tris, tData)

	// Apples-to-apples score: evaluate the final best point group with
	// Triangula's built-in scalar fitness, regardless of training mode.
	// This is THE comparable number across scalar/HFF/any-future-mode runs.
	scalarScore := scoreWithScalar(tData, cfg.blockSize, best)

	var outPath string
	var configLabel string
	if cfg.mode == "hff" {
		suffix := cfg.method
		if cfg.salience {
			suffix += "_sal"
		}
		outPath = filepath.Join(cfg.outDir, fmt.Sprintf("HFF_%s_grid%dx%d_pts%d_g%d_%s.png",
			cfg.tag, cfg.cellsX, cfg.cellsY, cfg.points, cfg.generations, suffix))
		configLabel = fmt.Sprintf("hff_grid%dx%d_%s", cfg.cellsX, cfg.cellsY, suffix)
	} else {
		outPath = filepath.Join(cfg.outDir, fmt.Sprintf("scalar_%s_pts%d_g%d.png",
			cfg.tag, cfg.points, cfg.generations))
		configLabel = "scalar"
	}
	if err := rasterToPNG(coloured, w, h, outPath); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("[%s %s] DONE training_fitness=%.6f scalar_score=%.6f wall=%s -> %s\n",
		cfg.tag, cfg.mode, algo.Stats().BestFitness, scalarScore,
		time.Since(start).Round(time.Second), outPath)

	// Append CSV row: image, config, points, gens, seed, training_fitness, scalar_score
	csvPath := filepath.Join(cfg.outDir, "scores.csv")
	_ = appendScoreRow(csvPath, cfg.tag, cfg.mode, configLabel, cfg.points, cfg.generations,
		cfg.seed, algo.Stats().BestFitness, scalarScore, filepath.Base(outPath))
}

// scoreWithScalar evaluates a point group using Triangula's built-in scalar
// fitness (trianglesImageFunction). This is the apples-to-apples metric: same
// ruler for every training mode.
func scoreWithScalar(tData imageData.Data, blockSize int, best normgeom.NormPointGroup) float64 {
	funcs := fitness.TrianglesImageFunctions(tData, blockSize, 1)
	f := funcs[0]
	return f.Calculate(fitness.PointsData{Points: best})
}

func appendScoreRow(path, image, mode, config string, pts, gens int, seed int64, trainFit, scalar float64, outBase string) error {
	isNew := false
	if _, err := os.Stat(path); os.IsNotExist(err) {
		isNew = true
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if isNew {
		fmt.Fprintln(f, "image,mode,config,points,generations,seed,training_fitness,scalar_score,output_file")
	}
	fmt.Fprintf(f, "%s,%s,%s,%d,%d,%d,%.8f,%.8f,%s\n",
		image, mode, config, pts, gens, seed, trainFit, scalar, outBase)
	return nil
}

// triangulate computes the Delaunay triangulation for a point group, returning
// geom.Triangle values in image-pixel space.
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

// rasterToPNG rasterises coloured triangles to a PNG.
func rasterToPNG(coloured []render.TriangleData, w, h int, path string) error {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// white background
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
