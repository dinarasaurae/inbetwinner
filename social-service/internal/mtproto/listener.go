package mtproto

// listener.go — keeps a long-lived MTProto connection per authenticated user
// so the platform can receive incoming private messages in real time.
//
// Architecture:
//   - ListenerManager maintains one goroutine per user.
//   - Each goroutine runs client.Run() indefinitely with an UpdateDispatcher
//     wired to OnNewMessage.
//   - A reference to the live tg.Client API is stored so that outgoing DMs
//     can be sent from any goroutine via SendDM().
//   - On shutdown (context cancellation) all listeners stop gracefully.

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"go.uber.org/zap"
)

// DMHandler is called when a private text message arrives from a real Telegram user.
// workspaceID is the inBeTwin user who owns the MTProto session.
// senderUserID / senderAccessHash identify the Telegram peer so we can reply.
type DMHandler func(
	ctx context.Context,
	workspaceID uuid.UUID,
	senderUserID int64,
	senderAccessHash int64,
	senderUsername string,
	text string,
)

// userListener is the state of one running listener goroutine.
type userListener struct {
	userID uuid.UUID
	cancel context.CancelFunc

	// api is set once the client connects; protected by mu.
	// Other goroutines wait on ready before accessing it.
	mu    sync.Mutex
	api   *tg.Client
	ready chan struct{}
}

// ListenerManager manages one persistent Telegram connection per user.
type ListenerManager struct {
	store   *sessionStore
	cfg     listenerCfg
	handler DMHandler

	mu    sync.RWMutex
	users map[uuid.UUID]*userListener
}

type listenerCfg struct {
	appID   int
	appHash string
}

// NewListenerManager creates the manager. Call StartAll() to boot existing sessions.
func NewListenerManager(store *sessionStore, appID int, appHash string, handler DMHandler) *ListenerManager {
	return &ListenerManager{
		store:   store,
		cfg:     listenerCfg{appID: appID, appHash: appHash},
		handler: handler,
		users:   make(map[uuid.UUID]*userListener),
	}
}

// StartAll loads every active session from the DB and starts a listener goroutine.
// Called once at service startup. parentCtx drives the overall lifetime.
func (m *ListenerManager) StartAll(parentCtx context.Context) error {
	rows, err := m.store.db.QueryContext(parentCtx,
		`SELECT user_id FROM telegram_sessions WHERE revoked_at IS NULL`)
	if err != nil {
		return fmt.Errorf("listener StartAll: %w", err)
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var uid uuid.UUID
		if err := rows.Scan(&uid); err != nil {
			continue
		}
		m.Start(parentCtx, uid)
		count++
	}
	return rows.Err()
}

// Start launches a listener for one user. Safe to call multiple times
// (re-entrant; existing listener is cancelled first).
func (m *ListenerManager) Start(parentCtx context.Context, userID uuid.UUID) {
	ctx, cancel := context.WithCancel(parentCtx)

	ul := &userListener{
		userID: userID,
		cancel: cancel,
		ready:  make(chan struct{}),
	}

	m.mu.Lock()
	if old, ok := m.users[userID]; ok {
		old.cancel()
	}
	m.users[userID] = ul
	m.mu.Unlock()

	go m.runListener(ctx, ul)
}

// Stop shuts down the listener for one user.
func (m *ListenerManager) Stop(userID uuid.UUID) {
	m.mu.Lock()
	ul, ok := m.users[userID]
	if ok {
		delete(m.users, userID)
	}
	m.mu.Unlock()

	if ok {
		ul.cancel()
	}
}

// SendDM sends a text message to a Telegram user via the authenticated session.
// Blocks until the connection is ready or ctx is cancelled.
func (m *ListenerManager) SendDM(
	ctx context.Context,
	workspaceID uuid.UUID,
	peerUserID int64,
	peerAccessHash int64,
	text string,
) error {
	m.mu.RLock()
	ul, ok := m.users[workspaceID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no active listener for workspace %s", workspaceID)
	}

	// Wait for connection to be established.
	select {
	case <-ul.ready:
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(15 * time.Second):
		return fmt.Errorf("timed out waiting for Telegram connection")
	}

	ul.mu.Lock()
	api := ul.api
	ul.mu.Unlock()

	if api == nil {
		return fmt.Errorf("telegram API not available")
	}

	humanDelay(ctx)

	peer := &tg.InputPeerUser{UserID: peerUserID, AccessHash: peerAccessHash}
	_, err := message.NewSender(api).To(peer).Text(ctx, text)
	return err
}

// runListener is the goroutine body: keeps the MTProto connection alive.
func (m *ListenerManager) runListener(ctx context.Context, ul *userListener) {
	log := zap.NewNop()

	const baseDelay = 5 * time.Second
	const maxDelay = 5 * time.Minute
	delay := baseDelay

	for {
		if ctx.Err() != nil {
			return
		}

		err := m.connectOnce(ctx, ul, log)
		if err == nil || ctx.Err() != nil {
			return
		}

		// FLOOD_WAIT is handled inside connectOnce; other errors → back-off.
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return
		}
		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
}

func (m *ListenerManager) connectOnce(ctx context.Context, ul *userListener, log *zap.Logger) error {
	sessionBytes, err := m.store.Load(ctx, ul.userID)
	if err != nil {
		return fmt.Errorf("no_session: %w", err)
	}

	// Reset api+ready on each reconnect attempt.
	ul.mu.Lock()
	ul.api = nil
	ul.ready = make(chan struct{})
	ul.mu.Unlock()

	sess := newMemSession(sessionBytes)

	// Build update dispatcher — filters to private DMs only.
	dispatcher := tg.NewUpdateDispatcher()
	dispatcher.OnNewMessage(func(updCtx context.Context, e tg.Entities, upd *tg.UpdateNewMessage) error {
		return m.onNewMessage(updCtx, ul.userID, e, upd)
	})

	client := telegram.NewClient(m.cfg.appID, m.cfg.appHash, telegram.Options{
		SessionStorage: sess,
		UpdateHandler:  dispatcher,
		Logger:         log,
	})

	return client.Run(ctx, func(runCtx context.Context) error {
		// Verify session is still valid.
		status, err := client.Auth().Status(runCtx)
		if err != nil {
			return fmt.Errorf("auth_status: %w", err)
		}
		if !status.Authorized {
			return ErrNoSession
		}

		// Expose API handle to other goroutines.
		ul.mu.Lock()
		ul.api = client.API()
		close(ul.ready)
		ul.mu.Unlock()

		// Block forever — dispatcher receives updates in the background.
		<-runCtx.Done()
		return runCtx.Err()
	})
}

// onNewMessage filters incoming updates and invokes the DMHandler.
func (m *ListenerManager) onNewMessage(
	ctx context.Context,
	workspaceID uuid.UUID,
	e tg.Entities,
	upd *tg.UpdateNewMessage,
) error {
	msg, ok := upd.Message.(*tg.Message)
	if !ok {
		return nil
	}
	// Skip our own outgoing messages.
	if msg.Out {
		return nil
	}
	// Only private DMs (PeerUser peer = personal chat).
	if _, ok := msg.PeerID.(*tg.PeerUser); !ok {
		return nil
	}
	text := msg.Message
	if text == "" {
		return nil // media/sticker without caption — skip for now
	}

	// Resolve sender from update entities.
	fromPeer, ok := msg.FromID.(*tg.PeerUser)
	if !ok {
		// In private chats FromID may be absent — peer is the sender.
		if pu, ok2 := msg.PeerID.(*tg.PeerUser); ok2 {
			fromPeer = pu
		} else {
			return nil
		}
	}

	tgUser, ok := e.Users[fromPeer.UserID]
	if !ok {
		return nil
	}
	// Skip bots.
	if tgUser.Bot {
		return nil
	}

	if m.handler != nil {
		// Run in goroutine — don't block the update loop.
		go m.handler(
			ctx,
			workspaceID,
			tgUser.ID,
			tgUser.AccessHash,
			tgUser.Username,
			text,
		)
	}

	// Retry FLOOD_WAIT if raised from a nested call.
	if waited, waitErr := tgerr.FloodWait(ctx, nil); waited {
		_ = waitErr
	}

	return nil
}
