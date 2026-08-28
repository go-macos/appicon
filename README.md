# go-macos/appicon

[![ci](https://github.com/go-macos/appicon/actions/workflows/ci.yml/badge.svg)](https://github.com/go-macos/appicon/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/go-macos/appicon.svg)](https://pkg.go.dev/github.com/go-macos/appicon)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue.svg)](LICENSE)

**A running application's own icon, as pixels, from pure Go with
`CGO_ENABLED=0`.** No cgo, no `sips`, no Objective-C source file anywhere in the
build: it reaches AppKit through
[`go-macos/objc`](https://github.com/go-macos/objc), which reaches it through
[purego](https://github.com/ebitengine/purego).

```go
px, err := appicon.ForPID(pid, 64)   // 64x64, straight RGBA
if err != nil {
        return err
}
cell.Image = toolkit.NewImageRGBA(px.Pix, px.W, px.H)
```

It exists because an interface that offers a person a list of what is running —
a switcher, a gallery, a picker for which window goes where — is a list of
**names** until it has the icons, and a name is not how anybody recognises an
application. Every system that shows one shows the icon: the Dock, ⌘-Tab,
Mission Control.

The icon belongs to whoever wrote the application. This package does not draw
one, guess one, or ship artwork: it asks for the icon that application is
already showing in the Dock.

**No permission of any kind.** An application's icon is public information about
a running process, unlike its windows — no Accessibility grant, no Screen
Recording, no prompt.

## A size is asked for, not inferred

An icon is a **family** of representations, not a picture — 32 of them for
Firefox on the machine this was written on. Asking for 64 gets the 64 the
designer drew; taking "the" icon and scaling it gets a blurred 512.

## What was measured, not assumed

Three things went wrong on the way here, and each is a comment in the code now:

**`NSRunningApplication` answers from a CACHE.** It is maintained by run-loop
notifications, so a process that never runs one asks a list filled before it
started and gets nil for an application that is plainly running — Firefox, pid
12849, "no such application". The run loop is pumped first, through
[`objc.PumpRunLoop`](https://github.com/go-macos/objc).

**A class that was never loaded is not an error.** `ClassID` answers zero and
every message to it answers zero, which arrives as "no such application" rather
than as "AppKit is not open". It is loaded explicitly.

**A bitmap context cannot draw non-premultiplied.** Asking for
`NSAlphaNonpremultiplied` makes `+graphicsContextWithBitmapImageRep:` return
**nil** — found with a probe that printed each step: the rep was fine, its
`bitmapData` was fine, the context was nil. So the icon is drawn premultiplied
and undone on the way out, because [`Pixels`](appicon.go) promises straight RGBA
— what a PNG holds and what an image widget takes. Handing out premultiplied
pixels as straight ones darkens exactly the transparent corners.

## Tests

```
go test ./...                        # portable logic
APPICON_LIVE=1 go test -run Live     # + the window server
```

The live test reads the icon of whatever application is **frontmost** — the
Finder when nothing else — and asserts the pixels are neither empty nor grey: an
icon that rasterised to nothing would pass every other check. A test binary
cannot read its own icon, and that is worth knowing rather than working around:
a plain Go binary has no bundle and an activation policy of Prohibited, so
AppKit does not know it exists.

## Requirements

macOS, Go 1.24+, `CGO_ENABLED=0`. Off darwin every entry point answers
`ErrUnsupported`, so a consumer that cross-compiles gets one clean error instead
of a missing symbol.

## Licence

BSD-3-Clause. See [LICENSE](LICENSE).
