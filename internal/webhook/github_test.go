package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerifySignature(t *testing.T) {
	t.Parallel()

	body := []byte(`{"action":"opened"}`)
	secret := "test-secret"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if err := VerifySignature(secret, signature, body); err != nil {
		t.Fatalf("VerifySignature() error = %v", err)
	}
}

func TestParseDeliveryPullRequestOpened(t *testing.T) {
	t.Parallel()

	delivery, err := ParseDelivery(
		"pull_request",
		"delivery-1",
		[]byte(`{
			"action":"opened",
			"number":42,
			"repository":{"id":123456},
			"pull_request":{"id":987654,"head":{"sha":"abc123"}}
		}`),
	)
	if err != nil {
		t.Fatalf("ParseDelivery() error = %v", err)
	}

	if !delivery.Supported {
		t.Fatalf("ParseDelivery() Supported = false, want true")
	}
	if delivery.EventType != EventTypePullRequestOpened {
		t.Fatalf("ParseDelivery() EventType = %q, want %q", delivery.EventType, EventTypePullRequestOpened)
	}
	if delivery.Event.PRNumber != 42 {
		t.Fatalf("ParseDelivery() PRNumber = %d, want 42", delivery.Event.PRNumber)
	}
	if delivery.Event.GithubPullRequestID != 987654 {
		t.Fatalf("ParseDelivery() GithubPullRequestID = %d, want 987654", delivery.Event.GithubPullRequestID)
	}
	if delivery.Event.PRHeadSHA != "abc123" {
		t.Fatalf("ParseDelivery() PRHeadSHA = %q, want %q", delivery.Event.PRHeadSHA, "abc123")
	}
}

func TestParseDeliveryIssueCommentOnIssueIsUnsupported(t *testing.T) {
	t.Parallel()

	delivery, err := ParseDelivery(
		"issue_comment",
		"delivery-2",
		[]byte(`{
			"action":"created",
			"repository":{"id":123456},
			"issue":{"number":42},
			"comment":{"id":99,"body":"/deploy","user":{"login":"octocat"}}
		}`),
	)
	if err != nil {
		t.Fatalf("ParseDelivery() error = %v", err)
	}

	if delivery.Supported {
		t.Fatalf("ParseDelivery() Supported = true, want false")
	}
	if delivery.EventType != EventTypeIssueCommentCreated {
		t.Fatalf("ParseDelivery() EventType = %q, want %q", delivery.EventType, EventTypeIssueCommentCreated)
	}
}

func TestParseDeliveryPullRequestCommentCommand(t *testing.T) {
	t.Parallel()

	delivery, err := ParseDelivery(
		"issue_comment",
		"delivery-3",
		[]byte(`{
			"action":"created",
			"repository":{"id":123456},
			"issue":{"number":42,"pull_request":{"url":"https://api.github.com/repos/acme/repo/pulls/42"}},
			"comment":{"id":99,"body":"/deploy\nplease","user":{"login":"octocat"}}
		}`),
	)
	if err != nil {
		t.Fatalf("ParseDelivery() error = %v", err)
	}

	if !delivery.Supported {
		t.Fatalf("ParseDelivery() Supported = false, want true")
	}
	if delivery.Event.CommentFirstLine != "/deploy" {
		t.Fatalf("ParseDelivery() CommentFirstLine = %q, want %q", delivery.Event.CommentFirstLine, "/deploy")
	}
	if delivery.Event.CommentAuthorLogin != "octocat" {
		t.Fatalf("ParseDelivery() CommentAuthorLogin = %q, want %q", delivery.Event.CommentAuthorLogin, "octocat")
	}
}
