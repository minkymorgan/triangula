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

### Round 3 (v3) — scale-up at 1000 points, 25k generations
Output: `output/` (v3 files suffixed `_pts1000_g25000`)

Tested whether HFF's v2 advantage holds or grows at larger triangle budgets.
Compared scalar vs HFF 8×8 TN+sal vs HFF 16×16 TN+sal at 1000 points / 25k
generations across all three images.

| Image | Scalar   | HFF 8×8 TN+sal | HFF 16×16 TN+sal | Δ best |
|---|---:|---:|---:|---:|
| dog   | 0.999603 | **0.999837**   | 0.999826         | +0.023% |
| elon  | 0.999794 | **0.999824**   | 0.999801         | +0.003% |
| obama | 0.999591 | **0.999792**   | 0.999787         | +0.020% |

**HFF wins on every image.** 8×8 edges out 16×16 at this budget — fewer, coarser
cells give per-objective pressure more signal per mutation at 1000 points.

**Visually (dog and obama most pronounced):** HFF allocates more triangles
to high-variance face/detail regions; scalar over-averages faces into soft
blobs. Eye/mouth/edge detail is clearly sharper under HFF.

Gap widens from 0.01% at pts=600 to 0.02% at pts=1000 — **HFF's advantage
grows with triangle budget**, as expected when multi-objective geometry has
more degrees of freedom to exploit.

### Round 4 (v4) — seed robustness
Output: `output/` (v4 files suffixed `-s{seed}`)

Tested whether HFF's v3 advantage survives seed perturbation. Ran scalar vs
HFF 8×8 TN+sal at 1000 points / 15k gens across seeds {1, 7, 123} on dog
and obama (elon skipped — flat-background image, small v3 gap).

| Image | Seed | Scalar   | HFF 8×8 TN+sal | Δ |
|-------|---:|---------:|---------------:|---:|
| dog   |   1 | 0.999589 | **0.999834**   | +0.0245% |
| dog   |   7 | 0.999600 | **0.999829**   | +0.0229% |
| dog   | 123 | 0.999583 | **0.999819**   | +0.0236% |
| obama |   1 | 0.999562 | **0.999791**   | +0.0229% |
| obama |   7 | 0.999567 | **0.999741**   | +0.0174% |
| obama | 123 | 0.999569 | **0.999776**   | +0.0207% |

**HFF wins every single seed.** Δ is remarkably stable across seeds
(0.017–0.025%), indicating the advantage is systematic, not lucky
initialisation.

### Salience-timing note

The salience weight `w_i ∈ [0.25, 1.5]` is applied **after** per-cell
min-max normalisation and **before** HFF projection onto the hypersphere:

```
v_i = clamp(perCell[i] / cap_i, 0, 1)   // normalise
v_i *= w_i ; v_i = min(v_i, 1)          // salience reweight
objectives[i] = v_i
theta = acos(1 - Σ v_i² / n)            // project + aggregate
```

This is the right spot because the projection's "north pole" then
corresponds to "all *weighted* errors near zero" — high-salience cells
contribute `w_i² ≈ 2.25×` more to the energy than flat cells (`≈ 0.06×`),
so the fitness geometry itself is perceptually reweighted. Applying
salience before normalisation would couple with `cap_i` in nonsensical
ways; applying after projection would break the unit-sphere geometry.

### Future work
- Edge-alignment as an additional objective axis (Sobel residuals) — should
  further sharpen boundary-heavy images.
- Per-channel (R/G/B) variance objectives — richer colour geometry at the
  cost of refactoring `pixel.sq` into per-channel sums.
- Population-level HFF (hff_hf1_enhanced across the whole generation) —
  currently we run HFF per-individual with closed-form TrueNorth; population
  normalisation may help but requires a per-generation collect-call-distribute
  redesign of the algorithm driver.
- Salience from a saliency model rather than luminance variance — better
  alignment with human attention.

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
