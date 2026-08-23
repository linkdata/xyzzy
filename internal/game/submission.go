package game

import "github.com/linkdata/xyzzy/internal/deck"

// Submission is one player's immutable response in a round.
//
// Rooms create and own submissions. Callers must use [Room.SubmissionCards]
// when they need a mutable copy of the card slice.
type Submission struct {
	// ID identifies the submission within its round.
	ID string
	// Player identifies the submitting participant.
	Player *Player
	// Cards preserves the submitted card order and must be treated as read-only.
	Cards []*deck.WhiteCard
}
