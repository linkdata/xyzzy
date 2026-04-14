package game

import (
	"sync"

	"github.com/linkdata/xyzzy/internal/deck"
)

type Manager struct {
	mu      sync.RWMutex
	rooms   map[string]*Room
	catalog *deck.Catalog
	opts    Options
	dirty   func(...any)
}
