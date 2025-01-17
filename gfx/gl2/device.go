// Copyright 2014 The Azul3D Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gl2

import (
	"fmt"
	"image"
	"io"
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/qmcloud/engine/gfx"
	"github.com/qmcloud/engine/gfx/clock"
	"github.com/qmcloud/engine/gfx/internal/gl/2.0/gl"
	"github.com/qmcloud/engine/gfx/internal/glc"
	"github.com/qmcloud/engine/gfx/internal/glutil"
	"github.com/qmcloud/engine/gfx/internal/tag"
	"github.com/qmcloud/engine/gfx/internal/util"
)

type pendingQuery struct {
	// The ID of the pending occlusion query.
	id uint32

	// The object of the pending occlusion query.
	o *gfx.Object
}

// rsrcManager keeps a list of meshes, shaders, textures, FBO's, and
// renderbuffers that should be free'd at the next available time.
type rsrcManager struct {
	sync.RWMutex
	meshes        []*nativeMesh
	shaders       []*nativeShader
	textures      []uint32
	fbos          []uint32
	renderbuffers []uint32
}

// freePending free's all of the pending resources.
func (r *rsrcManager) freePending() {
	r.Lock()

	// Free the meshes.
	if tag.Gfxdebug && len(r.meshes) > 0 {
		log.Printf("gfx: free %d meshes\n", len(r.meshes))
	}
	for _, native := range r.meshes {
		native.free()
	}
	r.meshes = r.meshes[:0]

	// Free the shaders.
	if tag.Gfxdebug && len(r.shaders) > 0 {
		log.Printf("gfx: free %d shaders\n", len(r.shaders))
	}
	for _, native := range r.shaders {
		native.free()
	}
	r.shaders = r.shaders[:0]

	r.Unlock()

	r.freeTextures()
	r.freeFBOs()
	r.freeRenderbuffers()
}

// device implements the Device interface.
type device struct {
	*util.BaseCanvas
	warner        *util.Warner
	common        *glc.Context
	clock         *clock.Clock
	devInfo       gfx.DeviceInfo
	rsrcManager   *rsrcManager
	graphicsState *graphicsState

	// Render execution channel.
	renderExec chan func() bool

	// The other shared device to be used for loading assets, or nil.
	shared struct {
		sync.RWMutex
		*device
	}

	// Whether or not certain extensions we use are present or not.
	glArbDebugOutput, glArbMultisample, glArbFramebufferObject,
	glArbOcclusionQuery bool

	// Number of multisampling samples, buffers.
	samples, sampleBuffers int32

	// List of OpenGL texture compression format identifiers.
	compressedTextureFormats []int32

	// A channel which will have one empty struct inside it in the event that
	// a finalizer for a mesh, texture, etc has ran and something needs to be
	// free'd.
	wantFree chan struct{}

	// Structure used to manage pending occlusion queries.
	pending struct {
		sync.Mutex
		queries []pendingQuery
	}

	// RTT format lookups (from gfx formats to GL ones).
	rttTexFormats map[gfx.TexFormat]int32
	rttDSFormats  map[gfx.DSFormat]int32

	// If non-nil, then we are currently rendering to a texture. It is only
	// touched inside renderExec.
	rttCanvas *rttCanvas

	// Channel to wait for a Render() call to finish.
	renderComplete chan struct{}

	// yieldExit signals to the yield goroutine that it should exit.
	yieldExit chan struct{}
}

// Exec implements the Device interface.
func (r *device) Exec() chan func() bool {
	return r.renderExec
}

// Clock implements the gfx.Device interface.
func (r *device) Clock() *clock.Clock {
	return r.clock
}

// Short methods that just call the hooked methods (hooked methods are used in
// rtt.go file for render to texture things).

// Clear implements the gfx.Canvas interface.
func (r *device) Clear(rect image.Rectangle, bg gfx.Color) {
	r.hookedClear(rect, bg, nil, nil)
}

// ClearDepth implements the gfx.Canvas interface.
func (r *device) ClearDepth(rect image.Rectangle, depth float64) {
	r.hookedClearDepth(rect, depth, nil, nil)
}

// ClearStencil implements the gfx.Canvas interface.
func (r *device) ClearStencil(rect image.Rectangle, stencil int) {
	r.hookedClearStencil(rect, stencil, nil, nil)
}

// Draw implements the gfx.Canvas interface.
func (r *device) Draw(rect image.Rectangle, o *gfx.Object, c gfx.Camera) {
	r.hookedDraw(rect, o, c, nil, nil)
}

// QueryWait implements the gfx.Canvas interface.
func (r *device) QueryWait() {
	r.hookedQueryWait(nil, nil)
}

// Render implements the gfx.Canvas interface.
func (r *device) Render() {
	r.hookedRender(nil, nil)
}

// Info implements the gfx.Device interface.
func (r *device) Info() gfx.DeviceInfo {
	return r.devInfo
}

// SetDebugOutput implements the Device interface.
func (r *device) SetDebugOutput(w io.Writer) {
	r.warner.RLock()
	r.warner.W = w
	r.warner.RUnlock()
}

// RestoreState implements the Device interface.
func (r *device) RestoreState() {
	r.graphicsState.Restore(r)
}

// Destroy implements the Device interface.
func (r *device) Destroy() {
	// TODO(slimsag): free pending resources.
	r.yieldExit <- struct{}{}
}

// Implements gfx.Canvas interface.
func (r *device) hookedClear(rect image.Rectangle, bg gfx.Color, pre, post func()) {
	// Clearing an empty rectangle is effectively no-op.
	if rect.Empty() {
		return
	}
	r.renderExec <- func() bool {
		if pre != nil {
			pre()
		}
		r.graphicsState.Begin(r)

		// Color write mask effects the glClear call below.
		r.graphicsState.ColorWrite(true, true, true, true)

		// Perform clearing.
		r.performScissor(rect)
		r.graphicsState.ClearColor(bg)
		gl.Clear(uint32(gl.COLOR_BUFFER_BIT))

		r.queryYield()
		if post != nil {
			post()
		}
		return false
	}
}

// Implements gfx.Canvas interface.
func (r *device) hookedClearDepth(rect image.Rectangle, depth float64, pre, post func()) {
	// Clearing an empty rectangle is effectively no-op.
	if rect.Empty() {
		return
	}
	r.renderExec <- func() bool {
		if pre != nil {
			pre()
		}
		r.graphicsState.Begin(r)

		// Depth write mask effects the glClear call below.
		r.graphicsState.DepthWrite(true)

		// Perform clearing.
		r.performScissor(rect)
		r.graphicsState.ClearDepth(depth)
		gl.Clear(uint32(gl.DEPTH_BUFFER_BIT))

		r.queryYield()
		if post != nil {
			post()
		}
		return false
	}
}

// Implements gfx.Canvas interface.
func (r *device) hookedClearStencil(rect image.Rectangle, stencil int, pre, post func()) {
	// Clearing an empty rectangle is effectively no-op.
	if rect.Empty() {
		return
	}
	r.renderExec <- func() bool {
		if pre != nil {
			pre()
		}
		r.graphicsState.Begin(r)

		// Stencil mask effects the glClear call below.
		r.graphicsState.stencilMaskSeparate(0xFFFF, 0xFFFF)

		// Perform clearing.
		r.performScissor(rect)
		r.graphicsState.ClearStencil(stencil)
		gl.Clear(uint32(gl.STENCIL_BUFFER_BIT))

		r.queryYield()
		if post != nil {
			post()
		}
		return false
	}
}

func (r *device) hookedQueryWait(pre, post func()) {
	// Ask the render channel to wait for query results now.
	r.renderExec <- func() bool {
		if pre != nil {
			pre()
		}

		// Flush OpenGL commands.
		gl.Flush()

		// Wait for occlusion query results to come in.
		r.queryWait()

		if post != nil {
			post()
		}

		// signal render completion.
		r.renderComplete <- struct{}{}
		return false
	}
	<-r.renderComplete
}

func (r *device) yield() {
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			r.renderExec <- func() bool {
				r.rsrcManager.freePending()
				r.queryYield()
				return false
			}
		case <-r.yieldExit:
			return
		}
	}
}

func (r *device) hookedRender(pre, post func()) {
	// Ask the render channel to render things now.
	r.renderExec <- func() bool {
		// If any finalizers have ran and actually want us to free something,
		// then we perform this operation now.
		r.rsrcManager.freePending()

		if pre != nil {
			pre()
		}

		// Execute all pending operations.
		for i := 0; i < len(r.renderExec); i++ {
			f := <-r.renderExec
			f()
		}

		// Flush OpenGL commands.
		gl.Flush()

		// Wait for occlusion query results to come in.
		r.queryWait()

		if post != nil {
			post()
		}

		if tag.Gfxdebug {
			r.debugRender()
		}

		if r.rttCanvas != nil {
			// We are rendering to a texture. We do not need to clear global
			// state, tick the clock, or return true (frame rendered).

			// We do still need to signal render completion.
			r.renderComplete <- struct{}{}
			return false
		}

		// Tick the clock.
		r.clock.Tick()

		// signal render completion.
		r.renderComplete <- struct{}{}
		return true
	}
	<-r.renderComplete
}

// Tries to receive pending occlusion query results, returns immediately if
// none are available yet. Returns the number of queries still pending.
func (r *device) queryYield() int {
	if !r.glArbOcclusionQuery {
		return 0
	}
	r.pending.Lock()
	var (
		available, result int32
		toRemove          []pendingQuery
	)
	for _, query := range r.pending.queries {
		gl.GetQueryObjectiv(query.id, gl.QUERY_RESULT_AVAILABLE, &available)
		if available == gl.TRUE {
			// Get the result then.
			gl.GetQueryObjectiv(query.id, gl.QUERY_RESULT, &result)

			// Delete the query.
			gl.DeleteQueries(1, &query.id)

			// Update object's sample count.
			nativeObj := query.o.NativeObject.(*nativeObject)
			nativeObj.sampleCount = int(result)
			query.o.NativeObject = nativeObj

			// Remove from pending slice.
			toRemove = append(toRemove, query)
		}
	}
	for _, query := range toRemove {
		// Find the index.
		idx := 0
		for i, q := range r.pending.queries {
			if q == query {
				idx = i
			}
		}

		// Remove from the list.
		r.pending.queries = append(r.pending.queries[:idx], r.pending.queries[idx+1:]...)
	}
	length := len(r.pending.queries)
	r.pending.Unlock()
	return length
}

// Blocks until all pending occlusion query results are received.
func (r *device) queryWait() {
	if !r.glArbOcclusionQuery {
		return
	}

	// We have no choice except to busy-wait until the results come: OpenGL
	// doesn't provide a blocking mechanism for waiting for query results but
	// at least we can runtime.Gosched() other goroutines.
	for i := 0; r.queryYield() > 0; i++ {
		// Only runtime.Gosched() every 16th iteration to avoid bogging down
		// rendering.
		if i != 0 && (i%16) == 0 {
			runtime.Gosched()
		}
	}
}

// Effectively just calls stateScissor(), but passes in the proper bounds
// according to whether or not we are rendering to an rttCanvas or not.
func (r *device) performScissor(rect image.Rectangle) {
	if r.rttCanvas != nil {
		r.graphicsState.Scissor(r.rttCanvas.Bounds(), rect)
	} else {
		r.graphicsState.Scissor(r.Bounds(), rect)
	}
}

// Initialization of OpenGL in two seperate thread at the same time is racy
// because it is storing information on the OpenGL function pointers.
var initLock sync.Mutex

// newDevice is the implementation of New.
func newDevice(opts ...Option) (Device, error) {
	r := &device{
		BaseCanvas: &util.BaseCanvas{
			VMSAA: true,
		},
		warner:         util.NewWarner(nil),
		common:         glc.NewContext(),
		clock:          clock.New(),
		rsrcManager:    &rsrcManager{},
		renderExec:     make(chan func() bool, 1024),
		renderComplete: make(chan struct{}, 8),
		wantFree:       make(chan struct{}, 1),
		yieldExit:      make(chan struct{}, 1),
	}
	r.graphicsState = &graphicsState{
		GraphicsState: glc.NewGraphicsState(r.common),
	}
	go r.yield()

	for _, opt := range opts {
		opt(r)
	}

	// Initialize OpenGL.
	initLock.Lock()
	err := gl.Init()
	if err != nil {
		return nil, fmt.Errorf("OpenGL Error: %v", err)
	}
	initLock.Unlock()

	// Note: we don't need r.gl.Lock() here because no other goroutines
	// can be using r.ctx yet since we haven't returned from New().

	// Find the device's framebuffer precision.
	var redBits, greenBits, blueBits, alphaBits, depthBits, stencilBits int32
	gl.GetIntegerv(gl.RED_BITS, &redBits)
	gl.GetIntegerv(gl.GREEN_BITS, &greenBits)
	gl.GetIntegerv(gl.BLUE_BITS, &blueBits)
	gl.GetIntegerv(gl.ALPHA_BITS, &alphaBits)
	gl.GetIntegerv(gl.DEPTH_BITS, &depthBits)
	gl.GetIntegerv(gl.STENCIL_BITS, &stencilBits)

	r.BaseCanvas.VPrecision.RedBits = uint8(redBits)
	r.BaseCanvas.VPrecision.GreenBits = uint8(greenBits)
	r.BaseCanvas.VPrecision.BlueBits = uint8(blueBits)
	r.BaseCanvas.VPrecision.AlphaBits = uint8(alphaBits)
	r.BaseCanvas.VPrecision.DepthBits = uint8(depthBits)
	r.BaseCanvas.VPrecision.StencilBits = uint8(stencilBits)

	// Get the list of OpenGL extensions and parse it.
	extStr := gl.GoStr(gl.GetString(gl.EXTENSIONS))
	exts := glutil.ParseExtensions(extStr)

	if tag.Gfxdebug {
		r.debugInit(exts)
	}

	// Query whether we have the GL_ARB_framebuffer_object extension.
	r.glArbFramebufferObject = exts.Present("GL_ARB_framebuffer_object")

	// Query whether we have the GL_ARB_occlusion_query extension.
	r.glArbOcclusionQuery = exts.Present("GL_ARB_occlusion_query")

	// Query whether we have the GL_ARB_multisample extension.
	r.glArbMultisample = exts.Present("GL_ARB_multisample")
	if r.glArbMultisample {
		// Query the number of samples and sample buffers we have, if any.
		gl.GetIntegerv(gl.SAMPLES, &r.samples)
		gl.GetIntegerv(gl.SAMPLE_BUFFERS, &r.sampleBuffers)
		r.BaseCanvas.VPrecision.Samples = int(r.samples)
	}

	// Store GPU info.
	var maxTextureSize, maxVaryingFloats, maxVertexInputs, maxFragmentInputs, occlusionQueryBits int32
	gl.GetIntegerv(gl.MAX_TEXTURE_SIZE, &maxTextureSize)
	gl.GetIntegerv(gl.MAX_VARYING_FLOATS, &maxVaryingFloats)
	gl.GetIntegerv(gl.MAX_VERTEX_UNIFORM_COMPONENTS, &maxVertexInputs)
	gl.GetIntegerv(gl.MAX_FRAGMENT_UNIFORM_COMPONENTS, &maxFragmentInputs)
	if r.glArbOcclusionQuery {
		gl.GetQueryiv(gl.SAMPLES_PASSED, gl.QUERY_COUNTER_BITS, &occlusionQueryBits)
	}

	// Collect GPU information.
	r.devInfo.DepthClamp = exts.Present("GL_ARB_depth_clamp")
	r.devInfo.MaxTextureSize = int(maxTextureSize)
	r.devInfo.AlphaToCoverage = r.glArbMultisample && r.samples > 0 && r.sampleBuffers > 0
	r.devInfo.Name = gl.GoStr(gl.GetString(gl.RENDERER))
	r.devInfo.Vendor = gl.GoStr(gl.GetString(gl.VENDOR))
	r.devInfo.OcclusionQuery = r.glArbOcclusionQuery && occlusionQueryBits > 0
	r.devInfo.OcclusionQueryBits = int(occlusionQueryBits)
	r.devInfo.NPOT = exts.Present("GL_ARB_texture_non_power_of_two")
	r.devInfo.TexWrapBorderColor = true

	// OpenGL Information.
	glInfo := &gfx.GLInfo{
		Extensions: exts.Slice(),
	}
	glInfo.MajorVersion, glInfo.MinorVersion, glInfo.ReleaseVersion, glInfo.VendorVersion = r.common.Version()
	r.devInfo.GL = glInfo

	// GLSL information.
	glslInfo := &gfx.GLSLInfo{
		MaxVaryingFloats:  int(maxVaryingFloats),
		MaxVertexInputs:   int(maxVertexInputs),
		MaxFragmentInputs: int(maxFragmentInputs),
	}
	glslInfo.MajorVersion, glslInfo.MinorVersion, glslInfo.ReleaseVersion, _ = r.common.ShadingLanguageVersion()
	r.devInfo.GLSL = glslInfo

	if r.glArbFramebufferObject {
		// See http://www.opengl.org/wiki/Image_Format for more information.
		//
		// TODO:
		//  GL_DEPTH32F_STENCIL8 and GL_DEPTH_COMPONENT32F via Texture.Format
		//      option. (does it require an extension check with GL 2.0?)
		//  GL_STENCIL_INDEX8 (looks like 4.3+ GL hardware)
		//  GL_RGBA16F, GL_RGBA32F via Texture.Format
		//  Compressed formats (DXT ?)
		//  sRGB formats
		//
		//  GL_RGB16, GL_RGBA16

		r.rttTexFormats = make(map[gfx.TexFormat]int32, 16)
		r.rttDSFormats = make(map[gfx.DSFormat]int32, 16)

		// Formats below are guaranteed to be supported in OpenGL 2.x hardware:
		fmts := r.devInfo.RTTFormats

		// Color formats.
		fmts.ColorFormats = append(fmts.ColorFormats, []gfx.TexFormat{
			gfx.RGB,
			gfx.RGBA,
		}...)
		for _, cf := range fmts.ColorFormats {
			r.rttTexFormats[cf] = convertTexFormat(cf)
		}

		// Depth formats.
		fmts.DepthFormats = append(fmts.DepthFormats, []gfx.DSFormat{
			gfx.Depth16,
			gfx.Depth24,
			gfx.Depth32,
			gfx.Depth24AndStencil8,
		}...)
		r.rttDSFormats[gfx.Depth16] = gl.DEPTH_COMPONENT16
		r.rttDSFormats[gfx.Depth24] = gl.DEPTH_COMPONENT24
		r.rttDSFormats[gfx.Depth32] = gl.DEPTH_COMPONENT32

		// Stencil formats.
		fmts.StencilFormats = append(fmts.StencilFormats, []gfx.DSFormat{
			gfx.Depth24AndStencil8,
		}...)
		r.rttDSFormats[gfx.Depth24AndStencil8] = gl.DEPTH24_STENCIL8

		// Sample counts.
		// TODO: Beware integer texture formats -- MSAA can at max be
		//       GL_MAX_INTEGER_SAMPLES with those.
		var maxSamples int32
		gl.GetIntegerv(gl.MAX_SAMPLES, &maxSamples)
		for i := 0; i < int(maxSamples); i++ {
			fmts.Samples = append(fmts.Samples, i)
		}

		r.devInfo.RTTFormats = fmts
	}

	// Grab the current renderer bounds (opengl viewport).
	var viewport [4]int32
	gl.GetIntegerv(gl.VIEWPORT, &viewport[0])
	r.BaseCanvas.VBounds = image.Rect(0, 0, int(viewport[2]), int(viewport[3]))

	// Load the existing graphics state.
	r.graphicsState.Begin(r)

	// Update scissor rectangle.
	r.graphicsState.Scissor(r.BaseCanvas.VBounds, r.BaseCanvas.VBounds)

	// Grab the number of texture compression formats.
	var numFormats int32
	gl.GetIntegerv(gl.NUM_COMPRESSED_TEXTURE_FORMATS, &numFormats)

	// Store the slice of texture compression formats.
	if numFormats > 0 {
		r.compressedTextureFormats = make([]int32, numFormats)
		gl.GetIntegerv(gl.COMPRESSED_TEXTURE_FORMATS, &r.compressedTextureFormats[0])
	}
	return r, nil
}
