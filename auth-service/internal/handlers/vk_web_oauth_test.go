package handlers

import (
	"net/url"
	"testing"
)

func TestBuildVKWebOAuthAuthorizeURL(t *testing.T) {
	authURL := buildVKWebOAuthAuthorizeURL(
		"54511648",
		"https://inbetwin.ru/api/v1/auth/vk/callback",
		"state-123",
	)

	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}

	if parsed.Scheme != "https" || parsed.Host != "oauth.vk.com" || parsed.Path != "/oauth/authorize" {
		t.Fatalf("unexpected auth URL: %s", authURL)
	}

	q := parsed.Query()
	if got := q.Get("client_id"); got != "54511648" {
		t.Fatalf("client_id = %q", got)
	}
	if got := q.Get("redirect_uri"); got != "https://inbetwin.ru/api/v1/auth/vk/callback" {
		t.Fatalf("redirect_uri = %q", got)
	}
	if got := q.Get("response_type"); got != "code" {
		t.Fatalf("response_type = %q", got)
	}
	if got := q.Get("state"); got != "state-123" {
		t.Fatalf("state = %q", got)
	}
}

func TestAllowedVKAuthReturnURL(t *testing.T) {
	if !isAllowedVKAuthReturnURL(
		"https://inbetwin.ru/auth/vk/callback",
		"https://inbetwin.ru",
		nil,
	) {
		t.Fatalf("expected frontend origin return URL to be allowed")
	}

	if !isAllowedVKAuthReturnURL(
		"http://localhost:8080/auth/vk/callback",
		"https://inbetwin.ru",
		nil,
	) {
		t.Fatalf("expected localhost return URL to be allowed for local web testing")
	}

	if !isAllowedVKAuthReturnURL(
		"https://preview.example.com/auth/vk/callback",
		"https://inbetwin.ru",
		[]string{"https://preview.example.com/auth/vk/callback"},
	) {
		t.Fatalf("expected explicitly allowed return URL to be allowed")
	}

	if isAllowedVKAuthReturnURL(
		"https://evil.example.com/auth/vk/callback",
		"https://inbetwin.ru",
		nil,
	) {
		t.Fatalf("unexpectedly allowed external return URL")
	}
}

func TestVKAuthReturnURLUsesFragment(t *testing.T) {
	returnURL := vkAuthReturnURLWithTokens(
		"http://localhost:8080/auth/vk/callback",
		"state-123",
		"access",
		"refresh",
		900,
	)

	parsed, err := url.Parse(returnURL)
	if err != nil {
		t.Fatalf("parse return URL: %v", err)
	}

	fragment, err := url.ParseQuery(parsed.Fragment)
	if err != nil {
		t.Fatalf("parse fragment: %v", err)
	}

	if parsed.RawQuery != "" {
		t.Fatalf("expected tokens in fragment, got query %q", parsed.RawQuery)
	}
	if got := fragment.Get("access_token"); got != "access" {
		t.Fatalf("access_token = %q", got)
	}
	if got := fragment.Get("refresh_token"); got != "refresh" {
		t.Fatalf("refresh_token = %q", got)
	}
}
