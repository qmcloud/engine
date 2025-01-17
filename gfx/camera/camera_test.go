// Copyright 2014 The Azul3D Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package camera

import (
	"image"

	"github.com/qmcloud/engine/gfx"
)

// A check for whether or not *camera.Camera implements gfx.Camera properly.
var _ gfx.Camera = New(image.Rectangle{})
