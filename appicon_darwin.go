// Copyright (c) the go-macos authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

//go:build darwin

package appicon

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/go-macos/objc"
)

// The AppKit constants this file needs. They are values from NSBitmapImageRep.h
// and NSGraphicsContext.h rather than anything this package invents.
const (
	// nsDeviceRGBColorSpace is the colour space name; NSCalibratedRGB would
	// make the pixels depend on the display's profile, which is the last thing
	// an icon copied into a texture wants.
	nsDeviceRGBColorSpace = "NSDeviceRGBColorSpace"
	// premultipliedBitmapFormat is ZERO -- the default, RGBA premultiplied.
	//
	// Not a preference. NSAlphaNonpremultiplied (1<<1) is what this package
	// wants to hand out, and asking for it makes
	// +graphicsContextWithBitmapImageRep: return NIL: a bitmap context draws
	// into premultiplied pixels only. Measured, with a probe that printed each
	// step -- the rep was fine, its bitmapData was fine, the context was nil.
	// So the icon is drawn premultiplied and undone on the way out.
	premultipliedBitmapFormat = 0
	// bitsPerSample, samples and bitsPerPixel: straight 8-bit RGBA.
	bitsPerSample = 8
	samples       = 4
	bitsPerPixel  = 32
)

// pumpSeconds is how long the run loop is serviced before asking AppKit which
// application owns a process. The same fifty milliseconds go-macos/accessibility
// measured: long enough for a pending workspace notification, short enough not
// to be felt.
const pumpSeconds = 0.05

// forPID is the darwin half of [ForPID].
//
// The whole of it runs inside ONE autorelease pool, on ONE locked thread: every
// object here is autoreleased by AppKit, and the pool belongs to the thread that
// made it (see go-macos/objc v0.4.0, which pins it).
// loadOnce brings AppKit in. NSRunningApplication, NSBitmapImageRep and
// NSGraphicsContext all live there, and a class that has not been loaded is not
// an error from the runtime -- ClassID answers zero and every message to it
// answers zero, which arrives here as "no such application" for an application
// that is plainly running. Measured, before this existed.
var (
	loadOnce sync.Once
	loadErr  error
)

func forPID(pid int32, size int) (out Pixels, err error) {
	loadOnce.Do(func() { loadErr = objc.Load(objc.AppKit, objc.Foundation) })
	if loadErr != nil {
		return Pixels{}, loadErr
	}
	objc.AutoreleasePool(func() {
		// NSRunningApplication answers from a list AppKit maintains through
		// run-loop notifications, so a process that never runs one is asking a
		// cache that was filled before it started -- and gets nil for an
		// application that is plainly running. Measured here: Firefox, pid
		// 12849, reported as no such application until the loop was pumped.
		// go-macos/objc owns the pump; see PumpRunLoop for what it costs.
		_ = objc.PumpRunLoop(pumpSeconds)

		app := objc.ClassID("NSRunningApplication").Send(
			objc.Sel("runningApplicationWithProcessIdentifier:"), pid)
		if app == 0 {
			err = ErrNoApp
			return
		}
		icon := app.Send(objc.Sel("icon"))
		if icon == 0 {
			err = ErrNoIcon
			return
		}
		out, err = rasterise(icon, size)
	})
	return out, err
}

// rasterise draws an NSImage into a bitmap this package owns and copies the
// bytes out.
//
// Drawn rather than read. An NSImage is a FAMILY of representations, and the
// nearest thing to "its pixels" — taking the first rep and reading its
// bitmapData — gives whatever size that rep happens to be, in whatever format
// the application shipped, which for a modern .icns is a 1024 the caller then
// scales down badly. Asking AppKit to draw it at the size wanted is what picks
// the right member of the family.
// rasterise draws an NSImage square at size pixels a side.
func rasterise(icon objc.ID, size int) (Pixels, error) { return rasteriseAt(icon, size, size) }

// rasteriseAt draws an NSImage into a w by h bitmap.
//
// Two sides rather than one because not every image is square: an application's
// icon is, and a system symbol is not -- visionpro is 21 by 13 points -- and
// drawing a wide glyph into a square box stretches it by however much the two
// differ. Measured: 62% taller than it should be, which is enough for the person
// looking at it to say that is not the icon that was there before.
func rasteriseAt(icon objc.ID, w, h int) (Pixels, error) {
	rep := objc.ClassID("NSBitmapImageRep").Send(objc.Sel("alloc")).Send(
		objc.Sel("initWithBitmapDataPlanes:pixelsWide:pixelsHigh:bitsPerSample:samplesPerPixel:hasAlpha:isPlanar:colorSpaceName:bitmapFormat:bytesPerRow:bitsPerPixel:"),
		uintptr(0), // let it allocate: a plane this side would have to outlive the call
		w, h,
		bitsPerSample, samples,
		true,  // hasAlpha
		false, // isPlanar
		objc.NSString(nsDeviceRGBColorSpace),
		uintptr(premultipliedBitmapFormat),
		w*4, bitsPerPixel,
	)
	if rep == 0 {
		return Pixels{}, ErrNoIcon
	}
	defer rep.Send(objc.Sel("release"))

	ctx := objc.ClassID("NSGraphicsContext").Send(
		objc.Sel("graphicsContextWithBitmapImageRep:"), rep)
	if ctx == 0 {
		return Pixels{}, ErrNoIcon
	}
	// Saved and restored around the draw: the current context is process-wide
	// state, and a program that draws its own interface is entitled to find it
	// as it left it.
	gc := objc.ClassID("NSGraphicsContext")
	gc.Send(objc.Sel("saveGraphicsState"))
	defer gc.Send(objc.Sel("restoreGraphicsState"))
	gc.Send(objc.Sel("setCurrentContext:"), ctx)

	icon.Send(objc.Sel("drawInRect:fromRect:operation:fraction:"),
		rect{0, 0, float64(w), float64(h)}, // where
		rect{0, 0, 0, 0},                   // all of it
		uintptr(2),                         // NSCompositingOperationSourceOver
		1.0,                                // fully opaque
	)
	ctx.Send(objc.Sel("flushGraphics"))

	// Taken as an unsafe.Pointer rather than as a uintptr that is then
	// converted: a uintptr is a NUMBER, the garbage collector does not see it,
	// and go vet's unsafeptr check is right to object to the round trip.
	data := objc.Send[unsafe.Pointer](rep, objc.Sel("bitmapData"))
	if data == nil {
		return Pixels{}, ErrNoIcon
	}
	n := w * h * 4
	// COPIED, not referenced: the rep is released when this returns and the
	// bytes go with it. A slice pointing into freed AppKit memory is the kind
	// of bug that shows up as somebody else's crash.
	pix := make([]byte, n)
	unpremultiply(pix, unsafe.Slice((*byte)(data), n))
	return Pixels{Pix: pix, W: w, H: h}, nil
}

// rect is CGRect, which is what drawInRect: takes by value.
type rect struct{ X, Y, W, H float64 }

// unpremultiply copies src into dst, undoing the alpha multiplication.
//
// A bitmap context can only draw premultiplied, and [Pixels] promises straight
// RGBA — what a PNG holds and what a toolkit's image widget takes. Doing it here
// once is the difference between a promise kept and a surprise in whatever
// draws the icon: premultiplied pixels shown as straight ones come out dark
// wherever they are transparent, which for an icon is the whole of its corners.
func unpremultiply(dst, src []byte) {
	for i := 0; i+3 < len(src) && i+3 < len(dst); i += 4 {
		a := src[i+3]
		dst[i+3] = a
		switch a {
		case 0:
			// Nothing to divide by, and nothing to see: leave it fully clear
			// rather than inventing a colour for it.
			dst[i], dst[i+1], dst[i+2] = 0, 0, 0
		case 255:
			dst[i], dst[i+1], dst[i+2] = src[i], src[i+1], src[i+2]
		default:
			for c := range 3 {
				v := int(src[i+c]) * 255 / int(a)
				if v > 255 {
					v = 255
				}
				dst[i+c] = byte(v)
			}
		}
	}
}

// symbol renders a system symbol by drawing the NSImage the system hands back.
//
// No symbol configuration is asked for: drawInRect: scales the glyph to the
// box, which is what a caller asking for a size means, and a point size and a
// weight would be two more numbers to get wrong for no gain.
func symbol(name string, size int) (out Pixels, err error) {
	loadOnce.Do(func() { loadErr = objc.Load(objc.AppKit, objc.Foundation) })
	if loadErr != nil {
		return Pixels{}, loadErr
	}
	objc.AutoreleasePool(func() {
		img := objc.ClassID("NSImage").Send(
			objc.Sel("imageWithSystemSymbolName:accessibilityDescription:"),
			objc.NSString(name), objc.NSString(name))
		if img == 0 {
			err = fmt.Errorf("%w: %q", ErrNoSymbol, name)
			return
		}
		// AT ITS OWN SHAPE. A system symbol is not square -- visionpro is 21 by
		// 13 points, eyeglasses 23 by 10 -- and a menu bar scales what it is
		// given by HEIGHT. Drawn into a square box the glyph comes out 62%
		// taller than it should be, which is what "that is not the icon that
		// was there before" looks like.
		//
		// size is the LONGER side, so a caller asking for 44 gets a picture no
		// bigger than that in either direction.
		w, h := size, size
		if s := objc.Send[cgSize](img, objc.Sel("size")); s.W > 0 && s.H > 0 {
			if s.W >= s.H {
				h = int(float64(size)*s.H/s.W + 0.5)
			} else {
				w = int(float64(size)*s.W/s.H + 0.5)
			}
		}
		if w < 1 || h < 1 {
			err = fmt.Errorf("%w: %q is %dx%d at that size", ErrNoSymbol, name, w, h)
			return
		}
		out, err = rasteriseAt(img, w, h)
	})
	return out, err
}

// cgSize is NSSize, which is what -[NSImage size] hands back by value.
type cgSize struct{ W, H float64 }
