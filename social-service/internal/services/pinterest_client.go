package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// pinterestUsernameRe extracts a Pinterest username from any pinterest.com URL.
// Handles: https://pinterest.com/user, https://www.pinterest.ru/user/, pinterest.com/user/boards
var pinterestUsernameRe = regexp.MustCompile(
	`(?i)(?:https?://)?(?:www\.)?pinterest\.(?:com|ru|co\.uk|fr|de|es|pt|jp)/([A-Za-z0-9_.-]+)/?`)

// PinterestClient calls the Pinterest API v5 using the workspace owner's
// OAuth access token.  The token is obtained once when the owner connects
// their Pinterest Business account and stored per-workspace.
//
// Key capabilities used for digital twin enrichment:
//   - GET /v5/users/{username}   → name, bio, follower_count, website
//   - GET /v5/boards?owner_username={username}&page_size=25 → board names = interests
type PinterestClient struct {
	accessToken string
	httpClient  *http.Client
}

func NewPinterestClient(accessToken string) *PinterestClient {
	return &PinterestClient{
		accessToken: accessToken,
		httpClient:  &http.Client{Timeout: 8 * time.Second},
	}
}

// Configured returns true when an access token is present.
func (c *PinterestClient) Configured() bool {
	return c != nil && c.accessToken != ""
}

// pinterestUser is the subset of Pinterest API /v5/users/{username} we care about.
type pinterestUser struct {
	Username       string `json:"username"`
	Name           string `json:"name"`
	Bio            string `json:"bio"`
	WebsiteURL     string `json:"website_url"`
	FollowerCount  int    `json:"follower_count"`
	FollowingCount int    `json:"following_count"`
	BoardCount     int    `json:"board_count"`
	PinCount       int    `json:"pin_count"`
}

// pinterestBoard is one item from GET /v5/boards.
type pinterestBoard struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// pinterestBoardsResp wraps the paginated boards response.
type pinterestBoardsResp struct {
	Items []pinterestBoard `json:"items"`
}

// GetLeadProfile fetches the public Pinterest profile and top board names
// for a lead identified by their Pinterest username.
// Returns (nil, nil) when the profile is not found (404).
func (c *PinterestClient) GetLeadProfile(ctx context.Context, username string) (*pinterestUser, []string, error) {
	if !c.Configured() {
		return nil, nil, nil
	}

	// 1. User profile.
	user, err := c.getUser(ctx, username)
	if err != nil {
		return nil, nil, err
	}
	if user == nil {
		return nil, nil, nil // 404 — profile not found / private
	}

	// 2. Board names — represent the person's interests.
	boards, err := c.getBoardNames(ctx, username)
	if err != nil {
		// Non-fatal: return the user profile without boards.
		return user, nil, nil
	}

	return user, boards, nil
}

func (c *PinterestClient) getUser(ctx context.Context, username string) (*pinterestUser, error) {
	reqURL := fmt.Sprintf("https://api.pinterest.com/v5/users/%s", url.PathEscape(username))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pinterest users.get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pinterest users.get: HTTP %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
	var user pinterestUser
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("pinterest decode user: %w", err)
	}
	return &user, nil
}

func (c *PinterestClient) getBoardNames(ctx context.Context, username string) ([]string, error) {
	reqURL := fmt.Sprintf(
		"https://api.pinterest.com/v5/boards?owner_username=%s&page_size=25",
		url.QueryEscape(username))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pinterest boards: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pinterest boards: HTTP %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
	var br pinterestBoardsResp
	if err := json.Unmarshal(body, &br); err != nil {
		return nil, err
	}

	var names []string
	for _, b := range br.Items {
		name := strings.TrimSpace(b.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// ExtractUsername parses a Pinterest username out of any Pinterest URL or
// bare username string. Returns "" when nothing matches.
func ExtractPinterestUsername(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	m := pinterestUsernameRe.FindStringSubmatch(raw)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}
