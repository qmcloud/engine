// Copyright 2014 The Azul3D Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gfxutil

import (
	"sort"

	"github.com/qmcloud/engine/gfx"
	"github.com/qmcloud/engine/lmath"
)

// ByDist sorts a list of graphics objects based on their distance away from
// a target position (typically the camera). As such if the sorted objects are
// drawn in order then they are drawn back-to-front (which is useful for
// rendering alpha-blended objects such that transparency appears correct).
//
// Using sort.Reverse this doubles as front-to-back sorting (which is useful
// for drawing opaque objects efficiently due to depth testing).
type ByDist struct {
	// The list of objects to sort.
	Objects []*gfx.Object

	// The target position to compare against. The list is sorted based off
	// each object's distance away from this position (typically this is the
	// camera's position).
	Target lmath.Vec3
}

// Len implements the sort interface.
func (b ByDist) Len() int {
	return len(b.Objects)
}

// Swap implements the sort interface.
func (b ByDist) Swap(i, j int) {
	b.Objects[i], b.Objects[j] = b.Objects[j], b.Objects[i]
}

// Less implements the sort interface.
func (b ByDist) Less(ii, jj int) bool {
	i := b.Objects[ii].Transform
	j := b.Objects[jj].Transform

	// Convert each position to world space.
	iPos := i.ConvertPos(i.Pos(), gfx.ParentToWorld)
	jPos := j.ConvertPos(j.Pos(), gfx.ParentToWorld)

	// Calculate the distance from each object to the target position.
	iDist := iPos.Sub(b.Target).LengthSq()
	jDist := jPos.Sub(b.Target).LengthSq()

	// If i is further away from j (greater value) then it should sort first.
	return iDist > jDist
}

// InsertionSort performs a simple insertion sort on the sort interface. In the
// case of ByDist it performs generally as fast as sort.Sort except that it can
// exploit temporal coherence improving performance dramatically when the
// objects have not moved much.
func InsertionSort(data sort.Interface) {
	for i := 0; i < data.Len(); i++ {
		for j := i; j > 0 && data.Less(j, j-1); j-- {
			data.Swap(j, j-1)
		}
	}
}

// ByState sorts a list of graphics objects based on the change of their
// graphics state in order to reduce graphics state changes and increase the
// overall throughput when rendering several objects whose graphics state
// differ.
type ByState []*gfx.Object

// Len implements the sort interface.
func (b ByState) Len() int {
	return len(b)
}

// Swap implements the sort interface.
func (b ByState) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

// Less implements the sort interface.
func (b ByState) Less(i, j int) bool {
	return b[i].Compare(b[j])
}
