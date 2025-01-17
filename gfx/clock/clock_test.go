// Copyright 2014 The Azul3D Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package clock

import (
	"testing"
	"time"

	"github.com/qmcloud/engine/lmath"
)

func TestHighResolutionTime(t *testing.T) {
	lrStart := time.Now()
	hrStart := getTime()

	var diffTotal time.Duration
	for i := 0; i < 10; i++ {
		lrDiff := time.Since(lrStart)
		hrDiff := getTime() - hrStart

		diffTotal += hrDiff
		t.Logf("%d.\ttime.Since()=%d\tgetTime()=%d", i, lrDiff, hrDiff)

		lrStart = time.Now()
		hrStart = getTime()
	}

	if diffTotal <= 0 {
		t.Fail()
	}
}

func TestFrameRateLimit(t *testing.T) {
	c := New()
	c.SetMaxFrameRate(100)
	c.SetAvgSamples(100)
	for i := 0; i < c.AvgSamples(); i++ {
		c.Tick()
	}
	avg := c.AvgFrameRate()
	if !lmath.AlmostEqual(avg, 100, 0.05) {
		t.Log("got avg", avg)
		t.Fatal("expected avg near", 100)
	}
}

func TestFrameRateStall(t *testing.T) {
	c := New()
	c.SetMaxFrameRate(100)
	c.SetAvgSamples(100)
	stop := time.After(1 * time.Second)
	for {
		c.Tick()
		select {
		case <-stop:
			if c.FrameRate() < 50 {
				t.Log("before stall", c.FrameRate())
				t.Fatal("expected > 50")
			}

			// Stall for an entire second.
			time.Sleep(1 * time.Second)

			if c.FrameRate() != 0 {
				t.Log("after stall", c.FrameRate())
				t.Fatal("expected 0")
			}
			return
		default:
		}
	}
}
