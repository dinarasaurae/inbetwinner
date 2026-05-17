package services

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/dinarasaurae/inbetwin-social-service/internal/config"
	vkapi "github.com/dinarasaurae/inbetwin-social-service/internal/vk"
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
	if got := parsed.Query().Get("scope"); got != "messages,photos,docs" {
		t.Fatalf("scope = %q, want %q", got, "messages,photos,docs")
	}
}

func TestNormalizeVKCommunityScopesFallsBackToAllowedDefaults(t *testing.T) {
	if got := normalizeVKCommunityScopes("offline,market"); got != "manage,messages,photos,docs" {
		t.Fatalf("scope = %q, want full allowed default", got)
	}
}

func TestShouldSurfaceAdminGroupsErrorOn1051WithoutConnectedGroups(t *testing.T) {
	got := shouldSurfaceAdminGroupsError(
		errors.New("vk error 1051: Method is not available for this profile type"),
		0,
	)
	if !got {
		t.Fatalf("expected 1051 to surface when no connected groups exist")
	}
}

func TestShouldNotSurfaceAdminGroupsErrorWhenConnectedGroupsExist(t *testing.T) {
	got := shouldSurfaceAdminGroupsError(
		errors.New("vk error 1051: Method is not available for this profile type"),
		1,
	)
	if got {
		t.Fatalf("expected 1051 to stay hidden when connected groups exist")
	}
}

func TestIsInvalidGroupsOAuthError(t *testing.T) {
	if !isInvalidGroupsOAuthError(errors.New("vk oauth error: invalid_groups")) {
		t.Fatalf("expected invalid_groups to be detected")
	}
	if !isInvalidGroupsOAuthError(errors.New("only group admins have access to group tokens")) {
		t.Fatalf("expected group admin text to be detected")
	}
	if isInvalidGroupsOAuthError(errors.New("vk oauth error: invalid_request")) {
		t.Fatalf("unexpected match for unrelated oauth error")
	}
}

func TestIsAdminGroup(t *testing.T) {
	if !isAdminGroup(&vkapi.Group{IsAdmin: 1}) {
		t.Fatalf("expected IsAdmin=1 to count as admin")
	}
	if !isAdminGroup(&vkapi.Group{AdminLevel: 3}) {
		t.Fatalf("expected AdminLevel>0 to count as admin")
	}
	if isAdminGroup(&vkapi.Group{}) {
		t.Fatalf("expected zero-value group to not count as admin")
	}
}

func farFuture() time.Time {
	return time.Now().Add(10 * time.Minute)
}
