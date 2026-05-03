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
			"pull_request":{"id":987654,"head":{"sha":"abc123","repo":{"id":654321,"name":"demo","full_name":"acme/demo","owner":{"login":"acme"}}}}
		}`),
	)
	if err != nil {
		t.Fatalf("ParseDelivery() error = %v", err)
	}

	if delivery.EventType != EventTypePullRequestOpened {
		t.Fatalf("ParseDelivery() EventType = %q, want %q", delivery.EventType, EventTypePullRequestOpened)
	}
	if delivery.GithubAction != "opened" {
		t.Fatalf("ParseDelivery() GithubAction = %q, want %q", delivery.GithubAction, "opened")
	}
	if delivery.Payload.Number != 42 {
		t.Fatalf("ParseDelivery() payload number = %d, want 42", delivery.Payload.Number)
	}
	if delivery.Payload.PullRequest.ID != 987654 {
		t.Fatalf("ParseDelivery() pull_request.id = %d, want 987654", delivery.Payload.PullRequest.ID)
	}
	if delivery.Payload.PullRequest.Head.SHA != "abc123" {
		t.Fatalf("ParseDelivery() pull_request.head.sha = %q, want %q", delivery.Payload.PullRequest.Head.SHA, "abc123")
	}
}

func TestParseDeliveryPullRequestUnlabeled(t *testing.T) {
	t.Parallel()

	delivery, err := ParseDelivery(
		"pull_request",
		"delivery-unlabeled",
		[]byte(`{
			"action":"unlabeled",
			"number":42,
			"repository":{"id":123456},
			"label":{"name":"preview"},
			"pull_request":{"id":987654,"head":{"sha":"abc123","repo":{"id":654321,"name":"demo","full_name":"acme/demo","owner":{"login":"acme"}}}}
		}`),
	)
	if err != nil {
		t.Fatalf("ParseDelivery() error = %v", err)
	}

	if delivery.EventType != EventTypePullRequestUnlabeled {
		t.Fatalf("ParseDelivery() EventType = %q, want %q", delivery.EventType, EventTypePullRequestUnlabeled)
	}
	if delivery.GithubAction != "unlabeled" {
		t.Fatalf("ParseDelivery() GithubAction = %q, want %q", delivery.GithubAction, "unlabeled")
	}
	if delivery.Payload.Label.Name != "preview" {
		t.Fatalf("ParseDelivery() label.name = %q, want %q", delivery.Payload.Label.Name, "preview")
	}
}

func TestParseDeliveryIssueCommentOnIssuePreservesPayload(t *testing.T) {
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

	if delivery.EventType != EventTypeIssueCommentCreated {
		t.Fatalf("ParseDelivery() EventType = %q, want %q", delivery.EventType, EventTypeIssueCommentCreated)
	}
	if delivery.Payload.Issue.PullRequest != nil {
		t.Fatalf("ParseDelivery() issue.pull_request = %#v, want nil", delivery.Payload.Issue.PullRequest)
	}
}

func TestParseDeliveryPullRequestCommentPayload(t *testing.T) {
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

	if delivery.Payload.Comment.Body != "/deploy\nplease" {
		t.Fatalf("ParseDelivery() comment.body = %q, want %q", delivery.Payload.Comment.Body, "/deploy\nplease")
	}
	if delivery.Payload.Comment.User.Login != "octocat" {
		t.Fatalf("ParseDelivery() comment.user.login = %q, want %q", delivery.Payload.Comment.User.Login, "octocat")
	}
}
