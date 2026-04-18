#!/usr/bin/env bash
# Extended runs: give HFF more generations to converge, and add wider grid
# settings so we see how the multi-objective geometry behaves with more/fewer
# objectives. Scalar also gets a matched long run.
set -euo pipefail
cd "$(dirname "$0")"

BIN=/tmp/hff-exp
GENS_LONG=25000
POP=400

mkdir -p output

for img in dog elon obama; do
  for pts in 300 600; do
    # Long scalar — for matched-budget comparison
    echo "=== ${img} pts=${pts} scalar (25k) ==="
    "$BIN" -in "inputs/${img}.png" -out output \
      -tag "${img}" -mode scalar \
      -points "${pts}" -gens "${GENS_LONG}" -pop "${POP}" \
      -seed 42 2>&1 | tail -2

    # Long HFF 16x16 — primary HFF setting at extended budget
    echo "=== ${img} pts=${pts} hff 16x16 (25k) ==="
    "$BIN" -in "inputs/${img}.png" -out output \
      -tag "${img}-g16" -mode hff \
      -points "${pts}" -gens "${GENS_LONG}" -pop "${POP}" \
      -cellsX 16 -cellsY 16 -seed 42 2>&1 | tail -2

    # Long HFF 32x32 — high-objective-count variant (1025 objectives)
    echo "=== ${img} pts=${pts} hff 32x32 (25k) ==="
    "$BIN" -in "inputs/${img}.png" -out output \
      -tag "${img}-g32" -mode hff \
      -points "${pts}" -gens "${GENS_LONG}" -pop "${POP}" \
      -cellsX 32 -cellsY 32 -seed 42 2>&1 | tail -2
  done
done

echo "=== done ==="
