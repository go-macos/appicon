// Copyright (c) the go-macos authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package appicon

import (
	"errors"
	"testing"
)

// TestForPIDRefusesAnUnusableSize before it asks the system anything: a size
// nothing can be drawn at is the caller's mistake, and saying so costs no
// round-trip.
func TestForPIDRefusesAnUnusableSize(t *testing.T) {
	for _, size := range []int{0, -1, MinSize - 1, MaxSize + 1, 1 << 20} {
		if _, err := ForPID(1, size); !errors.Is(err, ErrSize) {
			t.Errorf("ForPID(_, %d) = %v, want ErrSize", size, err)
		}
	}
}

// TestForPIDAcceptsTheEdgesOfTheRange, which are legal sizes and must not be
// refused by an off-by-one.
func TestForPIDAcceptsTheEdgesOfTheRange(t *testing.T) {
	for _, size := range []int{MinSize, MaxSize} {
		// A process id nothing owns: what comes back is whatever the platform
		// says, and what must NOT come back is ErrSize.
		if _, err := ForPID(-1, size); errors.Is(err, ErrSize) {
			t.Errorf("ForPID(_, %d) was refused for its size", size)
		}
	}
}

// TestPixelsIsPlainRGBA pins the shape of what a caller gets, because every
// consumer indexes it by hand.
func TestPixelsIsPlainRGBA(t *testing.T) {
	p := Pixels{Pix: make([]byte, 4*4*4), W: 4, H: 4}
	if got, want := len(p.Pix), p.W*p.H*4; got != want {
		t.Errorf("%d bytes for %dx%d, want %d", got, p.W, p.H, want)
	}
}

func TestSymbolRefusesWhatIsNotAName(t *testing.T) {
	for _, c := range []struct {
		name string
		px   int
		why  string
	}{
		{"", 44, "no name at all"},
		{"   ", 44, "a name of spaces"},
	} {
		if _, err := Symbol(c.name, c.px); !errors.Is(err, ErrNoSymbol) {
			t.Errorf("%s: Symbol = %v, want ErrNoSymbol", c.why, err)
		}
	}
	for _, px := range []int{MinSize - 1, MaxSize + 1} {
		if _, err := Symbol("display", px); !errors.Is(err, ErrSize) {
			t.Errorf("Symbol at %d pixels = %v, want ErrSize", px, err)
		}
	}
}

func TestSymbolReachesThePlatform(t *testing.T) {
	// The last statement of Symbol is the platform call. Off macOS it reports
	// that there are no system symbols; on macOS a name nothing has is refused
	// by the platform rather than by the argument checks above -- either way
	// the call is made, which is what this pins.
	_, err := Symbol("no.such.symbol.anywhere.at.all", 44)
	if err == nil {
		t.Fatal("a symbol nothing has was rendered")
	}
	if !errors.Is(err, ErrNoSymbol) && !errors.Is(err, ErrUnsupported) {
		t.Errorf("Symbol = %v, want ErrNoSymbol or ErrUnsupported", err)
	}
}
