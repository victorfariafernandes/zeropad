package acl

import (
	"testing"

	"zeropad-backend/adapters/db"
)

func TestValidateSlugPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		wantErr bool
	}{
		{"exact slug", "notes", false},
		{"nested exact slug", "team/eng/notes", false},
		{"trailing wildcard", "team/eng/*", false},
		{"bare wildcard", "*", true},
		{"empty", "", true},
		{"mid-path wildcard", "team/*/notes", true},
		{"double wildcard", "team/**", true},
		{"wildcard not at end", "team/*eng", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSlugPattern(tt.pattern)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateSlugPattern(%q) error = %v, wantErr %v", tt.pattern, err, tt.wantErr)
			}
		})
	}
}

func TestMatchesSlugPattern(t *testing.T) {
	tests := []struct {
		pattern string
		slug    string
		want    bool
	}{
		{"notes", "notes", true},
		{"notes", "other", false},
		{"team/eng/*", "team/eng/roadmap", true},
		{"team/eng/*", "team/eng/2026/q1", true},
		{"team/eng/*", "team/other", false},
		{"team/eng/*", "team/eng", false}, // prefix must include the trailing slash
	}
	for _, tt := range tests {
		if got := MatchesSlugPattern(tt.pattern, tt.slug); got != tt.want {
			t.Errorf("MatchesSlugPattern(%q, %q) = %v, want %v", tt.pattern, tt.slug, got, tt.want)
		}
	}
}

func TestDecide(t *testing.T) {
	readOnly := []db.ACLGrant{{SlugPattern: "team/eng/*", CanRead: true, CanWrite: false, CanDelete: false}}

	if !decide(readOnly, "team/eng/roadmap", ActionRead) {
		t.Error("expected read to be allowed for matching wildcard grant")
	}
	if decide(readOnly, "team/eng/roadmap", ActionWrite) {
		t.Error("expected write to be denied when grant only allows read")
	}
	if decide(readOnly, "team/other", ActionRead) {
		t.Error("expected read to be denied for a non-matching slug")
	}
	if decide(nil, "anything", ActionRead) {
		t.Error("expected no grants to deny access")
	}
}
