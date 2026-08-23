package game

// Options configures a [Manager].
type Options struct {
	// MinPlayers is the number of players required to start a game.
	//
	// Values below two use [MinPlayers].
	MinPlayers int

	// Debug allows a target score of one and forces the highest-pick prompt to open.
	Debug bool

	// Dirty publishes changed dependency tags after game state locks are released.
	//
	// [NewManagerWithOptions] captures it at construction and never replaces it.
	// It may be called concurrently. A nil callback disables publication.
	Dirty func(tags ...any)
}
