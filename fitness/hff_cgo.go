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

// HFFCDF wraps the Beta-CDF correction exposed by libhff_core. Takes a raw
// angular distance theta (radians) and the objective count m, returns a
// dimension-invariant percentile in [0, 1]. Required when individuals in the
// same population have different objective counts (e.g. evolved-T quadtree).
//
// NOTE: for large m and small theta the f64 result underflows to 0. Use
// HFFLogCDF in that regime.
func HFFCDF(theta float64, m int) float64 {
	return float64(C.hff_cdf_correction(C.double(theta), C.size_t(m)))
}

// HFFLogCDF returns log(CDF) directly — always finite (range ~[-1e5, 0]),
// so fitness values can be compared across m in the underflow regime.
func HFFLogCDF(theta float64, m int) float64 {
	return float64(C.hff_log_cdf_correction(C.double(theta), C.size_t(m)))
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
