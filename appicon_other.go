// Copyright (c) the go-macos authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

//go:build !darwin

package appicon

// forPID answers that it cannot, rather than being absent: a consumer that
// cross-compiles gets the same API and one clean error from it.
func forPID(int32, int) (Pixels, error) { return Pixels{}, ErrUnsupported }
