package main

import "testing"

func TestIsApproved(t *testing.T) {
	tests := []struct {
		name   string
		status reviewStatus
		want   bool
	}{
		{
			name:   "decision approved",
			status: reviewStatus{Decision: "APPROVED"},
			want:   true,
		},
		{
			name:   "review approved with no decision",
			status: reviewStatus{Decision: "", Reviews: []Review{{State: "APPROVED"}}},
			want:   true,
		},
		{
			name:   "two approvals with no decision",
			status: reviewStatus{Decision: "", Reviews: []Review{{State: "APPROVED"}, {State: "APPROVED"}}},
			want:   true,
		},
		{
			name:   "approval after comments",
			status: reviewStatus{Decision: "APPROVED", Reviews: []Review{{State: "COMMENTED"}, {State: "APPROVED"}}},
			want:   true,
		},
		{
			name:   "approval after a dismissal",
			status: reviewStatus{Decision: "", Reviews: []Review{{State: "DISMISSED"}, {State: "APPROVED"}}},
			want:   true,
		},
		{
			name:   "no reviews at all",
			status: reviewStatus{Decision: ""},
			want:   false,
		},
		{
			name:   "review required",
			status: reviewStatus{Decision: "REVIEW_REQUIRED"},
			want:   false,
		},
		{
			name:   "comments only",
			status: reviewStatus{Decision: "", Reviews: []Review{{State: "COMMENTED"}}},
			want:   false,
		},
		{
			name:   "dismissed approval only",
			status: reviewStatus{Decision: "REVIEW_REQUIRED", Reviews: []Review{{State: "DISMISSED"}}},
			want:   false,
		},
		{
			name:   "changes requested overrides an earlier approval",
			status: reviewStatus{Decision: "CHANGES_REQUESTED", Reviews: []Review{{State: "APPROVED"}, {State: "CHANGES_REQUESTED"}}},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isApproved(tt.status); got != tt.want {
				t.Errorf("isApproved(%+v) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestPRKey(t *testing.T) {
	pr := PR{Number: 123, Repository: Repository{NameWithOwner: "envato/repo"}}
	if got := pr.Key(); got != "envato/repo#123" {
		t.Errorf("Key() = %q, want %q", got, "envato/repo#123")
	}
}

func TestParseKey(t *testing.T) {
	tests := []struct {
		key        string
		wantRepo   string
		wantNumber int
		wantErr    bool
	}{
		{key: "envato/repo#123", wantRepo: "envato/repo", wantNumber: 123},
		{key: "umputun/revdiff#261", wantRepo: "umputun/revdiff", wantNumber: 261},
		{key: "envato/repo", wantErr: true},
		{key: "envato/repo#", wantErr: true},
		{key: "envato/repo#abc", wantErr: true},
		{key: "#123", wantErr: true},
		{key: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			repo, number, err := parseKey(tt.key)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseKey(%q) = (%q, %d, nil), want error", tt.key, repo, number)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseKey(%q): %v", tt.key, err)
			}
			if repo != tt.wantRepo || number != tt.wantNumber {
				t.Errorf("parseKey(%q) = (%q, %d), want (%q, %d)", tt.key, repo, number, tt.wantRepo, tt.wantNumber)
			}
		})
	}
}

// Round-tripping matters because merge detection looks up a departed PR using
// only the key stored on an earlier poll.
func TestKeyRoundTrip(t *testing.T) {
	pr := PR{Number: 292, Repository: Repository{NameWithOwner: "umputun/revdiff"}}

	repo, number, err := parseKey(pr.Key())
	if err != nil {
		t.Fatalf("parseKey: %v", err)
	}
	if repo != pr.Repository.NameWithOwner || number != pr.Number {
		t.Errorf("round trip gave (%q, %d), want (%q, %d)", repo, number, pr.Repository.NameWithOwner, pr.Number)
	}
}
