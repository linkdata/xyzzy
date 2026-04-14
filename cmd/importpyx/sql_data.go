package main

import "github.com/linkdata/xyzzy/internal/deck"

type sqlData struct {
	blackCards     map[int]deck.BlackCard
	whiteCards     map[int]deck.WhiteCard
	decks          map[int]deckRecord
	deckBlackLinks map[int][]int
	deckWhiteLinks map[int][]int
}
