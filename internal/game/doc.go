// Package game implements the synchronized in-memory game and its JaWS controls.
//
// [Manager] and [Room] methods may be called concurrently. Methods returning
// slices return shallow copies; the pointed-to cards, decks, players, and
// submissions remain shared records and must be treated as read-only unless a
// mutating method is provided.
package game
