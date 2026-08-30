// Copyright (c) the go-macos authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

//go:build darwin

package appicon

import (
	"errors"
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

// TestLiveRendersASystemSymbol asks the window server for one of its own
// symbols, so it runs only under APPICON_LIVE.
//
// It is the measurement the desk's menu bar needed: a glyph drawn as an
// outline by a cross-platform toolkit puts far less ink in a 22-point bar than
// the one the system draws for that bar, and "far less" is a number here
// rather than an opinion.
func TestLiveRendersASystemSymbol(t *testing.T) {
	if os.Getenv("APPICON_LIVE") == "" {
		t.Skip("set APPICON_LIVE=1 to run the test that talks to the window server")
	}
	const px = 44
	px44, err := Symbol("display", px)
	if err != nil {
		t.Fatalf("Symbol: %v", err)
	}
	// ITS OWN SHAPE, not a square. A menu bar scales what it is given by
	// height, so a wide glyph squeezed into a square comes out taller than the
	// system draws it -- visionpro is 21 by 13 points, and stretching it was
	// noticed by the person looking at their menu bar.
	if px44.W != px && px44.H != px {
		t.Errorf("Symbol gave %dx%d; one side should be the %d asked for", px44.W, px44.H, px)
	}
	if px44.W > px || px44.H > px {
		t.Errorf("Symbol gave %dx%d, bigger than the %d asked for", px44.W, px44.H, px)
	}
	if px44.W == px44.H {
		t.Logf("display came back square (%dx%d); that is this symbol's own shape", px44.W, px44.H)
	}
	inked, box := 0, px44.W*px44.H
	for i := 3; i < len(px44.Pix); i += 4 {
		if px44.Pix[i] > 0 {
			inked++
		}
	}
	if inked == 0 {
		t.Fatal("the symbol came back blank")
	}
	t.Logf("display: %dx%d, %d of %d pixels carry ink (%d%%)",
		px44.W, px44.H, inked, box, 100*inked/box)

	// And a symbol that is decidedly not square keeps that.
	if wide, err := Symbol("eyeglasses", px); err == nil {
		t.Logf("eyeglasses: %dx%d", wide.W, wide.H)
		if wide.W == wide.H {
			t.Errorf("eyeglasses came back square at %dx%d; it is 23 by 10 points",
				wide.W, wide.H)
		}
	}

	// A name the system does not have is refused rather than drawn as nothing:
	// a menu bar with a hole in it is worse than one with nothing in it.
	if _, err := Symbol("no.such.symbol.anywhere.at.all", px); !errors.Is(err, ErrNoSymbol) {
		t.Errorf("Symbol of a made-up name = %v, want ErrNoSymbol", err)
	}
}
