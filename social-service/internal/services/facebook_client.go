package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// FacebookClient calls the Facebook Graph API using the workspace owner's
// Page Access Token (obtained when connecting the Facebook Page / Messenger).
//
// For digital twin enrichment we use the Messenger Profile endpoint:
//   GET /{psid}?fields=name,profile_pic&access_token={page_token}
// where {psid} is the Page-Scoped User ID that arrives in every
// Messenger webhook event as sender.id.
//
// Permissions required on the Page token: pages_messaging, pages_read_engagement.
type FacebookClient struct {
	pageToken  string
	httpClient *http.Client
}

func NewFacebookClient(pageToken string) *FacebookClient {
	return &FacebookClient{
		pageToken:  pageToken,
		httpClient: &http.Client{Timeout: 8 * time.Second},
	}
}

// Configured returns true when a page token is present.
func (c *FacebookClient) Configured() bool {
	return c != nil && c.pageToken != ""
}

// MessengerProfile is the subset of Graph API user data relevant for enrichment.
type MessengerProfile struct {
	Name       string `json:"name"`
	ProfilePic string `json:"profile_pic"`
}

// GetMessengerProfile fetches the name and profile picture of a Messenger user
// by their Page-Scoped User ID (PSID).
// Returns (nil, nil) on 404 or when the client is not configured.
func (c *FacebookClient) GetMessengerProfile(ctx context.Context, psid string) (*MessengerProfile, error) {
	if !c.Configured() {
		return nil, nil
	}

	reqURL := fmt.Sprintf(
		"https://graph.facebook.com/v19.0/%s?fields=name,profile_pic&access_token=%s",
		url.PathEscape(psid),
		url.QueryEscape(c.pageToken),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("facebook graph: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("facebook graph HTTP %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))

	// Graph API returns {"error":{...}} on auth problems even with 200.
	var errCheck struct {
		Error *struct{ Message string } `json:"error"`
	}
	_ = json.Unmarshal(body, &errCheck)
	if errCheck.Error != nil {
		return nil, fmt.Errorf("facebook graph: %s", errCheck.Error.Message)
	}

	var profile MessengerProfile
	if err := json.Unmarshal(body, &profile); err != nil {
		return nil, fmt.Errorf("facebook decode: %w", err)
	}
	return &profile, nil
}
