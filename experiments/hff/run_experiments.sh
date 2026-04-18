#!/usr/bin/env bash
# Side-by-side experiment: Triangula scalar vs HFF multi-objective, matched
# point/population/generation budgets. Outputs PNG renders to ./output/.
#
# Usage: ./run_experiments.sh [GENS]
set -euo pipefail
cd "$(dirname "$0")"

BIN=/tmp/hff-exp
GENS="${1:-3000}"
POP=400
CUTOFF=5
BLOCK=5

# Rebuild to pick up any fitness changes.
(cd ../.. && go build -o "$BIN" ./experiments/hff)

mkdir -p output

IMAGES=(dog elon obama)
POINTS=(300 600)

for img in "${IMAGES[@]}"; do
  for pts in "${POINTS[@]}"; do
    echo "=== ${img} pts=${pts} scalar ==="
    "$BIN" -in "inputs/${img}.png" -out output \
      -tag "${img}" -mode scalar \
      -points "${pts}" -gens "${GENS}" \
      -pop "${POP}" -cutoff "${CUTOFF}" -block "${BLOCK}" \
      -seed 42 2>&1 | tail -3

    echo "=== ${img} pts=${pts} hff 16x16 ==="
    "$BIN" -in "inputs/${img}.png" -out output \
      -tag "${img}-g16" -mode hff \
      -points "${pts}" -gens "${GENS}" \
      -pop "${POP}" -cutoff "${CUTOFF}" -block "${BLOCK}" \
      -cellsX 16 -cellsY 16 \
      -seed 42 2>&1 | tail -3

    echo "=== ${img} pts=${pts} hff 8x8 ==="
    "$BIN" -in "inputs/${img}.png" -out output \
      -tag "${img}-g8" -mode hff \
      -points "${pts}" -gens "${GENS}" \
      -pop "${POP}" -cutoff "${CUTOFF}" -block "${BLOCK}" \
      -cellsX 8 -cellsY 8 \
      -seed 42 2>&1 | tail -3
  done
done

echo
echo "=== done ==="
ls -la output/
