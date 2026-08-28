// Copyright (c) the go-macos authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

//go:build darwin

package appicon

import (
	"os"
	"testing"

	"github.com/go-macos/objc"
)

// TestLiveReadsTheIconOfARunningApplication, all the way through: the window
// server's list, AppKit's icon, a bitmap this package owns, and the pixels out
// the other side.
//
// It reads the icon of whatever application is FRONTMOST, which on any machine
// with a session is something — the Finder when nothing else. A test binary
// cannot read its OWN icon: a plain Go binary has no bundle and an activation
// policy of Prohibited, so NSRunningApplication does not know it exists, which
// is worth knowing and is why this does not try.
func TestLiveReadsTheIconOfARunningApplication(t *testing.T) {
	if os.Getenv("APPICON_LIVE") == "" {
		t.Skip("set APPICON_LIVE=1 to run the test that talks to the window server")
	}
	pid := frontmostPID(t)

	px, err := ForPID(pid, 64)
	if err != nil {
		t.Fatalf("ForPID(%d, 64): %v", pid, err)
	}
	if px.W != 64 || px.H != 64 || len(px.Pix) != 64*64*4 {
		t.Fatalf("got %dx%d in %d bytes, want 64x64 in %d", px.W, px.H, len(px.Pix), 64*64*4)
	}

	// Not a blank square. An icon that rasterised to nothing would pass every
	// check above, and that is exactly the failure this path had while the
	// bitmap context was returning nil.
	var opaque, coloured int
	for i := 0; i+3 < len(px.Pix); i += 4 {
		if px.Pix[i+3] == 0 {
			continue
		}
		opaque++
		if px.Pix[i] != px.Pix[i+1] || px.Pix[i+1] != px.Pix[i+2] {
			coloured++
		}
	}
	if opaque < 64*64/8 {
		t.Errorf("only %d of %d pixels are opaque; the icon did not draw", opaque, 64*64)
	}
	t.Logf("%d of %d pixels opaque, %d of them coloured", opaque, 64*64, coloured)

	// The corners of an icon are transparent, and they are what proves the
	// alpha survived: a premultiplied buffer handed out as straight RGBA is
	// darkest exactly there.
	if px.Pix[3] != 0 && px.Pix[len(px.Pix)-1] != 0 {
		t.Log("both corners are opaque; this icon fills its square")
	}
}

// frontmostPID asks AppKit which application is in front.
func frontmostPID(t *testing.T) int32 {
	t.Helper()
	if err := objc.Load(objc.AppKit, objc.Foundation); err != nil {
		t.Skipf("no AppKit here: %v", err)
	}
	var pid int32
	objc.AutoreleasePool(func() {
		_ = objc.PumpRunLoop(0.05)
		ws := objc.ClassID("NSWorkspace").Send(objc.Sel("sharedWorkspace"))
		app := ws.Send(objc.Sel("frontmostApplication"))
		if app == 0 {
			return
		}
		pid = objc.Send[int32](app, objc.Sel("processIdentifier"))
	})
	if pid == 0 {
		t.Skip("nothing is frontmost: this session has no window server")
	}
	return pid
}
