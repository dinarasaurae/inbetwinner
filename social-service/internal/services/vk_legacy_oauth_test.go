package services

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/dinarasaurae/inbetwin-social-service/internal/config"
)

func TestUserLegacyOAuthStartAndroidUsesCommunityRedirect(t *testing.T) {
	svc := &VKService{
		cfg: &config.Config{
			VKWebAppID:         "54511648",
			VKGroupRedirectURI: "https://inbetwin.ru/api/v1/social/vk/oauth/callback",
			VKWebRedirectURI:   "https://inbetwin.ru/api/v1/social/vk/oauth/user/callback",
		},
		oauthStates: make(map[string]pendingOAuth),
	}

	authURL, state, implicit, err := svc.UserLegacyOAuthStart(context.Background(), uuid.New(), "android")
	if err != nil {
		t.Fatalf("UserLegacyOAuthStart: %v", err)
	}
	if implicit {
		t.Fatalf("implicit = true, want false")
	}
	if state == "" {
		t.Fatalf("state is empty")
	}

	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	if got := parsed.Query().Get("redirect_uri"); got != svc.cfg.VKGroupRedirectURI {
		t.Fatalf("redirect_uri = %q, want %q", got, svc.cfg.VKGroupRedirectURI)
	}
	if got := parsed.Query().Get("client_id"); got != svc.cfg.VKWebAppID {
		t.Fatalf("client_id = %q, want %q", got, svc.cfg.VKWebAppID)
	}
}

func TestIsUserOAuthStateRecognizesGroupZero(t *testing.T) {
	svc := &VKService{
		oauthStates: map[string]pendingOAuth{
			"user":  {groupID: 0, expiry: farFuture()},
			"group": {groupID: 123, expiry: farFuture()},
		},
	}

	if !svc.IsUserOAuthState("user") {
		t.Fatalf("user state not recognized")
	}
	if svc.IsUserOAuthState("group") {
		t.Fatalf("group state misrecognized as user oauth")
	}
	if svc.IsUserOAuthState("missing") {
		t.Fatalf("missing state should be false")
	}
}

func TestOAuthStartSanitizesCommunityScopes(t *testing.T) {
	svc := &VKService{
		cfg: &config.Config{
			VKWebAppID:         "54511648",
			VKGroupRedirectURI: "https://inbetwin.ru/api/v1/social/vk/oauth/callback",
			VKCommunityScopes:  "messages,wall,photos,offline,market,docs",
		},
		oauthStates: make(map[string]pendingOAuth),
	}

	authURL, state, err := svc.OAuthStart(context.Background(), uuid.New(), 42, "android")
	if err != nil {
		t.Fatalf("OAuthStart: %v", err)
	}
	if state == "" {
		t.Fatalf("state is empty")
	}

	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	if got := parsed.Query().Get("scope"); got != "messages,photos,docs,wall" {
		t.Fatalf("scope = %q, want %q", got, "messages,photos,docs,wall")
	}
}

func TestNormalizeVKCommunityScopesFallsBackToAllowedDefaults(t *testing.T) {
	if got := normalizeVKCommunityScopes("offline,market"); got != "messages,manage,photos,docs,wall,stories" {
		t.Fatalf("scope = %q, want full allowed default", got)
	}
}

func farFuture() time.Time {
	return time.Now().Add(10 * time.Minute)
}
