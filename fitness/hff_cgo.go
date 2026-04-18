package fitness

/*
#cgo CFLAGS: -I/Users/andrewmorgan/Dev/kaito/hff/include
#cgo LDFLAGS: -L/Users/andrewmorgan/Dev/kaito/hff/target/release -lhff_core -Wl,-rpath,/Users/andrewmorgan/Dev/kaito/hff/target/release
#include <stdlib.h>
#include "hff.h"
*/
import "C"

import (
	"unsafe"
)

// HFFSingle computes the angular fitness of one individual's (already
// in-[0,1]) objective vector against the balanced north pole, via
// hff_hf1_f64. Returns angular distance in radians [0, pi]; lower is better.
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
