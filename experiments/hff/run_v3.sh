#!/usr/bin/env bash
# v3: scale-up. Best v2 config (HFF 8x8 TrueNorth + salience) vs scalar at
# 1000 points, 25k generations. Tests whether HFF's advantage persists or
# grows at larger triangle budgets and longer convergence horizons.
set -euo pipefail
cd "$(dirname "$0")"

BIN=/tmp/hff-exp
GENS="${1:-25000}"
PTS="${2:-1000}"
POP=400

(cd ../.. && go build -o "$BIN" ./experiments/hff)
mkdir -p output

for img in dog elon obama; do
  echo "=== ${img} pts=${PTS} scalar (${GENS} gens) ==="
  "$BIN" -in "inputs/${img}.png" -out output -tag "${img}" \
    -mode scalar -points "${PTS}" -gens "${GENS}" -pop "${POP}" -seed 42 \
    2>&1 | tail -2

  echo "=== ${img} pts=${PTS} hff 8x8 TN+sal (${GENS} gens) ==="
  "$BIN" -in "inputs/${img}.png" -out output -tag "${img}" \
    -mode hff -points "${PTS}" -gens "${GENS}" -pop "${POP}" \
    -cellsX 8 -cellsY 8 -method truenorth -salience -seed 42 \
    2>&1 | tail -2

  echo "=== ${img} pts=${PTS} hff 16x16 TN+sal (${GENS} gens) ==="
  "$BIN" -in "inputs/${img}.png" -out output -tag "${img}" \
    -mode hff -points "${PTS}" -gens "${GENS}" -pop "${POP}" \
    -cellsX 16 -cellsY 16 -method truenorth -salience -seed 42 \
    2>&1 | tail -2
done

echo "=== done ==="
