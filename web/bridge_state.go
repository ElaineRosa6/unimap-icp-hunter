package web

import (
	"sync"

	"github.com/unimap-icp-hunter/project/internal/screenshot"
)

// BridgeState holds all screenshot bridge-related state.
type BridgeState struct {
	mu             sync.Mutex
	Service        *screenshot.BridgeService
	Mock           *bridgeMockClient
	Tokens         map[string]int64
	CallbackNonces map[string]int64
	LastSeen       map[string]int64
	LastErr        string
	LastAt         int64
}
