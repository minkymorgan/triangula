#!/usr/bin/env bash
# Apples-to-apples rescore run: each configuration's final point group is
# evaluated with Triangula's built-in scalar fitness (regardless of training
# mode), written to output/scores.csv. This is THE comparable number across
# all modes.
set -euo pipefail
cd "$(dirname "$0")"

BIN=/tmp/hff-exp
GENS="${1:-25000}"
PTS="${2:-1000}"
POP=400

(cd ../.. && go build -o "$BIN" ./experiments/hff)
mkdir -p output

# Start a fresh scores file for this sweep (keeps old renders in place).
rm -f output/scores.csv

for img in dog elon obama; do
  for pts in 300 600 1000; do
    echo "=== ${img} pts=${pts} scalar ==="
    "$BIN" -in "inputs/${img}.png" -out output -tag "${img}" \
      -mode scalar -points "${pts}" -gens "${GENS}" -pop "${POP}" -seed 42 \
      2>&1 | tail -1

    for grid in 8 16; do
      echo "=== ${img} pts=${pts} hff ${grid}x${grid} TN+sal ==="
      "$BIN" -in "inputs/${img}.png" -out output -tag "${img}" \
        -mode hff -points "${pts}" -gens "${GENS}" -pop "${POP}" \
        -cellsX "${grid}" -cellsY "${grid}" -method truenorth -salience -seed 42 \
        2>&1 | tail -1
    done
  done
done

echo
echo "=== scores.csv ==="
cat output/scores.csv
