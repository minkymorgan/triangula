# HFF vs Scalar Fitness — Triangula Experiment

This directory contains side-by-side comparisons of Triangula's built-in
**scalar fitness** (sum of per-triangle pixel variance + blank-area penalty)
against **HFF many-objective fitness** (per-cell variance objectives aggregated
by angular distance on a unit hypersphere, via libhff_core).

## The hypothesis

> Splitting the single scalar fitness into many perceptually-grounded objectives
> and aggregating with HFF should — at a fixed triangle budget — produce visibly
> better images, because scalar fitness throws away structure the optimiser
> could have used.

## Three rounds of experiments

### Round 1 (v1) — BalancedNorth HFF
Output: `output/v1_balanced/`

Used HFF's default "BalancedNorth" pole: `(1/√m, …, 1/√m)`. Result: **visually
noisier than scalar.** Diagnosis: BalancedNorth rewards solutions where every
objective is equally good. For per-cell variance, that's "equal error across
regions" — which pulls triangle density into uniform background cells and
starves the visually-important face/detail regions. Wrong geometry for image
reconstruction.

### Round 2 (v2) — TrueNorth HFF (+/– salience)
Output: `output/` (current)

Switched to HFF's "TrueNorth" method: augments the objective space with an
energy dimension and uses pole `(0, …, 0, 1)`. The fitness reduces to
`acos(1 − Σxᵢ²/n)`, which is angular-geometry's analogue of "minimise total
squared error" — the *right* geometry for this task.

Also added optional **salience weighting**: per-cell multiplier derived from
target-image luminance variance, so high-detail cells (faces) get up to 1.5×
weight and flat cells get down to 0.25×. The `_sal` suffix marks these.

Observed pattern (10k gens, pop=400, seed=42):

- HFF TrueNorth beats scalar on **every** image/point configuration tested.
- Salience weighting consistently improves HFF further.
- `8×8 + salience` or `16×16 + salience` is the sweet spot; `32×32` sometimes
  regresses (too many near-empty cells for 300-point budgets).
- Numerical gap at saturation is small (0.01–0.1% absolute fitness) but
  **visually clear** on faces — HFF versions show more detail around eyes,
  mouth, and edges; scalar versions are smoother but "over-averaged" in
  high-information regions.

Example (obama, pts=600, 10k gens):

| Config | Fitness |
|---|---:|
| scalar                           | 0.999268 |
| HFF 8×8 truenorth                | 0.999615 |
| **HFF 8×8 truenorth + salience** | **0.999656** |
| HFF 16×16 truenorth + salience   | 0.999602 |
| HFF 32×32 truenorth + salience   | 0.999617 |

### Round 3 (v3) — TBD
Ideas to try:
- Multi-scale salience (coarse + fine maps combined)
- Edge-alignment as an additional objective axis (Sobel residuals)
- Per-channel (R/G/B) variance objectives for richer colour geometry
- Larger triangle budgets (1000+ points) where HFF's many-objective structure should matter more

## Filename convention

- `scalar_{image}_pts{N}_g{gens}.png`
- `HFF_{image}_grid{WxH}_pts{N}_g{gens}_{method}[_sal].png`

where `method ∈ {balanced, truenorth}` and `_sal` denotes salience weighting.

## How to reproduce

```bash
# Build HFF with C ABI (one-time; see /Users/andrewmorgan/Dev/kaito/hff/)
cd ~/Dev/kaito/hff && cargo build --release --no-default-features --features c-api

# Run the experiment matrix
cd ~/Dev/gamakon/triangula/experiments/hff
./run_v2.sh 10000   # generations

# Or a single run
/tmp/hff-exp -in inputs/dog.png -out output \
  -tag dog -mode hff -points 600 -gens 10000 \
  -cellsX 16 -cellsY 16 -method truenorth -salience
```

## Architecture

```
┌──────────────────────────────────────────┐
│ Triangula (Go) — modifiedGenetic algo    │
│                                          │
│  fitness.TrianglesImageFunctions()       │  ← scalar baseline (unchanged)
│  fitness.TrianglesHFFFunctions()         │  ← new, this work
│                                          │
│        per-triangle variance             │
│              ↓                           │
│        bin by cell centroid              │
│              ↓                           │
│        N-dim objective vector            │
│              ↓                           │
│    HFFSingleTrueNorth() ← Go             │
│        or HFFSingle() ───┐               │
└──────────────────────────┼───────────────┘
                           ↓ cgo
             ┌─────────────────────────┐
             │ libhff_core.dylib (Rust)│
             │  hff_hf1_f64            │
             │  hff_hf1_enhanced       │
             │  hff_higd               │
             │  hff_angular_igd        │
             └─────────────────────────┘
```

The HFF Rust crate's new C ABI layer (feature `c-api`) exposes the same
mathematical surface as its pyo3 layer, but through `extern "C"` symbols
consumable by any FFI-capable language — this is how Go reaches it.
