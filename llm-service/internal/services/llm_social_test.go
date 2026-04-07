package services

import "testing"

func TestParseVKDecisionContentJSON(t *testing.T) {
	content := `{"mode":"draft","draft_text":"Здравствуйте!","confidence":0.83,"intent":"faq","safe_intent":true,"rationale":"direct json"}`

	parsed, err := parseVKDecisionContent(content)
	if err != nil {
		t.Fatalf("parseVKDecisionContent returned error: %v", err)
	}
	if parsed.Mode != "draft" {
		t.Fatalf("Mode = %q, want draft", parsed.Mode)
	}
	if parsed.DraftText != "Здравствуйте!" {
		t.Fatalf("DraftText = %q, want greeting", parsed.DraftText)
	}
	if parsed.Intent != "faq" {
		t.Fatalf("Intent = %q, want faq", parsed.Intent)
	}
	if !parsed.SafeIntent {
		t.Fatalf("SafeIntent = false, want true")
	}
}

func TestParseVKDecisionContentFencedJSON(t *testing.T) {
	content := "```json\n{\"mode\":\"auto_reply\",\"draft_text\":\"Да, поможем.\",\"confidence\":0.91,\"intent\":\"qualification\",\"safe_intent\":true,\"rationale\":\"fenced json\"}\n```"

	parsed, err := parseVKDecisionContent(content)
	if err != nil {
		t.Fatalf("parseVKDecisionContent returned error: %v", err)
	}
	if parsed.Mode != "auto_reply" {
		t.Fatalf("Mode = %q, want auto_reply", parsed.Mode)
	}
	if parsed.Rationale != "fenced json" {
		t.Fatalf("Rationale = %q, want fenced json", parsed.Rationale)
	}
}
