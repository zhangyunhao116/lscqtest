// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build race

package lscq

import (
	"unsafe"
)

const raceEnabled = true

//go:linkname raceDisable runtime.RaceDisable
func raceDisable()

//go:linkname raceEnable runtime.RaceEnable
func raceEnable()

//go:linkname raceReleaseMerge runtime.RaceReleaseMerge
func raceReleaseMerge(addr unsafe.Pointer)

//go:linkname raceAcquire runtime.RaceAcquire
func raceAcquire(addr unsafe.Pointer)
