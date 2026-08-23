package game

// FinalScore is one participant's captured result from a completed game.
type FinalScore struct {
	// Player identifies the participant and remains a shared live record.
	Player *Player
	// Nickname is the participant's name when the result was captured.
	Nickname string
	// Score is the participant's score when the result was captured.
	Score int
	// IsWinner reports whether the participant won the game.
	IsWinner bool
}
