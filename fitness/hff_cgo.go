package fitness

/*
#cgo CFLAGS: -I/Users/andrewmorgan/Dev/kaito/hff/include
#cgo LDFLAGS: -L/Users/andrewmorgan/Dev/kaito/hff/target/release -lhff_core -Wl,-rpath,/Users/andrewmorgan/Dev/kaito/hff/target/release
#include <stdlib.h>
#include "hff.h"
*/
import "C"

import (
	"math"
	"unsafe"
)

// HFFSingle computes angular fitness of one individual's [0,1]-range objective
// vector against the balanced north pole. Lower is better.
func HFFSingle(objectives []float64) float64 {
	if len(objectives) == 0 {
		return 0
	}
	out := float64(0)
	rc := C.hff_hf1_f64(
		(*C.double)(unsafe.Pointer(&objectives[0])),
		C.size_t(1),
		C.size_t(len(objectives)),
		C.int32_t(0),
		(*C.double)(unsafe.Pointer(&out)),
	)
	if rc != 0 {
		return 3.141592653589793
	}
	return out
}

// HFFSingleTrueNorth computes the TrueNorth variant for a single individual
// without requiring population context. TrueNorth augments the objective
// space with an energy dimension and uses pole (0,...,0,1), so for a single
// individual the angular distance reduces to acos(energy_score), where
// energy_score = max(0, 1 - Σx²/n_obj). This behaves like a "minimise total
// squared error" fitness but with angular geometry — closer to what scalar
// fitness rewards than Balanced's "equal-error-across-objectives" pole.
//
// Caller supplies pre-normalised objectives in [0,1]. Lower output is better.
func HFFSingleTrueNorth(objectives []float64) float64 {
	if len(objectives) == 0 {
		return 0
	}
	var energy float64
	for _, v := range objectives {
		energy += v * v
	}
	n := float64(len(objectives))
	score := 1 - energy/n
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return math.Acos(score)
}
