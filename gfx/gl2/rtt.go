// Copyright 2014 The Azul3D Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gl2

import (
	"image"
	"log"
	"runtime"
	"sync"

	"github.com/qmcloud/engine/gfx"
	"github.com/qmcloud/engine/gfx/internal/gl/2.0/gl"
	"github.com/qmcloud/engine/gfx/internal/glc"
	"github.com/qmcloud/engine/gfx/internal/tag"
	"github.com/qmcloud/engine/gfx/internal/util"
)

func (r *rsrcManager) freeFBOs() {
	// Lock the list.
	r.Lock()

	if len(r.fbos) > 0 {
		if tag.Gfxdebug {
			log.Printf("gfx: free %d FBOs\n", len(r.fbos))
		}
		// Free the FBOs.
		gl.DeleteFramebuffers(int32(len(r.fbos)), &r.fbos[0])

		// Flush OpenGL commands.
		gl.Flush()
	}

	// Slice to zero, and unlock.
	r.fbos = r.fbos[:0]
	r.Unlock()
}

func (r *rsrcManager) freeRenderbuffers() {
	// Lock the list.
	r.Lock()

	if len(r.renderbuffers) > 0 {
		// Free the FBOs.
		if tag.Gfxdebug {
			log.Printf("gfx: free %d renderbuffers\n", len(r.renderbuffers))
		}
		gl.DeleteRenderbuffers(int32(len(r.renderbuffers)), &r.renderbuffers[0])

		// Flush OpenGL commands.
		gl.Flush()
	}

	// Slice to zero, and unlock.
	r.renderbuffers = r.renderbuffers[:0]
	r.Unlock()
}

// rttCanvas is the gfx.Canvas returned by RenderToTexture.
type rttCanvas struct {
	*util.BaseCanvas
	r   *device
	cfg gfx.RTTConfig

	// Frame buffer ID.
	fbo uint32

	// Render buffer ID's (rbColor is only a valid render buffer if e.g. the
	// cfg.Color field is nil).
	//
	// rbDepthAndStencil is only set if cfg.DepthFormat.IsCombined()
	rbColor, rbDepth, rbStencil, rbDepthAndStencil uint32

	// Decremented until zero, then all textures are free'd and all of the
	// canvas methods are no-op.
	textureCount struct {
		sync.RWMutex
		count int
	}
}

func (r *rttCanvas) freeTexture(n *nativeTexture) {
	r.textureCount.Lock()
	if r.textureCount.count == 0 {
		r.textureCount.Unlock()
		return
	}
	r.textureCount.count--
	if r.textureCount.count == 0 {
		// Everything is free now.
		if r.cfg.Color != nil {
			finalizeTexture(r.cfg.Color.NativeTexture.(*nativeTexture))
		}
		if r.cfg.Depth != nil {
			finalizeTexture(r.cfg.Depth.NativeTexture.(*nativeTexture))
		}
		if r.cfg.Stencil != nil {
			finalizeTexture(r.cfg.Stencil.NativeTexture.(*nativeTexture))
		}

		// Add the FBO to the free list.
		if r.fbo != 0 {
			r.r.rsrcManager.Lock()
			r.r.rsrcManager.fbos = append(r.r.rsrcManager.fbos, r.fbo)
			r.r.rsrcManager.Unlock()
		}

		// Add the render buffers to the free list.
		freeRb := func(id uint32) {
			if id == 0 {
				return
			}
			r.r.rsrcManager.Lock()
			r.r.rsrcManager.renderbuffers = append(r.r.rsrcManager.renderbuffers, id)
			r.r.rsrcManager.Unlock()
		}
		freeRb(r.rbColor)
		freeRb(r.rbDepth)
		freeRb(r.rbStencil)
		freeRb(r.rbDepthAndStencil)
	}
	r.textureCount.Unlock()
}

func finalizeRTTTexture(n *nativeTexture) {
	n.rttCanvas.freeTexture(n)
}

// Tells if all textures have been free'd and canvas methods are considered
// no-op.
func (r *rttCanvas) noop() bool {
	r.textureCount.RLock()
	if r.textureCount.count == 0 {
		return true
	}
	r.textureCount.RUnlock()
	return false
}

// Short methods that just call the hooked methods. We insert calls to rttBegin
// and rttEnd (they are executed via r.r.renderExec, i.e. legal for GL
// rendering commands to be invoked).

// Implements gfx.Canvas interface.
func (r *rttCanvas) Clear(rect image.Rectangle, bg gfx.Color) {
	if r.noop() {
		return
	}
	r.r.hookedClear(rect, bg, r.rttBegin, r.rttEnd)
}

// Implements gfx.Canvas interface.
func (r *rttCanvas) ClearDepth(rect image.Rectangle, depth float64) {
	r.r.hookedClearDepth(rect, depth, r.rttBegin, r.rttEnd)
}

// Implements gfx.Canvas interface.
func (r *rttCanvas) ClearStencil(rect image.Rectangle, stencil int) {
	r.r.hookedClearStencil(rect, stencil, r.rttBegin, r.rttEnd)
}

// Implements gfx.Canvas interface.
func (r *rttCanvas) Draw(rect image.Rectangle, o *gfx.Object, c gfx.Camera) {
	r.r.hookedDraw(rect, o, c, r.rttBegin, r.rttEnd)
}

// Implements gfx.Canvas interface.
func (r *rttCanvas) QueryWait() {
	r.r.hookedQueryWait(r.rttBegin, r.rttEnd)
}

// Implements gfx.Canvas interface.
func (r *rttCanvas) Render() {
	r.r.hookedRender(nil, func() {
		// Generate mipmaps for any texture with a mipmapped format. This must
		// be done here because the texture has just been rendered to.
		do := func(t *gfx.Texture) {
			if t == nil || !t.MinFilter.Mipmapped() {
				return
			}
			n := t.NativeTexture.(*nativeTexture)
			gl.BindTexture(gl.TEXTURE_2D, n.id)
			gl.GenerateMipmap(gl.TEXTURE_2D)
		}
		do(r.cfg.Color)
		do(r.cfg.Depth)
		do(r.cfg.Stencil)
		gl.BindTexture(gl.TEXTURE_2D, 0)
	})
}

// Implements gfx.Downloadable interface.
func (r *rttCanvas) Download(rect image.Rectangle, complete chan image.Image) {
	r.r.hookedDownload(rect, complete, r.rttBegin, r.rttEnd)
}

func (r *rttCanvas) rttBegin() {
	r.r.rttCanvas = r

	// Bind the framebuffer object.
	gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo)
}

func (r *rttCanvas) rttEnd() {
	r.r.rttCanvas = nil

	// Unbind the framebuffer object.
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
}

// RenderToTexture implements the gfx.Renderer interface.
func (r *device) RenderToTexture(cfg gfx.RTTConfig) gfx.Canvas {
	if !cfg.Valid() {
		panic("RenderToTexture(): Configuration is invalid!")
	}

	if !r.glArbFramebufferObject {
		// We don't have GL_ARB_framebuffer_object extension, we can't do this
		// at all.
		return nil
	}

	// Find OpenGL versions of formats.
	colorFormat, ok := r.rttTexFormats[cfg.ColorFormat]
	if cfg.ColorFormat != gfx.ZeroTexFormat && !ok {
		return nil
	}
	depthFormat, ok := r.rttDSFormats[cfg.DepthFormat]
	if cfg.DepthFormat != gfx.ZeroDSFormat && !ok {
		return nil
	}
	stencilFormat, ok := r.rttDSFormats[cfg.StencilFormat]
	if cfg.StencilFormat != gfx.ZeroDSFormat && !ok {
		return nil
	}

	// Create the RTT canvas.
	cr, cg, cb, ca := cfg.ColorFormat.Bits()
	canvas := &rttCanvas{
		BaseCanvas: &util.BaseCanvas{
			VMSAA: true,
			VPrecision: gfx.Precision{
				RedBits: cr, GreenBits: cg, BlueBits: cb, AlphaBits: ca,
				DepthBits:   cfg.DepthFormat.DepthBits(),
				StencilBits: cfg.StencilFormat.StencilBits(),
			},
			VBounds: cfg.Bounds,
		},
		r:   r,
		cfg: cfg,
	}

	var (
		nTexColor, nTexDepth, nTexStencil *nativeTexture
		fbError                           error
	)
	r.renderExec <- func() bool {
		width := int32(cfg.Bounds.Dx())
		height := int32(cfg.Bounds.Dy())

		// Create the FBO.
		gl.GenFramebuffers(1, &canvas.fbo)
		gl.BindFramebuffer(gl.FRAMEBUFFER, canvas.fbo)

		// Create an OpenGL render buffer for each nil cfg texture. This allows
		// the driver a chance to optimize storage for e.g. a depth buffer when
		// you don't intend to use it as a texture.
		samples := int32(cfg.Samples)
		if cfg.Color == nil && cfg.ColorFormat != gfx.ZeroTexFormat {
			// We do not want a color texture, but we do want a color buffer.
			gl.GenRenderbuffers(1, &canvas.rbColor)
			gl.BindRenderbuffer(gl.RENDERBUFFER, canvas.rbColor)
			gl.RenderbufferStorageMultisample(gl.RENDERBUFFER, samples, uint32(colorFormat), width, height)
			gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.RENDERBUFFER, canvas.rbColor)
		}
		dsCombined := cfg.DepthFormat == cfg.StencilFormat && cfg.DepthFormat.IsCombined()
		if cfg.Depth == nil && cfg.Stencil == nil && dsCombined {
			// We do not want a depth or stencil texture, but we do want a
			// combined depth/stencil buffer.
			gl.GenRenderbuffers(1, &canvas.rbDepthAndStencil)
			gl.BindRenderbuffer(gl.RENDERBUFFER, canvas.rbDepthAndStencil)
			gl.RenderbufferStorageMultisample(gl.RENDERBUFFER, samples, uint32(depthFormat), width, height)
			gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, gl.RENDERBUFFER, canvas.rbDepthAndStencil)
			gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.STENCIL_ATTACHMENT, gl.RENDERBUFFER, canvas.rbDepthAndStencil)
		} else {
			if cfg.Depth == nil && cfg.DepthFormat != gfx.ZeroDSFormat {
				// We do not want a depth texture, but we do want a depth buffer.
				gl.GenRenderbuffers(1, &canvas.rbDepth)
				gl.BindRenderbuffer(gl.RENDERBUFFER, canvas.rbDepth)
				gl.RenderbufferStorageMultisample(gl.RENDERBUFFER, samples, uint32(depthFormat), width, height)
				gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, gl.RENDERBUFFER, canvas.rbDepth)
			}
			if cfg.Stencil == nil && cfg.StencilFormat != gfx.ZeroDSFormat {
				// We do not want a stencil texture, but we do want a stencil buffer.
				gl.GenRenderbuffers(1, &canvas.rbStencil)
				gl.BindRenderbuffer(gl.RENDERBUFFER, canvas.rbStencil)
				gl.RenderbufferStorageMultisample(gl.RENDERBUFFER, samples, uint32(stencilFormat), width, height)
				gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.STENCIL_ATTACHMENT, gl.RENDERBUFFER, canvas.rbStencil)
			}
		}

		// Create an OpenGL texture for every non-nil cfg texture.
		if cfg.Color != nil && cfg.ColorFormat != gfx.ZeroTexFormat {
			// We want a color texture, not a color buffer.
			nTexColor = newNativeTexture(r, colorFormat, int(width), int(height))
			gl.TexImage2D(gl.TEXTURE_2D, 0, colorFormat, width, height, 0, gl.BGRA, gl.UNSIGNED_BYTE, nil)
			gl.GenerateMipmap(gl.TEXTURE_2D)
			gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, nTexColor.id, 0)
		}
		// Only non-combined depth/stencil formats can render into a texture.
		if !dsCombined {
			if cfg.Depth != nil && cfg.DepthFormat != gfx.ZeroDSFormat {
				// We want a depth texture, not a depth buffer.
				nTexDepth = newNativeTexture(r, depthFormat, int(width), int(height))
				gl.TexImage2D(gl.TEXTURE_2D, 0, depthFormat, width, height, 0, gl.DEPTH_COMPONENT, gl.UNSIGNED_BYTE, nil)
				gl.GenerateMipmap(gl.TEXTURE_2D)
				gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, gl.TEXTURE_2D, nTexDepth.id, 0)
			}
		}

		// Check for errors.
		status := int(gl.CheckFramebufferStatus(gl.FRAMEBUFFER))
		fbError = r.common.FramebufferStatus(status)

		// Unbind textures, render buffers, and the FBO.
		gl.BindTexture(gl.TEXTURE_2D, 0)
		gl.BindRenderbuffer(gl.RENDERBUFFER, 0)
		gl.BindFramebuffer(gl.FRAMEBUFFER, 0)

		// Signal render completion.
		r.renderComplete <- struct{}{}
		return false // No frame was rendered.
	}
	<-r.renderComplete

	if fbError != nil {
		if fbError == glc.FramebufferUnsupported {
			// Ideally this shouldn't happen, but it could under e.g. strange
			// drivers not supporting a combination of 'supported' formats.
			return nil
		}
		panic(fbError)
	}

	// Finish textures (mark as loaded, clear data slices, unlock).
	finishTexture := func(t *gfx.Texture, dsFmt *gfx.DSFormat, native *nativeTexture) {
		if t == nil {
			return
		}
		if native == nil {
			return
		}
		canvas.textureCount.count++
		// Attach a finalizer to the texture that will later free it.
		runtime.SetFinalizer(native, finalizeRTTTexture)
		native.rttCanvas = canvas
		native.destroyHandler = finalizeRTTTexture
		t.NativeTexture = native
		t.Bounds = cfg.Bounds
		t.Loaded = true
		t.ClearData()
	}
	finishTexture(cfg.Color, nil, nTexColor)
	finishTexture(cfg.Depth, &cfg.DepthFormat, nTexDepth)
	finishTexture(cfg.Stencil, &cfg.StencilFormat, nTexStencil)

	// OpenGL makes no guarantee about the data existing in the texture until
	// we actually draw something, so clear everything now.
	canvas.Clear(image.Rect(0, 0, 0, 0), gfx.Color{0, 0, 0, 1})
	canvas.ClearDepth(image.Rect(0, 0, 0, 0), 1.0)
	canvas.ClearStencil(image.Rect(0, 0, 0, 0), 0)

	return canvas
}
