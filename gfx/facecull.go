// Copyright 2014 The Azul3D Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gfx

// FaceCullMode represents a single face culling mode. BackFaceCulling is the
// default (zero value).
type FaceCullMode uint8

const (
	// BackFaceCulling is a face culling mode for culling back faces only (i.e.
	// only the front side will be drawn).
	BackFaceCulling FaceCullMode = iota

	// FrontFaceCulling is a face culling mode for culling front faces only
	// (i.e. only the back side is drawn).
	FrontFaceCulling

	// NoFaceCulling is a face culling mode for culling no faces at all (i.e.
	// both sides will be drawn).
	NoFaceCulling
)
