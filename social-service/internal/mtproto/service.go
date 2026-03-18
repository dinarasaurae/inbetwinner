package mtproto

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"go.uber.org/zap"

	"github.com/dinarasaurae/inbetwin-social-service/internal/config"
	"github.com/dinarasaurae/inbetwin-social-service/internal/crypto"
	"github.com/dinarasaurae/inbetwin-social-service/internal/database"
	"github.com/dinarasaurae/inbetwin-social-service/internal/models"
)

var (
	ErrNoSession       = errors.New("no_session: user has not authenticated via Telegram")
	ErrAuthPending     = errors.New("auth_pending: send-code was not called first")
	ErrAlreadyAuthed   = errors.New("already_authenticated")
	ErrSignInFailed    = errors.New("sign_in_failed")
	ErrNotChannelAdmin = errors.New("not_channel_admin")
)

// TelegramUser is returned after a successful sign-in.
type TelegramUser struct {
	TgUserID    int64
	TgUsername  string
	TgFirstName string
	TgLastName  string
}

// Channel is a Telegram channel/group where the user is admin.
type Channel struct {
	ID          int64
	AccessHash  int64
	Title       string
	Username    string
	MembersCount int32
}

// Service is the MTProto service — it acts as the authenticated user.
type Service struct {
	cfg     *config.Config
	store   *sessionStore
	manager *authManager
}

func NewService(db *database.DB, enc *crypto.Encryptor, cfg *config.Config) *Service {
	return &Service{
		cfg:     cfg,
		store:   newSessionStore(db, enc),
		manager: newAuthManager(),
	}
}

// ── Authentication ────────────────────────────────────────────────────────────

// SendCode initiates phone-number authentication with Telegram.
// It starts a goroutine that blocks waiting for the OTP from SignIn.
// Returns the phone_code_hash that the client must send back with SignIn.
func (s *Service) SendCode(ctx context.Context, userID uuid.UUID, phone string) (phoneCodeHash string, timeout int32, err error) {
	if s.cfg.TelegramAppID == 0 {
		return "", 0, errors.New("TELEGRAM_APP_ID not configured")
	}

	// Reject if already authenticated.
	if _, loadErr := s.store.Load(ctx, userID); loadErr == nil {
		return "", 0, ErrAlreadyAuthed
	}

	flowCtx, cancel := context.WithTimeout(context.Background(), authTTL)
	pf := s.manager.start(userID.String(), cancel)

	sess := newMemSession(nil) // fresh session — no existing auth

	client := telegram.NewClient(s.cfg.TelegramAppID, s.cfg.TelegramAppHash, telegram.Options{
		SessionStorage: sess,
		Logger:         zap.NewNop(),
	})

	// The goroutine keeps the MTProto connection alive between SendCode and SignIn.
	go func() {
		defer s.manager.complete(userID.String())
		defer cancel()

		runErr := client.Run(flowCtx, func(runCtx context.Context) error {
			// Step 1: send the OTP
			sentCodeRaw, sendErr := client.Auth().SendCode(runCtx, phone, auth.SendCodeOptions{})
			if sendErr != nil {
				pf.hashCh <- "" // unblock the HTTP handler
				return fmt.Errorf("send_code: %w", sendErr)
			}
			sentCode, ok := sentCodeRaw.(*tg.AuthSentCode)
			if !ok {
				pf.hashCh <- "" // unblock the HTTP handler
				return errors.New("unexpected sentCode type from Telegram")
			}

			// Signal the HTTP handler with the hash.
			pf.hashCh <- sentCode.PhoneCodeHash

			// Step 2: wait for the code from the sign-in HTTP handler.
			select {
			case code := <-pf.codeCh:
				authorization, signErr := client.Auth().SignIn(runCtx, phone, code, sentCode.PhoneCodeHash)
				if signErr != nil {
					pf.doneCh <- authResult{Err: fmt.Errorf("sign_in: %w", signErr)}
					return signErr
				}

				user, ok := authorization.User.(*tg.User)
				if !ok {
					pf.doneCh <- authResult{Err: errors.New("unexpected auth response type")}
					return errors.New("unexpected auth response type")
				}

				// Persist the encrypted session before signalling success.
				saveErr := s.store.Save(
					runCtx, userID,
					user.ID, user.Username, user.FirstName, user.LastName,
					sess.Snapshot(),
				)
				if saveErr != nil {
					pf.doneCh <- authResult{Err: saveErr}
					return saveErr
				}

				pf.doneCh <- authResult{
					TgUserID:    user.ID,
					TgUsername:  user.Username,
					TgFirstName: user.FirstName,
					TgLastName:  user.LastName,
				}
				return nil

			case <-runCtx.Done():
				return runCtx.Err()
			}
		})

		if runErr != nil && runErr != context.Canceled && runErr != context.DeadlineExceeded {
			// Ensure sign-in handler is unblocked if it's waiting.
			select {
			case pf.doneCh <- authResult{Err: runErr}:
			default:
			}
		}
	}()

	// Wait for the goroutine to deliver the hash (or fail).
	select {
	case hash := <-pf.hashCh:
		if hash == "" {
			return "", 0, ErrSignInFailed
		}
		return hash, int32(authTTL.Seconds()), nil
	case <-ctx.Done():
		cancel()
		return "", 0, ctx.Err()
	}
}

// SignIn completes authentication by providing the OTP code.
// The auth goroutine started by SendCode calls Telegram's auth.signIn and
// persists the resulting session to the database.
func (s *Service) SignIn(ctx context.Context, userID uuid.UUID, code string) (*TelegramUser, error) {
	pf, ok := s.manager.get(userID.String())
	if !ok {
		return nil, ErrAuthPending
	}

	// Send the OTP to the waiting goroutine.
	select {
	case pf.codeCh <- code:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Wait for the result.
	select {
	case result := <-pf.doneCh:
		if result.Err != nil {
			return nil, result.Err
		}
		return &TelegramUser{
			TgUserID:    result.TgUserID,
			TgUsername:  result.TgUsername,
			TgFirstName: result.TgFirstName,
			TgLastName:  result.TgLastName,
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SignOut revokes the session on Telegram's side and deletes it from the database.
func (s *Service) SignOut(ctx context.Context, userID uuid.UUID) error {
	err := s.run(ctx, userID, func(runCtx context.Context, client *telegram.Client) error {
		_, logoutErr := client.API().AuthLogOut(runCtx)
		return logoutErr
	})
	// Always revoke locally, even if Telegram call failed.
	_ = s.store.Revoke(ctx, userID)
	return err
}

// ── Channel discovery ─────────────────────────────────────────────────────────

// GetAdminChannels returns the channels where the user is creator or administrator.
func (s *Service) GetAdminChannels(ctx context.Context, userID uuid.UUID) ([]Channel, error) {
	var channels []Channel

	err := s.run(ctx, userID, func(runCtx context.Context, client *telegram.Client) error {
		// Fetch dialogs — paginate until we have all channels.
		var offsetDate int
		var offsetID int
		var offsetPeer tg.InputPeerClass = &tg.InputPeerEmpty{}

		for {
			result, err := client.API().MessagesGetDialogs(runCtx, &tg.MessagesGetDialogsRequest{
				Limit:      100,
				OffsetDate: offsetDate,
				OffsetID:   offsetID,
				OffsetPeer: offsetPeer,
				FolderID:   0,
			})
			if err != nil {
				return fmt.Errorf("get_dialogs: %w", err)
			}

			var chats []tg.ChatClass
			var more bool

			switch r := result.(type) {
			case *tg.MessagesDialogs:
				chats = r.Chats
			case *tg.MessagesDialogsSlice:
				chats = r.Chats
				more = len(r.Chats) == 100
			}

			for _, chat := range chats {
				ch, ok := chat.(*tg.Channel)
				if !ok {
					continue
				}
				// Only include channels (not groups) where user is admin/creator.
				_, hasAdminRights := ch.GetAdminRights()
				if ch.Broadcast && (ch.Creator || hasAdminRights) {
					channels = append(channels, Channel{
						ID:         ch.ID,
						AccessHash: ch.AccessHash,
						Title:      ch.Title,
						Username:   ch.Username,
					})
				}
			}

			if !more {
				break
			}
			// Set offset for next page using the last dialog.
			offsetDate++
		}
		return nil
	})

	return channels, err
}

// ── History import ─────────────────────────────────────────────────────────────

// ImportHistory fetches posts from a channel, from newest to oldest.
// Pass offsetMsgID=0 for a full import; pass the last known message_id for incremental sync.
func (s *Service) ImportHistory(
	ctx context.Context,
	userID uuid.UUID,
	integrationID uuid.UUID,
	channelID, accessHash int64,
	offsetMsgID int,
	limit int,
) ([]*models.TelegramPost, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	var posts []*models.TelegramPost

	err := s.run(ctx, userID, func(runCtx context.Context, client *telegram.Client) error {
		peer := &tg.InputPeerChannel{
			ChannelID:  channelID,
			AccessHash: accessHash,
		}

		result, err := client.API().MessagesGetHistory(runCtx, &tg.MessagesGetHistoryRequest{
			Peer:     peer,
			Limit:    limit,
			OffsetID: offsetMsgID,
		})
		if err != nil {
			return fmt.Errorf("get_history: %w", err)
		}

		var msgs []tg.MessageClass
		switch r := result.(type) {
		case *tg.MessagesMessages:
			msgs = r.Messages
		case *tg.MessagesMessagesSlice:
			msgs = r.Messages
		case *tg.MessagesChannelMessages:
			msgs = r.Messages
		}

		for _, msg := range msgs {
			m, ok := msg.(*tg.Message)
			if !ok {
				continue
			}
			posts = append(posts, messageToPost(m, integrationID, userID, channelID))
		}
		return nil
	})

	return posts, err
}

// ── Actions (as the authenticated user) ───────────────────────────────────────

// SendMessage posts a message to the channel as the user.
func (s *Service) SendMessage(
	ctx context.Context,
	userID uuid.UUID,
	channelID, accessHash int64,
	text string,
) (int64, error) {
	var msgID int64

	humanDelay(ctx) // avoid instant-bot pattern
	err := s.run(ctx, userID, func(runCtx context.Context, client *telegram.Client) error {
		peer := &tg.InputPeerChannel{ChannelID: channelID, AccessHash: accessHash}
		result, err := message.NewSender(client.API()).To(peer).Text(runCtx, text)
		if err != nil {
			return err
		}
		// Extract the message ID from the Updates result.
		switch u := result.(type) {
		case *tg.Updates:
			for _, upd := range u.Updates {
				if mu, ok := upd.(*tg.UpdateMessageID); ok {
					msgID = int64(mu.ID)
				}
			}
		}
		return nil
	})

	return msgID, err
}

// ReplyToComment replies to a comment in a channel post discussion.
// postMsgID is the channel post; replyToMsgID=0 means reply to the post itself.
func (s *Service) ReplyToComment(
	ctx context.Context,
	userID uuid.UUID,
	channelID, accessHash int64,
	postMsgID int,
	replyToMsgID int,
	text string,
) (int64, error) {
	var msgID int64

	humanDelay(ctx) // avoid instant-bot pattern
	err := s.run(ctx, userID, func(runCtx context.Context, client *telegram.Client) error {
		channelPeer := &tg.InputPeerChannel{ChannelID: channelID, AccessHash: accessHash}

		// Resolve the discussion (comment) group for this post.
		disc, err := client.API().MessagesGetDiscussionMessage(runCtx,
			&tg.MessagesGetDiscussionMessageRequest{
				Peer:  channelPeer,
				MsgID: postMsgID,
			})
		if err != nil {
			return fmt.Errorf("get_discussion: %w", err)
		}

		// The discussion happens in a linked group, not the channel itself.
		groupPeer := disc.Chats[0]
		linkedGroup, ok := groupPeer.(*tg.Channel)
		if !ok {
			return errors.New("discussion group is not a channel")
		}
		groupInputPeer := &tg.InputPeerChannel{
			ChannelID:  linkedGroup.ID,
			AccessHash: linkedGroup.AccessHash,
		}

		// replyToMsgID=0 → reply to the mirrored post in the discussion group.
		replyTo := disc.MaxID
		if replyToMsgID > 0 {
			replyTo = replyToMsgID
		}

		result, err := message.NewSender(client.API()).
			To(groupInputPeer).
			Reply(replyTo).
			Text(runCtx, text)
		if err != nil {
			return err
		}
		switch u := result.(type) {
		case *tg.Updates:
			for _, upd := range u.Updates {
				if mu, ok := upd.(*tg.UpdateMessageID); ok {
					msgID = int64(mu.ID)
				}
			}
		}
		return nil
	})

	return msgID, err
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// run loads the user's encrypted session, creates a gotd/td client with it,
// and executes fn inside the authenticated connection.
// Automatically retries on FLOOD_WAIT up to maxFloodRetries times.
const maxFloodRetries = 5

func (s *Service) run(
	ctx context.Context,
	userID uuid.UUID,
	fn func(ctx context.Context, client *telegram.Client) error,
) error {
	sessionBytes, err := s.store.Load(ctx, userID)
	if err != nil {
		return ErrNoSession
	}

	for attempt := 0; attempt < maxFloodRetries; attempt++ {
		sess := newMemSession(sessionBytes)

		client := telegram.NewClient(s.cfg.TelegramAppID, s.cfg.TelegramAppHash, telegram.Options{
			SessionStorage: sess,
			Logger:         zap.NewNop(),
		})

		runErr := client.Run(ctx, func(runCtx context.Context) error {
			status, err := client.Auth().Status(runCtx)
			if err != nil {
				return fmt.Errorf("auth_status: %w", err)
			}
			if !status.Authorized {
				return ErrNoSession
			}
			return fn(runCtx, client)
		})

		if runErr == nil {
			return nil
		}
		// FLOOD_WAIT — tgerr.FloodWait sleeps the required duration, then we retry.
		if waited, waitErr := tgerr.FloodWait(ctx, runErr); waited {
			continue
		} else if waitErr != nil {
			return waitErr // context cancelled during the wait
		}
		return runErr // real error, no retry
	}
	return fmt.Errorf("flood_wait: max retries (%d) exceeded", maxFloodRetries)
}

// humanDelay adds a small random pause (2–5 s) before outgoing messages
// so the account doesn't look like an instant-reply bot.
func humanDelay(ctx context.Context) {
	// math/rand is fine here — this is not security-sensitive.
	jitter := time.Duration(2000+time.Now().UnixNano()%3000) * time.Millisecond
	select {
	case <-time.After(jitter):
	case <-ctx.Done():
	}
}

// messageToPost converts a tg.Message into our TelegramPost model.
func messageToPost(m *tg.Message, integrationID, userID uuid.UUID, channelID int64) *models.TelegramPost {
	post := &models.TelegramPost{
		IntegrationID: integrationID,
		UserID:        userID,
		MessageID:     int64(m.ID),
		ChannelID:     channelID,
		PostedAt:      time.Unix(int64(m.Date), 0).UTC(),
	}

	if m.Message != "" {
		text := m.Message
		post.Text = &text
	}

	mediaType, hasMedia := detectTgMediaType(m)
	post.MediaType = mediaType
	post.HasMedia = hasMedia

	urls := extractTgURLs(m)
	post.ExtractedURLs = urls
	post.LinkCount = len(urls)

	if m.Views > 0 {
		v := m.Views
		post.ViewCount = &v
	}
	if m.Forwards > 0 {
		f := m.Forwards
		post.ForwardCount = &f
	}

	return post
}

func detectTgMediaType(m *tg.Message) (*string, bool) {
	if m.Media == nil {
		return nil, false
	}
	var t string
	switch m.Media.(type) {
	case *tg.MessageMediaPhoto:
		t = "photo"
	case *tg.MessageMediaDocument:
		t = "document"
	case *tg.MessageMediaPoll:
		t = "poll"
	case *tg.MessageMediaGeo:
		t = "geo"
	case *tg.MessageMediaWebPage:
		t = "webpage"
	default:
		t = "other"
	}
	return &t, true
}

func extractTgURLs(m *tg.Message) []string {
	seen := make(map[string]struct{})
	var urls []string

	addURL := func(u string) {
		if u == "" {
			return
		}
		if _, dup := seen[u]; dup {
			return
		}
		seen[u] = struct{}{}
		urls = append(urls, u)
	}

	for _, entity := range m.Entities {
		switch e := entity.(type) {
		case *tg.MessageEntityURL:
			runes := []rune(m.Message)
			if e.Offset >= 0 && e.Offset+e.Length <= len(runes) {
				addURL(string(runes[e.Offset : e.Offset+e.Length]))
			}
		case *tg.MessageEntityTextURL:
			addURL(e.URL)
		}
	}
	return urls
}
