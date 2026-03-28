package vk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client is a lightweight VK API HTTP client.
// It uses a community (group) access token for group-scoped operations.
type Client struct {
	token   string
	groupID int64
	http    *http.Client
}

// NewClient creates a new VK API client with the given group token.
func NewClient(groupToken string, groupID int64) *Client {
	return &Client{
		token:   groupToken,
		groupID: groupID,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// NewUserClient creates a client with a user token (for users.get, wall.get, groups.get).
func NewUserClient(userToken string) *Client {
	return &Client{
		token: userToken,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ─── Core request helper ──────────────────────────────────────────────────────

func (c *Client) call(ctx context.Context, method string, params url.Values) (json.RawMessage, error) {
	params.Set("access_token", c.token)
	params.Set("v", APIVersion)

	reqURL := APIBase + "/" + method + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("vk: build request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vk: %s: %w", method, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("vk: read body: %w", err)
	}

	// Check for API-level error.
	var envelope struct {
		Error *APIError `json:"error"`
	}
	if json.Unmarshal(body, &envelope) == nil && envelope.Error != nil {
		return nil, envelope.Error
	}

	// Extract the "response" field.
	var wrapper struct {
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("vk: parse response: %w", err)
	}
	return wrapper.Response, nil
}

// ─── Users ────────────────────────────────────────────────────────────────────

// UsersGet fetches public profiles for the given VK user IDs.
func (c *Client) UsersGet(ctx context.Context, userIDs []int64) ([]User, error) {
	ids := make([]string, len(userIDs))
	for i, id := range userIDs {
		ids[i] = strconv.FormatInt(id, 10)
	}
	p := url.Values{
		"user_ids": {strings.Join(ids, ",")},
		"fields":   {UserFields},
	}
	raw, err := c.call(ctx, "users.get", p)
	if err != nil {
		return nil, err
	}
	var users []User
	return users, json.Unmarshal(raw, &users)
}

// ─── Groups ───────────────────────────────────────────────────────────────────

// UsersGetMe fetches the current user's own profile (requires a user token).
func (c *Client) UsersGetMe(ctx context.Context) (*User, error) {
	p := url.Values{
		"fields": {UserFields},
	}
	raw, err := c.call(ctx, "users.get", p)
	if err != nil {
		return nil, err
	}
	var users []User
	if err := json.Unmarshal(raw, &users); err != nil || len(users) == 0 {
		return nil, fmt.Errorf("vk: users.get returned empty response")
	}
	return &users[0], nil
}

// UsersGetSubscriptions returns the users and groups the authenticated user follows.
// Returns an extended list (objects, not just IDs).
func (c *Client) UsersGetSubscriptions(ctx context.Context, count int) (*SubscriptionsExtendedResponse, error) {
	if count <= 0 || count > 200 {
		count = 100
	}
	p := url.Values{
		"extended": {"1"},
		"count":    {strconv.Itoa(count)},
		"fields":   {"photo_200,screen_name"},
	}
	raw, err := c.call(ctx, "users.getSubscriptions", p)
	if err != nil {
		return nil, err
	}
	var resp SubscriptionsExtendedResponse
	return &resp, json.Unmarshal(raw, &resp)
}

// GroupsGetAdmin returns groups where the authenticated user is an administrator.
func (c *Client) GroupsGetAdmin(ctx context.Context) ([]Group, error) {
	p := url.Values{
		"filter":   {"admin"},
		"fields":   {GroupFields},
		"extended": {"1"},
		"count":    {"1000"},
	}
	raw, err := c.call(ctx, "groups.get", p)
	if err != nil {
		return nil, err
	}
	var resp GroupsGetResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("vk: parse groups: %w", err)
	}
	return resp.Items, nil
}

// GroupsGetByID fetches info about a specific group.
func (c *Client) GroupsGetByID(ctx context.Context, groupID int64) (*Group, error) {
	p := url.Values{
		"group_ids": {strconv.FormatInt(groupID, 10)},
		"fields":    {GroupFields},
	}
	raw, err := c.call(ctx, "groups.getById", p)
	if err != nil {
		return nil, err
	}
	var groups []Group
	if err := json.Unmarshal(raw, &groups); err != nil || len(groups) == 0 {
		return nil, fmt.Errorf("vk: group not found or parse error")
	}
	return &groups[0], nil
}

// ─── Wall ─────────────────────────────────────────────────────────────────────

// WallGet fetches posts from the owner's wall (ownerID is negative for groups).
func (c *Client) WallGet(ctx context.Context, ownerID int64, count, offset int) (*WallGetResponse, error) {
	p := url.Values{
		"owner_id": {strconv.FormatInt(ownerID, 10)},
		"count":    {strconv.Itoa(count)},
		"offset":   {strconv.Itoa(offset)},
		"filter":   {"owner"},
	}
	raw, err := c.call(ctx, "wall.get", p)
	if err != nil {
		return nil, err
	}
	var resp WallGetResponse
	return &resp, json.Unmarshal(raw, &resp)
}

// ─── Messages ─────────────────────────────────────────────────────────────────

// MessagesSend sends a message to a VK user from the group.
func (c *Client) MessagesSend(ctx context.Context, toUserID int64, text string) (int64, error) {
	p := url.Values{
		"user_id":   {strconv.FormatInt(toUserID, 10)},
		"message":   {text},
		"group_id":  {strconv.FormatInt(c.groupID, 10)},
		"random_id": {strconv.FormatInt(time.Now().UnixNano(), 10)},
	}
	raw, err := c.call(ctx, "messages.send", p)
	if err != nil {
		return 0, err
	}
	var msgID int64
	return msgID, json.Unmarshal(raw, &msgID)
}

// ─── Long Poll setup ──────────────────────────────────────────────────────────

// GroupsGetLongPollServer fetches the Long Poll server parameters for a group.
func (c *Client) GroupsGetLongPollServer(ctx context.Context, groupID int64) (*LongPollServer, error) {
	p := url.Values{
		"group_id": {strconv.FormatInt(groupID, 10)},
	}
	raw, err := c.call(ctx, "groups.getLongPollServer", p)
	if err != nil {
		return nil, err
	}
	var server LongPollServer
	return &server, json.Unmarshal(raw, &server)
}

// PollEvents polls the LP server once and returns raw events.
// Returns the next ts value and the list of events.
func (c *Client) PollEvents(ctx context.Context, server LongPollServer) (*GroupLongPollUpdate, error) {
	params := url.Values{
		"act":  {"a_check"},
		"key":  {server.Key},
		"ts":   {server.Ts},
		"wait": {strconv.Itoa(LongPollWait)},
	}
	pollURL := server.Server + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pollURL, nil)
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{Timeout: time.Duration(LongPollWait+5) * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vk lp: poll: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("vk lp: read: %w", err)
	}

	var update GroupLongPollUpdate
	if err := json.Unmarshal(body, &update); err != nil {
		return nil, fmt.Errorf("vk lp: parse: %w", err)
	}
	return &update, nil
}
