package mtproto

import (
	"context"
	"errors"
	"sync"

	"github.com/dinarasaurae/inbetwin-social-service/internal/crypto"
	"github.com/dinarasaurae/inbetwin-social-service/internal/database"
	"github.com/google/uuid"
)

// errNoSession is returned by memSession.LoadSession when no data is stored.
// gotd/td treats any LoadSession error as "start fresh session".
var errNoSession = errors.New("no session data")

// memSession is a thread-safe in-memory SessionStorage.
// Used during the auth flow (before the session is persisted) and
// as a carrier when loading a stored session for a single operation.
type memSession struct {
	mu   sync.Mutex
	data []byte
}

func newMemSession(initial []byte) *memSession {
	cp := make([]byte, len(initial))
	copy(cp, initial)
	return &memSession{data: cp}
}

func (s *memSession) LoadSession(_ context.Context) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.data) == 0 {
		return nil, errNoSession
	}
	cp := make([]byte, len(s.data))
	copy(cp, s.data)
	return cp, nil
}

func (s *memSession) StoreSession(_ context.Context, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = make([]byte, len(data))
	copy(s.data, data)
	return nil
}

// Snapshot returns a copy of the current session bytes.
func (s *memSession) Snapshot() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(s.data))
	copy(cp, s.data)
	return cp
}

// sessionStore handles loading and saving encrypted sessions in PostgreSQL.
type sessionStore struct {
	db  *database.DB
	enc *crypto.Encryptor
}

func newSessionStore(db *database.DB, enc *crypto.Encryptor) *sessionStore {
	return &sessionStore{db: db, enc: enc}
}

// Load decrypts and returns the session bytes for a user.
// Returns errNoSession if no row exists.
func (s *sessionStore) Load(ctx context.Context, userID uuid.UUID) ([]byte, error) {
	var enc, iv []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT session_enc, session_iv FROM telegram_sessions
		 WHERE user_id = $1 AND revoked_at IS NULL`,
		userID,
	).Scan(&enc, &iv)
	if err != nil {
		return nil, errNoSession
	}

	if s.enc == nil {
		// Dev mode: no encryption key — stored raw
		return enc, nil
	}
	return s.enc.Decrypt(enc, iv)
}

// Save encrypts and upserts session bytes for a user.
func (s *sessionStore) Save(
	ctx context.Context,
	userID uuid.UUID,
	tgUserID int64,
	tgUsername, tgFirstName, tgLastName string,
	sessionData []byte,
) error {
	var enc, iv []byte
	var err error

	if s.enc != nil {
		enc, iv, err = s.enc.Encrypt(sessionData)
		if err != nil {
			return err
		}
	} else {
		// Dev mode: store raw
		enc = sessionData
		iv = []byte("DEV_NO_IV_TWELVE!")
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO telegram_sessions
			(user_id, tg_user_id, tg_username, tg_first_name, tg_last_name,
			 session_enc, session_iv, last_used_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,NOW())
		 ON CONFLICT (user_id) DO UPDATE SET
			tg_user_id    = EXCLUDED.tg_user_id,
			tg_username   = EXCLUDED.tg_username,
			tg_first_name = EXCLUDED.tg_first_name,
			tg_last_name  = EXCLUDED.tg_last_name,
			session_enc   = EXCLUDED.session_enc,
			session_iv    = EXCLUDED.session_iv,
			last_used_at  = NOW(),
			revoked_at    = NULL`,
		userID, tgUserID,
		nullStr(tgUsername), nullStr(tgFirstName), nullStr(tgLastName),
		enc, iv,
	)
	return err
}

// Revoke marks the session as revoked (caller must also sign out from Telegram).
func (s *sessionStore) Revoke(ctx context.Context, userID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE telegram_sessions SET revoked_at = NOW()
		 WHERE user_id = $1 AND revoked_at IS NULL`,
		userID,
	)
	return err
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
