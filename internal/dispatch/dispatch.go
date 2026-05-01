package dispatch

import (
	"encoding/json"
	"fmt"

	"github.com/dominikpalatynski/toolshed/internal/webhook"
)

const (
	DispatchTypePullRequestOpened         = "trigger_dispatch_pull_request_opened"
	DispatchTypePullRequestLabel          = "trigger_dispatch_pull_request_label"
	DispatchTypePullRequestCommentCommand = "trigger_dispatch_pull_request_comment_command"
)

type Intent struct {
	DispatchType string
	PayloadJSON  []byte
}

func NewIntent(triggerType string, triggerID int64, identityKey, deliveryID string, event webhook.NormalizedEvent) (Intent, error) {
	dispatchType, err := dispatchTypeForTrigger(triggerType)
	if err != nil {
		return Intent{}, err
	}

	payload, err := json.Marshal(struct {
		DeliveryID string                  `json:"delivery_id"`
		Event      webhook.NormalizedEvent `json:"event"`
		Trigger    struct {
			ID          int64  `json:"id"`
			Type        string `json:"type"`
			IdentityKey string `json:"identity_key"`
		} `json:"trigger"`
	}{
		DeliveryID: deliveryID,
		Event:      event,
		Trigger: struct {
			ID          int64  `json:"id"`
			Type        string `json:"type"`
			IdentityKey string `json:"identity_key"`
		}{
			ID:          triggerID,
			Type:        triggerType,
			IdentityKey: identityKey,
		},
	})
	if err != nil {
		return Intent{}, fmt.Errorf("marshal dispatch payload: %w", err)
	}

	return Intent{
		DispatchType: dispatchType,
		PayloadJSON:  payload,
	}, nil
}

func dispatchTypeForTrigger(triggerType string) (string, error) {
	switch triggerType {
	case "pull_request_opened":
		return DispatchTypePullRequestOpened, nil
	case "pull_request_label":
		return DispatchTypePullRequestLabel, nil
	case "pull_request_comment_command":
		return DispatchTypePullRequestCommentCommand, nil
	default:
		return "", fmt.Errorf("unknown trigger type %q", triggerType)
	}
}
