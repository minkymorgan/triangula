#!/usr/bin/env bash
# Robustness check: does HFF's v3 advantage hold across seeds, or was seed=42
# lucky? Runs scalar vs HFF 8x8 TN+sal at pts=1000/25k gens on multiple seeds
# for the clearest signal image (dog).
set -euo pipefail
cd "$(dirname "$0")"

BIN=/tmp/hff-exp
GENS=15000
PTS=1000
POP=400

(cd ../.. && go build -o "$BIN" ./experiments/hff)
mkdir -p output

for seed in 1 7 123; do
  for img in dog obama; do
    echo "=== ${img} seed=${seed} scalar ==="
    "$BIN" -in "inputs/${img}.png" -out output -tag "${img}-s${seed}" \
      -mode scalar -points "${PTS}" -gens "${GENS}" -pop "${POP}" -seed "${seed}" \
      2>&1 | tail -2

    echo "=== ${img} seed=${seed} hff 8x8 TN+sal ==="
    "$BIN" -in "inputs/${img}.png" -out output -tag "${img}-s${seed}" \
      -mode hff -points "${PTS}" -gens "${GENS}" -pop "${POP}" \
      -cellsX 8 -cellsY 8 -method truenorth -salience -seed "${seed}" \
      2>&1 | tail -2
  done
done

echo "=== done ==="
