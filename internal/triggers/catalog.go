package triggers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dominikpalatynski/toolshed/internal/dispatch"
	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
	"github.com/dominikpalatynski/toolshed/internal/webhook"
)

const (
	TypePullRequestOpened         = "pull_request_opened"
	TypePullRequestLabel          = "pull_request_label"
	TypePullRequestCommentCommand = "pull_request_comment_command"

	CommentMatcherExactFirstLine = "exact_first_line"
)

type Catalog struct{}

type TypeDefinition struct {
	Type         string        `json:"type"`
	EventFamily  string        `json:"event_family"`
	Description  string        `json:"description"`
	ConfigFields []ConfigField `json:"config_fields"`
}

type ConfigField struct {
	Name          string   `json:"name"`
	Type          string   `json:"type"`
	Required      bool     `json:"required"`
	Description   string   `json:"description"`
	AllowedValues []string `json:"allowed_values,omitempty"`
}

type ValidatedTrigger struct {
	Type        string
	EventFamily string
	IdentityKey string
	ConfigJSON  []byte
}

type Evaluation struct {
	Matched             bool
	Reason              string
	TriggerSnapshotJSON []byte
	DispatchIntent      *dispatch.Intent
}

func NewCatalog() *Catalog {
	return &Catalog{}
}

func (c *Catalog) Definitions() []TypeDefinition {
	return []TypeDefinition{
		{
			Type:        TypePullRequestOpened,
			EventFamily: webhook.EventTypePullRequestOpened,
			Description: "Matches pull_request.opened deliveries for the repository.",
		},
		{
			Type:        TypePullRequestLabel,
			EventFamily: webhook.EventTypePullRequestLabeled,
			Description: "Matches pull_request.labeled deliveries for one exact label.",
			ConfigFields: []ConfigField{
				{
					Name:        "label",
					Type:        "string",
					Required:    true,
					Description: "Exact GitHub label name to match.",
				},
			},
		},
		{
			Type:        TypePullRequestCommentCommand,
			EventFamily: webhook.EventTypeIssueCommentCreated,
			Description: "Matches a pull request conversation comment command using the configured comment matcher.",
			ConfigFields: []ConfigField{
				{
					Name:          "matcher",
					Type:          "string",
					Required:      true,
					Description:   "Comment matching strategy.",
					AllowedValues: []string{CommentMatcherExactFirstLine},
				},
				{
					Name:        "command",
					Type:        "string",
					Required:    true,
					Description: "Case-sensitive command text matched against the first comment line.",
				},
			},
		},
	}
}

func (c *Catalog) ValidateAndNormalize(triggerType string, rawConfig json.RawMessage) (ValidatedTrigger, error) {
	switch strings.TrimSpace(triggerType) {
	case TypePullRequestOpened:
		var config struct{}
		if err := decodeConfig(rawConfig, &config); err != nil {
			return ValidatedTrigger{}, err
		}
		configJSON, err := json.Marshal(config)
		if err != nil {
			return ValidatedTrigger{}, err
		}
		return ValidatedTrigger{
			Type:        TypePullRequestOpened,
			EventFamily: webhook.EventTypePullRequestOpened,
			IdentityKey: TypePullRequestOpened,
			ConfigJSON:  configJSON,
		}, nil
	case TypePullRequestLabel:
		var config pullRequestLabelConfig
		if err := decodeConfig(rawConfig, &config); err != nil {
			return ValidatedTrigger{}, err
		}
		config.Label = strings.TrimSpace(config.Label)
		if config.Label == "" {
			return ValidatedTrigger{}, fmt.Errorf("trigger config field label is required")
		}
		configJSON, err := json.Marshal(config)
		if err != nil {
			return ValidatedTrigger{}, err
		}
		return ValidatedTrigger{
			Type:        TypePullRequestLabel,
			EventFamily: webhook.EventTypePullRequestLabeled,
			IdentityKey: TypePullRequestLabel + ":" + config.Label,
			ConfigJSON:  configJSON,
		}, nil
	case TypePullRequestCommentCommand:
		var config pullRequestCommentCommandConfig
		if err := decodeConfig(rawConfig, &config); err != nil {
			return ValidatedTrigger{}, err
		}
		config.Matcher = strings.TrimSpace(config.Matcher)
		config.Command = strings.TrimSpace(config.Command)
		if config.Matcher != CommentMatcherExactFirstLine {
			return ValidatedTrigger{}, fmt.Errorf("trigger config field matcher must be %q", CommentMatcherExactFirstLine)
		}
		if config.Command == "" {
			return ValidatedTrigger{}, fmt.Errorf("trigger config field command is required")
		}
		configJSON, err := json.Marshal(config)
		if err != nil {
			return ValidatedTrigger{}, err
		}
		return ValidatedTrigger{
			Type:        TypePullRequestCommentCommand,
			EventFamily: webhook.EventTypeIssueCommentCreated,
			IdentityKey: TypePullRequestCommentCommand + ":" + config.Matcher + ":" + config.Command,
			ConfigJSON:  configJSON,
		}, nil
	default:
		return ValidatedTrigger{}, fmt.Errorf("unsupported trigger type %q", triggerType)
	}
}

func (c *Catalog) Evaluate(trigger sqlc.RepositoryTriggers, deliveryID string, event webhook.NormalizedEvent) (Evaluation, error) {
	triggerSnapshotJSON, err := triggerSnapshotJSON(trigger)
	if err != nil {
		return Evaluation{}, err
	}

	if trigger.EventFamily != event.Type {
		return Evaluation{
			Matched:             false,
			Reason:              "event_family_mismatch",
			TriggerSnapshotJSON: triggerSnapshotJSON,
		}, nil
	}

	evaluation := Evaluation{
		TriggerSnapshotJSON: triggerSnapshotJSON,
	}

	switch trigger.Type {
	case TypePullRequestOpened:
		evaluation.Matched = true
		evaluation.Reason = "pull_request_opened_matched"
	case TypePullRequestLabel:
		var config pullRequestLabelConfig
		if err := decodeConfig(trigger.ConfigJson, &config); err != nil {
			return Evaluation{}, fmt.Errorf("decode pull_request_label config: %w", err)
		}
		if config.Label == event.Label {
			evaluation.Matched = true
			evaluation.Reason = "label_matched"
		} else {
			evaluation.Reason = "label_mismatch"
		}
	case TypePullRequestCommentCommand:
		var config pullRequestCommentCommandConfig
		if err := decodeConfig(trigger.ConfigJson, &config); err != nil {
			return Evaluation{}, fmt.Errorf("decode pull_request_comment_command config: %w", err)
		}
		if config.Command == event.CommentFirstLine {
			evaluation.Matched = true
			evaluation.Reason = "comment_command_matched"
		} else {
			evaluation.Reason = "comment_command_mismatch"
		}
	default:
		return Evaluation{}, fmt.Errorf("unsupported trigger type %q", trigger.Type)
	}

	if !evaluation.Matched {
		return evaluation, nil
	}

	dispatchIntent, err := dispatch.NewIntent(trigger.Type, trigger.ID, trigger.IdentityKey, deliveryID, event)
	if err != nil {
		return Evaluation{}, err
	}
	evaluation.DispatchIntent = &dispatchIntent
	return evaluation, nil
}

type pullRequestLabelConfig struct {
	Label string `json:"label"`
}

type pullRequestCommentCommandConfig struct {
	Matcher string `json:"matcher"`
	Command string `json:"command"`
}

func decodeConfig(rawConfig []byte, dst any) error {
	payload := rawConfig
	if len(bytes.TrimSpace(payload)) == 0 {
		payload = []byte("{}")
	}

	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode trigger config: %w", err)
	}
	return nil
}

func triggerSnapshotJSON(trigger sqlc.RepositoryTriggers) ([]byte, error) {
	return json.Marshal(struct {
		ID           int64           `json:"id"`
		RepositoryID int64           `json:"repository_id"`
		Type         string          `json:"type"`
		EventFamily  string          `json:"event_family"`
		IdentityKey  string          `json:"identity_key"`
		Enabled      bool            `json:"enabled"`
		Config       json.RawMessage `json:"config"`
	}{
		ID:           trigger.ID,
		RepositoryID: trigger.RepositoryID,
		Type:         trigger.Type,
		EventFamily:  trigger.EventFamily,
		IdentityKey:  trigger.IdentityKey,
		Enabled:      trigger.Enabled,
		Config:       append(json.RawMessage(nil), trigger.ConfigJson...),
	})
}
