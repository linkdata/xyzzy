// Package deck loads card and deck definitions into an in-memory catalog.
//
// Callers must treat a successfully loaded [Catalog] and its contents as
// immutable. They support concurrent reads, and returned card and deck pointers
// are read-only.
package deck
