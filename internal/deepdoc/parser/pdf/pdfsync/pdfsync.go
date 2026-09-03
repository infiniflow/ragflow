//go:build cgo

// Package pdfsync provides a single process-wide mutex that serializes ALL
// access to the PDFium C library, regardless of which Go binding issues the
// call.
//
// Both the cgo pdfium binding (package pdfium) and the Rust pdf_oxide binding
// (package pdfoxide) are linked into the same final binary against the same
// PDFium static library. build.sh collapses the duplicate FPDF_* symbols with
// --allow-multiple-definition, so the process contains exactly ONE PDFium
// instance. PDFium is documented as NOT thread-safe for any call — concurrent
// calls corrupt the global heap and crash with SIGSEGV even when the calls
// operate on different documents.
//
// Therefore every native entry point in either binding must hold this one
// shared lock. Two independent mutexes would give the illusion of safety
// while still allowing a cgo pdfium call and a Rust pdf_oxide call to
// interleave onto the same PDFium instance. A single shared mutex is correct
// in both the one-instance and the (hypothetical, build-breaking) two-instance
// cases; two mutexes are only correct in the two-instance case.
package pdfsync

import "sync"

// Mu serializes all PDFium C API calls across every binding linked into the
// process. Acquire it (via With / WithErr) around every FPDF_* call and every
// pdf_oxide Rust call that re-enters the same PDFium instance.
var Mu sync.Mutex

// With runs f while holding the shared PDFium mutex.
func With(f func()) {
	Mu.Lock()
	defer Mu.Unlock()
	f()
}

// WithErr runs f while holding the shared PDFium mutex and propagates its
// error unchanged.
func WithErr(f func() error) error {
	Mu.Lock()
	defer Mu.Unlock()
	return f()
}
