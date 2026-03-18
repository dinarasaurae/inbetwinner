package vk

import (
	"context"
	"encoding/json"
	"log"
	"time"
)

// MessageHandler is called for each incoming VK message received via Long Poll.
type MessageHandler func(ctx context.Context, groupID int64, msg IncomingMessage)

// Worker polls VK Long Poll for group events and calls h for each message_new.
type Worker struct {
	client *Client
	groupID int64
	handler MessageHandler
	// getTsFunc returns the last persisted ts for this group (called on startup).
	getTsFunc func() string
	// saveTsFunc persists the new ts after each successful poll batch.
	saveTsFunc func(ts string)
}

// NewWorker creates a new Long Poll worker for the given group.
func NewWorker(
	client *Client,
	groupID int64,
	handler MessageHandler,
	getTs func() string,
	saveTs func(ts string),
) *Worker {
	return &Worker{
		client:     client,
		groupID:    groupID,
		handler:    handler,
		getTsFunc:  getTs,
		saveTsFunc: saveTs,
	}
}

// Run starts the Long Poll loop. Blocks until ctx is cancelled.
// Automatically reconnects on transient errors and handles failed codes.
func (w *Worker) Run(ctx context.Context) {
	log.Printf("[vk-lp] group %d: starting Long Poll worker", w.groupID)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[vk-lp] group %d: worker stopped", w.groupID)
			return
		default:
		}

		if err := w.runLoop(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("[vk-lp] group %d: error %v, reconnecting in 5s", w.groupID, err)
			select {
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
				return
			}
		}
	}
}

func (w *Worker) runLoop(ctx context.Context) error {
	// Fetch fresh Long Poll server params.
	server, err := w.client.GroupsGetLongPollServer(ctx, w.groupID)
	if err != nil {
		return err
	}

	// Use the persisted ts if it is newer than the server default.
	if saved := w.getTsFunc(); saved != "" && saved > server.Ts {
		server.Ts = saved
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		update, err := w.client.PollEvents(ctx, *server)
		if err != nil {
			return err
		}

		switch update.Failed {
		case 0:
			// Normal — process events.
		case 1:
			// Ts outdated — use the new ts returned.
			server.Ts = update.Ts
			continue
		case 2, 3:
			// Key or server expired — fetch new params.
			return nil // triggers reconnect in outer loop
		default:
			return nil
		}

		for _, event := range update.Updates {
			if event.Type == "message_new" {
				var mn MessageNew
				if err := json.Unmarshal(event.Object, &mn); err == nil {
					// Only handle messages from real users (positive from_id).
					if mn.Message.FromID > 0 {
						w.handler(ctx, w.groupID, mn.Message)
					}
				}
			}
		}

		server.Ts = update.Ts
		w.saveTsFunc(server.Ts)
	}
}
