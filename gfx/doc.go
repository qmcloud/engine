// Copyright 2014 The Azul3D Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package gfx provides generic interfaces to GPU-based rendering techniques.
//
// This package is not useful by itself but instead part of a larger picture as
// this package provides generic interfaces and data types to modern graphics
// rendering API's such as OpenGL, OpenGL ES, WebGL, Direct3D, etc.
//
// The coordinate system used by this package is the right-handed Z up
// coordinate system unless explicitly specified otherwise.
//
// Texture coordinates do not follow OpenGL convention but rather Go convention
// where the origin (0, 0) is the top-left corner of the texture.
//
// # Examples
//
// The examples repository contains several examples which utilize the gfx core
// packages. Please see:
//
// https://azul3d.org/examples
package gfx // import "github.com/qmcloud/engine/gfx"
