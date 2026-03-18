package mtproto

import (
	"sync"
	"time"
)

// authResult is the outcome of a completed sign-in.
type authResult struct {
	TgUserID    int64
	TgUsername  string
	TgFirstName string
	TgLastName  string
	Err         error
}

// pendingFlow tracks an in-progress phone authentication.
// The two HTTP requests (send-code and sign-in) communicate via channels:
//
//	send-code handler:  starts goroutine → goroutine sends hash → handler returns hash to client
//	sign-in  handler:   sends OTP to goroutine → goroutine calls auth.SignIn → returns result
type pendingFlow struct {
	// hashCh: auth goroutine → send-code HTTP handler (phone_code_hash from Telegram)
	hashCh chan string
	// codeCh: sign-in HTTP handler → auth goroutine (OTP the user typed)
	codeCh chan string
	// doneCh: auth goroutine → sign-in HTTP handler (success/failure)
	doneCh chan authResult
	// cancel terminates the goroutine if the user never completes sign-in
	cancel func()
	timer  *time.Timer
}

// authManager is a thread-safe registry of pending auth flows.
// Each entry lives for at most authTTL (5 min) — matching Telegram's OTP expiry.
type authManager struct {
	mu      sync.Mutex
	pending map[string]*pendingFlow // keyed by platform user ID (string)
}

const authTTL = 5 * time.Minute

func newAuthManager() *authManager {
	return &authManager{pending: make(map[string]*pendingFlow)}
}

// start registers a new pending flow for the given user and returns it.
// Any existing flow for the user is cancelled first.
func (m *authManager) start(userID string, cancel func()) *pendingFlow {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Cancel any previous incomplete attempt.
	if old, ok := m.pending[userID]; ok {
		old.timer.Stop()
		old.cancel()
	}

	pf := &pendingFlow{
		hashCh: make(chan string, 1),
		codeCh: make(chan string, 1),
		doneCh: make(chan authResult, 1),
		cancel: cancel,
	}
	pf.timer = time.AfterFunc(authTTL, func() {
		cancel()
		m.mu.Lock()
		delete(m.pending, userID)
		m.mu.Unlock()
	})

	m.pending[userID] = pf
	return pf
}

// get returns the pending flow for a user (nil, false if none).
func (m *authManager) get(userID string) (*pendingFlow, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	pf, ok := m.pending[userID]
	return pf, ok
}

// complete removes and cleans up the pending flow after sign-in succeeds or fails.
func (m *authManager) complete(userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if pf, ok := m.pending[userID]; ok {
		pf.timer.Stop()
		delete(m.pending, userID)
	}
}
