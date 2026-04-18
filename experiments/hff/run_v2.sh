#!/usr/bin/env bash
# v2 experiments: TrueNorth HFF (the fix for v1 noise) + salience weighting.
# Compares against scalar baseline on matched budgets.
set -euo pipefail
cd "$(dirname "$0")"

BIN=/tmp/hff-exp
GENS="${1:-10000}"
POP=400

(cd ../.. && go build -o "$BIN" ./experiments/hff)

mkdir -p output

for img in dog elon obama; do
  for pts in 300 600; do
    echo "=== ${img} pts=${pts} scalar ==="
    "$BIN" -in "inputs/${img}.png" -out output -tag "${img}" \
      -mode scalar -points "${pts}" -gens "${GENS}" -pop "${POP}" -seed 42 \
      2>&1 | tail -2

    for grid in 8 16 32; do
      echo "=== ${img} pts=${pts} hff truenorth ${grid}x${grid} ==="
      "$BIN" -in "inputs/${img}.png" -out output -tag "${img}" \
        -mode hff -points "${pts}" -gens "${GENS}" -pop "${POP}" \
        -cellsX "${grid}" -cellsY "${grid}" -method truenorth -seed 42 \
        2>&1 | tail -2

      echo "=== ${img} pts=${pts} hff truenorth ${grid}x${grid} +salience ==="
      "$BIN" -in "inputs/${img}.png" -out output -tag "${img}" \
        -mode hff -points "${pts}" -gens "${GENS}" -pop "${POP}" \
        -cellsX "${grid}" -cellsY "${grid}" -method truenorth -salience -seed 42 \
        2>&1 | tail -2
    done
  done
done

echo "=== done ==="
