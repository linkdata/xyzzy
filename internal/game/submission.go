package game

import "github.com/linkdata/xyzzy/internal/deck"

type Submission struct {
	ID     string
	Player *Player
	Cards  []*deck.WhiteCard
}
