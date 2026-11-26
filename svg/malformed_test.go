// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package svg

import (
	"testing"
	"time"
)

// TestMalformedXMLIsRefusedPromptly: the pre-scan reads tokens until the end of
// the document, and a malformed one keeps returning the same error forever, so
// the loop must stop on the first error. A regression hangs; the deadline turns
// that into this failure rather than a stuck process.
func TestMalformedXMLIsRefusedPromptly(t *testing.T) {
	for _, doc := range []string{
		`<svg><g></svg>`,
		`<svg attr=">`,
		`<svg><`,
		"<svg>\x00</svg>",
	} {
		done := make(chan error, 1)
		go func() {
			_, err := Rasterize(doc, Options{})
			done <- err
		}()
		select {
		case err := <-done:
			if err == nil {
				t.Errorf("%q: accepted a malformed document", doc)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("%q: the pre-scan did not return", doc)
		}
	}
}
