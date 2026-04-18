# HFF vs Scalar Fitness — Triangula Experiment

> **Important correction (v5)**: rounds v1–v4 below compared *training fitness*
> between scalar and HFF modes — but those are measured on **different
> yardsticks** (scalar = normalised pixel-MSE; HFF = angular distance on
> hypersphere). Both land in [0,1] with "higher is better" but are not
> directly comparable.
>
> The apples-to-apples rescore against the original input (pixel MSE / PSNR,
> and Triangula's own scalar fitness applied to every run's final point group)
> shows a different story: **Triangula's scalar fitness wins or ties on MSE /
> PSNR in almost every configuration**. See Round 5 below.


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

### Round 5 (v5) — apples-to-apples rescoring

Two independent rescores applied to every existing PNG render:

1. **PSNR / pixel-MSE** between the original input image and the rendered
   output (computed by `experiments/hff/rescore/`, output
   `output/scores_rgb.csv`).
2. **Triangula's scalar fitness** applied to every run's final point group
   regardless of training mode — computed in the runner and logged to
   `output/scores.csv`.

Both metrics agree. A sample from `output/rescore_ranking.txt`:

```
## dog pts=1000
  scalar  gens= 15000  seed=123  MSE= 549.0330  PSNR= 20.73 dB  [scalar]
  scalar  gens= 15000  seed=  1  MSE= 549.8789  PSNR= 20.73 dB  [scalar]
  scalar  gens= 15000  seed=  7  MSE= 554.5847  PSNR= 20.69 dB  [scalar]
     hff  gens= 15000  seed=  1  MSE= 558.0904  PSNR= 20.66 dB  [hff_8x8_truenorth_sal]
     hff  gens= 15000  seed=123  MSE= 558.1808  PSNR= 20.66 dB  [hff_8x8_truenorth_sal]
     hff  gens= 15000  seed=  7  MSE= 560.3389  PSNR= 20.65 dB  [hff_8x8_truenorth_sal]
  scalar  gens= 25000  seed= 42  MSE= 551.1262  PSNR= 20.72 dB  [scalar]
     hff  gens= 25000  seed= 42  MSE= 555.3466  PSNR= 20.69 dB  [hff_8x8_truenorth_sal]
```

**Corrected headline**: Triangula's scalar-fitness training wins or ties
scalar-score and PSNR across nearly every configuration tested. Typical
gap is ~0.05–0.35 dB PSNR — small, but real and consistent.

HFF's earlier "wins" were an artefact of training on its own angular
metric and reporting that number. On the apples-to-apples pixel-error
yardstick, scalar training is the better optimisation objective for
this task.

**The visual differences I initially described are therefore likely
confirmation bias** — at ~0.03 dB PSNR gap the renders are
indistinguishable in practice. Whether HFF *distributes* the same total
error differently (concentrated in flat areas, sparing face detail) is a
separate perceptual hypothesis that PSNR can't adjudicate — it would need
SSIM/LPIPS or a human forced-choice to test.

### What we actually learned (honest summary)

- HFF as a per-individual aggregator over per-cell variance objectives, at
  these budgets, does not beat MSE-minimising scalar fitness on MSE.
- BalancedNorth pole is actively wrong geometry for this task (v1).
- TrueNorth is the correct pole choice; behaves comparably to scalar but
  doesn't beat it.
- Salience weighting helps HFF vs. uniform-weight HFF but doesn't close
  the gap to scalar.
- Grid coarseness matters: 8×8 consistently beats 16×16 and 32×32 at
  these point budgets.

### Round 6 (v6) — evolve T as a gene (quadtree decomposition is learned)

Code: `experiments/hff_evolveT/`. The quadtree variance threshold `T` is
encoded as a gene alongside point positions. Each individual carries its own
`(points, T)` genome, builds its own quadtree decomposition (cached by
rounded T), produces its own objective vector, and is scored by HFF-TrueNorth
angular distance with CDF correction. Single evolutionary loop — no
outer/inner nesting, no PSNR anchor.

**Fixing the CDF underflow.** First run with `fitness = 1 − CDF(theta, m)`
saturated immediately at 1.000000 — every candidate's CDF underflowed f64
to exactly 0. In the image-reconstruction regime, theta values live in the
far-left whisker (theta ≈ 0.001 rad at m ≈ 2000) where
`exp(α·ln(x) + β·ln(1−x) − lnB(α,β))` drops well below `f64::MIN_POSITIVE`.
The CDF correction was designed for DTLZ/WFG where thetas span the bulk of
[0, π]; our regime is different.

Fix: added `log_cdf_beta_correction` in the HFF Rust core
(`higd::log_cdf_beta_correction`, exposed via `hff_log_cdf_correction` in
the C ABI, HFF branch `feat/c-api` commit `07c4521`). Keeps the prefactor
in log space and runs the standard Lentz continued fraction in linear
space; returns `ln(CDF)` directly, always representable in f64.

Probe at m=2000, theta=1° → raw CDF = 0 (underflow), log_cdf = −8096.5 —
discriminating.

**Second measurement: log_cdf is ~linear in m at fixed per-objective
quality.** Probe at V=0.001 across m ∈ {16, 64, …, 4096} shows
`log_cdf/m ≈ −6.56` (asymptotically). So `−log_cdf` as fitness rewards m
mechanically; tried `−log_cdf/m` to remove this — same m-bias at our
working precision, T still pins to minimum (max m).

Decision: T evolving to max m is not itself a failure — if the resulting
triangulation is better than scalar Triangula's on image quality, the
decomposition choice is incidental. Compared on PSNR.

**PSNR comparison, dog pts=600, seed=42:**

| Method                                  | Gens | PSNR (dB) |
|-----------------------------------------|-----:|----------:|
| scalar Triangula                        | 5000 | **20.54** |
| scalar Triangula                        |10000 | 20.53     |
| scalar Triangula                        |25000 | 20.72     |
| evolveT HFF-TN logcdfpm                 |  300 | 18.94     |
| evolveT HFF-TN logcdfpm                 | 5000 | 19.30     |
| evolveT HFF-TN raw theta                |  300 | 18.94     |
| evolveT HFF-TN logcdf (no /m)           |  300 | 18.82     |
| evolveT HFF-TN raw CDF (underflow)      |  100 | 13.87     |

At matched compute (5000 gens), evolveT HFF is ~1.24 dB worse than scalar.
The gap doesn't close.

### Honest conclusion across v1–v6

On pixel MSE / PSNR — the standard ground-truth metric for image
reconstruction — Triangula's scalar fitness beats every HFF variant we
tried:

| HFF variant                      | Best result vs scalar                    |
|----------------------------------|------------------------------------------|
| BalancedNorth fixed grid (v1)    | visibly noisier; wrong pole geometry     |
| TrueNorth fixed grid (v2–v4)     | ties or loses by 0.05–0.35 dB on PSNR    |
| Salience-weighted fixed grid     | narrows but does not overturn gap        |
| EvolveT raw CDF (v6)             | fitness degenerate (underflow)           |
| EvolveT log-CDF (v6)             | m-biased; ~1.24 dB below scalar @ 5k gens|

**What we learned:**

1. HFF's CDF correction has an f64 underflow problem outside the DTLZ/WFG
   benchmark regime. Fixed with a log-space variant pushed upstream — a
   useful contribution for any future many-objective problem in the
   left-tail regime, whether or not image triangulation is the right task.
2. BalancedNorth is the wrong pole for reconstruction; TrueNorth is
   correct but doesn't add value over direct scalar MSE here.
3. Variance-subdivided quadtrees give HFF sensible per-region objectives,
   and T evolves to the finest possible decomposition — consistent with
   "more degrees of freedom = better angular fitness". Doesn't translate
   to better PSNR.
4. The cleanest apples-to-apples benchmark is `experiments/hff/rescore/`:
   pixel MSE + PSNR against the input, no training-metric dependency.

**Where HFF likely still helps** (untested in this experiment): problems
where the user genuinely has multiple incommensurate objectives (not a
single MSE target) and needs one scalar selection rule over all of them.
That's the use case the GECCO 2026 poster targets (many-objective,
autonomous decision loops). Image triangulation, with a single MSE-like
ground truth, is effectively single-objective and scalar MSE is already
the right fitness.

### Future work (if this direction is continued)

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
