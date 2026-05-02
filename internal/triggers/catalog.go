package triggers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dominikpalatynski/toolshed/internal/operations"
	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
	"github.com/dominikpalatynski/toolshed/internal/webhook"
)

const (
	TypePullRequestOpened         = "pull_request_opened"
	TypePullRequestLabel          = "pull_request_label"
	TypePullRequestCommentCommand = "pull_request_comment_command"

	CommentMatcherExactFirstLine = "exact_first_line"
)

type Catalog struct {
	definitions map[string]definition
	order       []string
}

type TypeDefinition struct {
	Type         string          `json:"type"`
	EventFamily  string          `json:"event_family"`
	Description  string          `json:"description"`
	Config       json.RawMessage `json:"config,omitempty"`
	ConfigFields []ConfigField   `json:"config_fields,omitempty"`
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
	OperationIntent     *operations.Intent
}

type definition struct {
	TypeDefinition
	IdentityKey     string
	normalizeConfig func(json.RawMessage) ([]byte, error)
	matchEvent      func(webhook.NormalizedEvent) (bool, string)
}

func NewCatalog() *Catalog {
	return &Catalog{
		definitions: map[string]definition{
			TypePullRequestOpened: {
				TypeDefinition: TypeDefinition{
					Type:        TypePullRequestOpened,
					EventFamily: webhook.EventTypePullRequestOpened,
					Description: "Creates a preview when a pull request is opened.",
					Config:      mustMarshalRawMessage(struct{}{}),
				},
				IdentityKey:     TypePullRequestOpened,
				normalizeConfig: normalizeEmptyConfig,
				matchEvent: func(_ webhook.NormalizedEvent) (bool, string) {
					return true, "pull_request_opened_matched"
				},
			},
			TypePullRequestLabel: {
				TypeDefinition: TypeDefinition{
					Type:        TypePullRequestLabel,
					EventFamily: webhook.EventTypePullRequestLabeled,
					Description: "Creates a preview when the fixed preview label preset is applied.",
					Config: mustMarshalRawMessage(pullRequestLabelConfig{
						Label: "preview",
					}),
				},
				IdentityKey:     TypePullRequestLabel + ":preview",
				normalizeConfig: normalizePullRequestLabelConfig,
				matchEvent: func(event webhook.NormalizedEvent) (bool, string) {
					if strings.TrimSpace(event.Label) == "preview" {
						return true, "label_matched"
					}
					return false, "label_mismatch"
				},
			},
			TypePullRequestCommentCommand: {
				TypeDefinition: TypeDefinition{
					Type:        TypePullRequestCommentCommand,
					EventFamily: webhook.EventTypeIssueCommentCreated,
					Description: "Creates a preview when the fixed preview comment command preset is posted on the pull request.",
					Config: mustMarshalRawMessage(pullRequestCommentCommandConfig{
						Matcher: CommentMatcherExactFirstLine,
						Command: "/preview",
					}),
				},
				IdentityKey:     TypePullRequestCommentCommand + ":" + CommentMatcherExactFirstLine + ":/preview",
				normalizeConfig: normalizePullRequestCommentCommandConfig,
				matchEvent: func(event webhook.NormalizedEvent) (bool, string) {
					if event.CommentFirstLine == "/preview" {
						return true, "comment_command_matched"
					}
					return false, "comment_command_mismatch"
				},
			},
		},
		order: []string{
			TypePullRequestOpened,
			TypePullRequestLabel,
			TypePullRequestCommentCommand,
		},
	}
}

func (c *Catalog) Definitions() []TypeDefinition {
	definitions := make([]TypeDefinition, 0, len(c.order))
	for _, triggerType := range c.order {
		definition := c.definitions[triggerType].TypeDefinition
		definition.Config = append(json.RawMessage(nil), definition.Config...)
		definitions = append(definitions, definition)
	}
	return definitions
}

func (c *Catalog) ValidateAndNormalize(triggerType string, rawConfig json.RawMessage) (ValidatedTrigger, error) {
	triggerType = strings.TrimSpace(triggerType)
	definition, ok := c.definitions[triggerType]
	if !ok {
		return ValidatedTrigger{}, fmt.Errorf("unsupported trigger type %q", triggerType)
	}

	configJSON, err := validateAndNormalizeConfig(definition, rawConfig)
	if err != nil {
		return ValidatedTrigger{}, err
	}

	return ValidatedTrigger{
		Type:        definition.Type,
		EventFamily: definition.EventFamily,
		IdentityKey: definition.IdentityKey,
		ConfigJSON:  configJSON,
	}, nil
}

func (c *Catalog) Evaluate(trigger sqlc.RepositoryTriggers, _ string, event webhook.NormalizedEvent) (Evaluation, error) {
	triggerSnapshotJSON, err := triggerSnapshotJSON(trigger)
	if err != nil {
		return Evaluation{}, err
	}

	definition, ok := c.definitions[trigger.Type]
	if !ok {
		return Evaluation{}, fmt.Errorf("unsupported trigger type %q", trigger.Type)
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

	configMatches, reason, err := matchesDefinitionConfig(definition, trigger.ConfigJson)
	if err != nil {
		return Evaluation{}, err
	}
	if !configMatches {
		evaluation.Reason = reason
		return evaluation, nil
	}

	evaluation.Matched, evaluation.Reason = definition.matchEvent(event)

	if !evaluation.Matched {
		return evaluation, nil
	}

	operationIntent, err := operations.NewIntent(trigger.Type)
	if err != nil {
		return Evaluation{}, err
	}
	evaluation.OperationIntent = &operationIntent
	return evaluation, nil
}

type pullRequestLabelConfig struct {
	Label string `json:"label"`
}

type pullRequestCommentCommandConfig struct {
	Matcher string `json:"matcher"`
	Command string `json:"command"`
}

func mustMarshalRawMessage(value any) json.RawMessage {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return payload
}

func validateAndNormalizeConfig(definition definition, rawConfig json.RawMessage) ([]byte, error) {
	if isOmittedConfig(rawConfig) {
		return append([]byte(nil), definition.Config...), nil
	}

	normalizedConfig, err := definition.normalizeConfig(rawConfig)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(normalizedConfig, definition.Config) {
		return nil, fmt.Errorf("trigger type %q expects config %s", definition.Type, definition.Config)
	}

	return append([]byte(nil), definition.Config...), nil
}

func matchesDefinitionConfig(definition definition, rawConfig json.RawMessage) (bool, string, error) {
	normalizedConfig, err := definition.normalizeConfig(rawConfig)
	if err != nil {
		return false, "trigger_config_invalid", nil
	}
	if !bytes.Equal(normalizedConfig, definition.Config) {
		return false, "trigger_config_mismatch", nil
	}
	return true, "", nil
}

func normalizeEmptyConfig(rawConfig json.RawMessage) ([]byte, error) {
	var config struct{}
	if err := decodeConfig(rawConfig, &config); err != nil {
		return nil, err
	}
	return json.Marshal(config)
}

func normalizePullRequestLabelConfig(rawConfig json.RawMessage) ([]byte, error) {
	var config pullRequestLabelConfig
	if err := decodeConfig(rawConfig, &config); err != nil {
		return nil, err
	}
	config.Label = strings.TrimSpace(config.Label)
	if config.Label == "" {
		return nil, fmt.Errorf("trigger config field label is required")
	}
	return json.Marshal(config)
}

func normalizePullRequestCommentCommandConfig(rawConfig json.RawMessage) ([]byte, error) {
	var config pullRequestCommentCommandConfig
	if err := decodeConfig(rawConfig, &config); err != nil {
		return nil, err
	}
	config.Matcher = strings.TrimSpace(config.Matcher)
	config.Command = strings.TrimSpace(config.Command)
	if config.Matcher != CommentMatcherExactFirstLine {
		return nil, fmt.Errorf("trigger config field matcher must be %q", CommentMatcherExactFirstLine)
	}
	if config.Command == "" {
		return nil, fmt.Errorf("trigger config field command is required")
	}
	return json.Marshal(config)
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

func isOmittedConfig(rawConfig json.RawMessage) bool {
	trimmed := bytes.TrimSpace(rawConfig)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("{}")) || bytes.Equal(trimmed, []byte("null"))
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
