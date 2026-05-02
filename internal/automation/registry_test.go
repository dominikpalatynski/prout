package automation

import (
	"encoding/json"
	"testing"

	"github.com/dominikpalatynski/toolshed/internal/operations"
	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
	"github.com/dominikpalatynski/toolshed/internal/webhook"
)

func TestRegistryExposesSupportedEventFamiliesAndTriggerMappings(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()

	eventFamilies := registry.EventFamilies()
	if len(eventFamilies) != 3 {
		t.Fatalf("len(EventFamilies()) = %d, want 3", len(eventFamilies))
	}
	if eventFamilies[0].Key != EventFamilyPullRequestOpened {
		t.Fatalf("EventFamilies()[0].Key = %q, want %q", eventFamilies[0].Key, EventFamilyPullRequestOpened)
	}
	if eventFamilies[2].TriggerTypes[0].Key != TriggerTypePreviewOnCommentPreview {
		t.Fatalf(
			"EventFamilies()[2].TriggerTypes[0].Key = %q, want %q",
			eventFamilies[2].TriggerTypes[0].Key,
			TriggerTypePreviewOnCommentPreview,
		)
	}

	triggerType, err := registry.TriggerTypeByKey(TriggerTypePreviewOnLabelPreview)
	if err != nil {
		t.Fatalf("TriggerTypeByKey() error = %v", err)
	}
	if triggerType.EventFamilyKey != EventFamilyPullRequestLabeled {
		t.Fatalf("TriggerTypeByKey() EventFamilyKey = %q, want %q", triggerType.EventFamilyKey, EventFamilyPullRequestLabeled)
	}
	if triggerType.StartsOperation != operations.TypePreviewStart {
		t.Fatalf("TriggerTypeByKey() StartsOperation = %q, want %q", triggerType.StartsOperation, operations.TypePreviewStart)
	}

	operation, err := registry.OperationByKey(operations.TypePreviewStart)
	if err != nil {
		t.Fatalf("OperationByKey() error = %v", err)
	}
	if operation.HandlerKey != HandlerPreviewStart {
		t.Fatalf("OperationByKey() HandlerKey = %q, want %q", operation.HandlerKey, HandlerPreviewStart)
	}
}

func TestEventFamilyNormalizeIssueCommentOnIssueIsNotApplicable(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	eventFamily, err := registry.EventFamilyByKey(EventFamilyPullRequestCommentCreated)
	if err != nil {
		t.Fatalf("EventFamilyByKey() error = %v", err)
	}

	delivery, err := webhook.ParseDelivery(
		"issue_comment",
		"delivery-1",
		[]byte(`{
			"action":"created",
			"repository":{"id":123456},
			"issue":{"number":42},
			"comment":{"id":99,"body":"/preview","user":{"login":"octocat"}}
		}`),
	)
	if err != nil {
		t.Fatalf("ParseDelivery() error = %v", err)
	}

	_, applicable, err := eventFamily.Normalize(delivery)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if applicable {
		t.Fatalf("Normalize() applicable = true, want false")
	}
}

func TestEvaluateRepositoryTriggerUsesRegistryOwnedMetadata(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()

	evaluation, err := registry.EvaluateRepositoryTrigger(sqlc.RepositoryTriggers{
		ID:           7,
		RepositoryID: 3,
		Type:         TriggerTypePreviewOnLabelPreview,
		Enabled:      true,
	}, EventFamilyPullRequestLabeled, webhook.NormalizedEvent{
		Type:               webhook.EventTypePullRequestLabeled,
		GithubRepositoryID: 123456,
		PRNumber:           42,
		PRHeadSHA:          "abc123",
		Label:              "preview",
	})
	if err != nil {
		t.Fatalf("EvaluateRepositoryTrigger() error = %v", err)
	}
	if !evaluation.Matched {
		t.Fatalf("EvaluateRepositoryTrigger() Matched = false, want true")
	}
	if evaluation.OperationType != operations.TypePreviewStart {
		t.Fatalf("EvaluateRepositoryTrigger() OperationType = %q, want %q", evaluation.OperationType, operations.TypePreviewStart)
	}

	var snapshot struct {
		Type           string `json:"type"`
		EventFamilyKey string `json:"event_family_key"`
	}
	if err := json.Unmarshal(evaluation.TriggerSnapshotJSON, &snapshot); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if snapshot.Type != TriggerTypePreviewOnLabelPreview {
		t.Fatalf("snapshot type = %q, want %q", snapshot.Type, TriggerTypePreviewOnLabelPreview)
	}
	if snapshot.EventFamilyKey != EventFamilyPullRequestLabeled {
		t.Fatalf("snapshot event_family_key = %q, want %q", snapshot.EventFamilyKey, EventFamilyPullRequestLabeled)
	}
}
