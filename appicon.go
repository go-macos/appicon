// Copyright (c) the go-macos authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

// Package appicon reads a running application's own icon, as pixels, from pure
// Go with CGO_ENABLED=0.
//
// It exists because an interface that offers a person a list of what is running
// — a switcher, a gallery, a picker for which window goes where — is a list of
// NAMES until it has the icons, and a name is not how anybody recognises an
// application. Every system that shows one shows the icon: the Dock, ⌘-Tab,
// Mission Control.
//
// The icon belongs to whoever wrote the application. This package does not draw
// one, guess one, or ship artwork: it asks the window server for the icon that
// application is already showing in the Dock, and hands over the pixels.
//
//	px, err := appicon.ForPID(pid, 64)
//	if err == nil {
//		cell.Image = toolkit.NewImageRGBA(px.Pix, px.W, px.H)
//	}
//
// It needs no permission of any kind: NSRunningApplication's icon is public
// information about a running process, unlike its windows.
package appicon

import "errors"

// Errors this package returns.
var (
	// ErrUnsupported is what every entry point answers away from macOS.
	ErrUnsupported = errors.New("appicon: unsupported on this platform (macOS only)")
	// ErrNoApp means no running application owns that process id. A shell, a
	// daemon and a process that has already exited all look like this.
	ErrNoApp = errors.New("appicon: no running application with that process id")
	// ErrNoIcon means the application has one but it could not be rasterised —
	// an icon that is entirely a vector at a size nothing has drawn yet, or a
	// bitmap context the window server would not give.
	ErrNoIcon = errors.New("appicon: the application's icon could not be read")
	// ErrSize means a size outside [MinSize, MaxSize] was asked for.
	ErrSize = errors.New("appicon: unusable size")
)

// The sizes an icon may be asked for.
//
// The floor is where an icon stops being recognisable and the ceiling is what
// macOS itself stores: an .icns holds 1024 at the largest, and asking for more
// scales a picture up rather than giving a better one.
const (
	MinSize = 8
	MaxSize = 1024
)

// Pixels is a rasterised icon: straight RGBA, 8 bits a channel, W*H*4 bytes,
// rows top to bottom with no padding.
//
// Straight and not premultiplied, because that is what a toolkit's image widget
// takes and what a PNG holds; the conversion is done here, once, rather than
// left as a surprise in whatever draws it.
type Pixels struct {
	Pix  []byte
	W, H int
}

// ForPID returns the icon of the application owning pid, rasterised square at
// size pixels a side.
//
// A size is asked for rather than inferred because an icon is a family of
// representations, not a picture: asking for 64 gets the 64 the designer drew,
// where taking "the" icon and scaling it gets a blurred 512.
func ForPID(pid int32, size int) (Pixels, error) {
	if size < MinSize || size > MaxSize {
		return Pixels{}, ErrSize
	}
	return forPID(pid, size)
}
