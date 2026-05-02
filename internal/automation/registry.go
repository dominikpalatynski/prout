package automation

import (
	"encoding/json"
	"fmt"

	"github.com/dominikpalatynski/toolshed/internal/operations"
	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
	"github.com/dominikpalatynski/toolshed/internal/webhook"
)

const (
	EventFamilyPullRequestOpened         = "pull-request-opened"
	EventFamilyPullRequestLabeled        = "pull-request-labeled"
	EventFamilyPullRequestCommentCreated = "pull-request-comment-created"

	TriggerTypePreviewOnPullRequestOpened = "preview_on_pull_request_opened"
	TriggerTypePreviewOnLabelPreview      = "preview_on_label_preview"
	TriggerTypePreviewOnCommentPreview    = "preview_on_comment_preview"

	HandlerPreviewStart             = "handle_preview_start"
	HandlerPreviewCleanupSuperseded = "handle_preview_cleanup_superseded"
)

type GitHubEventPattern struct {
	Event  string `json:"event"`
	Action string `json:"action"`
}

type MatchResult struct {
	Matched bool
	Reason  string
}

type TriggerEvaluation struct {
	Matched             bool
	Reason              string
	TriggerSnapshotJSON []byte
	OperationType       string
}

type OperationDefinition struct {
	Key                        string                                                     `json:"key"`
	Name                       string                                                     `json:"name"`
	Description                string                                                     `json:"description"`
	RuntimeEnvironmentType     string                                                     `json:"runtime_environment_type"`
	InitialStep                operations.StepStatus                                      `json:"initial_step"`
	HandlerKey                 string                                                     `json:"handler_key"`
	BuildTriggerIntentSnapshot func(operations.PreviewStartSnapshotInput) ([]byte, error) `json:"-"`
}

type TriggerTypeDefinition struct {
	Key             string                                    `json:"type"`
	Name            string                                    `json:"name"`
	Description     string                                    `json:"description"`
	EventFamilyKey  string                                    `json:"event_family_key"`
	StartsOperation string                                    `json:"operation_type"`
	Match           func(webhook.NormalizedEvent) MatchResult `json:"-"`
}

type EventFamilyDefinition struct {
	Key            string                                                        `json:"key"`
	Name           string                                                        `json:"name"`
	Description    string                                                        `json:"description"`
	Recognizes     []GitHubEventPattern                                          `json:"github_event_patterns"`
	Normalize      func(webhook.Delivery) (webhook.NormalizedEvent, bool, error) `json:"-"`
	DisabledReason string                                                        `json:"-"`
	TriggerTypes   []TriggerTypeDefinition                                       `json:"trigger_types"`
}

type Definitions struct {
	Operations    []OperationDefinition
	EventFamilies []EventFamilyDefinition
}

type Registry struct {
	operations       map[string]OperationDefinition
	eventFamilies    map[string]EventFamilyDefinition
	triggerTypes     map[string]TriggerTypeDefinition
	operationOrder   []string
	eventFamilyOrder []string
	triggerTypeOrder []string
}

func NewRegistry() *Registry {
	registry, err := newRegistry(registryDefinitions())
	if err != nil {
		panic(err)
	}
	return registry
}

func newRegistry(definitions Definitions) (*Registry, error) {
	registry := &Registry{
		operations:    make(map[string]OperationDefinition, len(definitions.Operations)),
		eventFamilies: make(map[string]EventFamilyDefinition, len(definitions.EventFamilies)),
		triggerTypes:  make(map[string]TriggerTypeDefinition),
	}

	for _, operation := range definitions.Operations {
		if operation.Key == "" {
			return nil, fmt.Errorf("operation definition key is required")
		}
		if _, exists := registry.operations[operation.Key]; exists {
			return nil, fmt.Errorf("duplicate operation definition %q", operation.Key)
		}
		registry.operations[operation.Key] = operation
		registry.operationOrder = append(registry.operationOrder, operation.Key)
	}

	for _, eventFamily := range definitions.EventFamilies {
		if eventFamily.Key == "" {
			return nil, fmt.Errorf("event family definition key is required")
		}
		if _, exists := registry.eventFamilies[eventFamily.Key]; exists {
			return nil, fmt.Errorf("duplicate event family definition %q", eventFamily.Key)
		}

		resolvedEventFamily := eventFamily
		resolvedEventFamily.Recognizes = clonePatterns(eventFamily.Recognizes)
		resolvedEventFamily.TriggerTypes = make([]TriggerTypeDefinition, 0, len(eventFamily.TriggerTypes))
		if resolvedEventFamily.DisabledReason == "" {
			resolvedEventFamily.DisabledReason = repositoryEventFamilyDisabledReason(eventFamily.Key)
		}

		for _, triggerType := range eventFamily.TriggerTypes {
			if triggerType.Key == "" {
				return nil, fmt.Errorf("trigger type key is required for event family %q", eventFamily.Key)
			}
			if _, exists := registry.triggerTypes[triggerType.Key]; exists {
				return nil, fmt.Errorf("duplicate trigger type definition %q", triggerType.Key)
			}
			if triggerType.StartsOperation == "" {
				return nil, fmt.Errorf("trigger type %q is missing operation binding", triggerType.Key)
			}
			if _, exists := registry.operations[triggerType.StartsOperation]; !exists {
				return nil, fmt.Errorf(
					"trigger type %q references unknown operation %q",
					triggerType.Key,
					triggerType.StartsOperation,
				)
			}

			resolvedTriggerType := triggerType
			resolvedTriggerType.EventFamilyKey = eventFamily.Key
			registry.triggerTypes[resolvedTriggerType.Key] = resolvedTriggerType
			registry.triggerTypeOrder = append(registry.triggerTypeOrder, resolvedTriggerType.Key)
			resolvedEventFamily.TriggerTypes = append(resolvedEventFamily.TriggerTypes, resolvedTriggerType)
		}

		registry.eventFamilies[resolvedEventFamily.Key] = resolvedEventFamily
		registry.eventFamilyOrder = append(registry.eventFamilyOrder, resolvedEventFamily.Key)
	}

	return registry, nil
}

func (r *Registry) EventFamilies() []EventFamilyDefinition {
	eventFamilies := make([]EventFamilyDefinition, 0, len(r.eventFamilyOrder))
	for _, key := range r.eventFamilyOrder {
		definition := r.eventFamilies[key]
		definition.Recognizes = clonePatterns(definition.Recognizes)
		definition.TriggerTypes = cloneTriggerTypes(definition.TriggerTypes)
		eventFamilies = append(eventFamilies, definition)
	}
	return eventFamilies
}

func (r *Registry) TriggerTypes() []TriggerTypeDefinition {
	triggerTypes := make([]TriggerTypeDefinition, 0, len(r.triggerTypeOrder))
	for _, key := range r.triggerTypeOrder {
		triggerTypes = append(triggerTypes, r.triggerTypes[key])
	}
	return triggerTypes
}

func (r *Registry) SupportedEventFamilyKeys() []string {
	keys := make([]string, 0, len(r.eventFamilyOrder))
	keys = append(keys, r.eventFamilyOrder...)
	return keys
}

func (r *Registry) ResolveEventFamily(delivery webhook.Delivery) (EventFamilyDefinition, bool) {
	for _, key := range r.eventFamilyOrder {
		definition := r.eventFamilies[key]
		for _, pattern := range definition.Recognizes {
			if pattern.Event != delivery.GithubEvent {
				continue
			}
			if pattern.Action != delivery.GithubAction {
				continue
			}
			return definition, true
		}
	}
	return EventFamilyDefinition{}, false
}

func (r *Registry) EventFamilyByKey(key string) (EventFamilyDefinition, error) {
	definition, ok := r.eventFamilies[key]
	if !ok {
		return EventFamilyDefinition{}, fmt.Errorf("unsupported event family %q", key)
	}
	definition.Recognizes = clonePatterns(definition.Recognizes)
	definition.TriggerTypes = cloneTriggerTypes(definition.TriggerTypes)
	return definition, nil
}

func (r *Registry) TriggerTypeByKey(key string) (TriggerTypeDefinition, error) {
	definition, ok := r.triggerTypes[key]
	if !ok {
		return TriggerTypeDefinition{}, fmt.Errorf("unsupported trigger type %q", key)
	}
	return definition, nil
}

func (r *Registry) OperationByKey(key string) (OperationDefinition, error) {
	definition, ok := r.operations[key]
	if !ok {
		return OperationDefinition{}, fmt.Errorf("unsupported operation type %q", key)
	}
	return definition, nil
}

func (r *Registry) EvaluateRepositoryTrigger(
	trigger sqlc.RepositoryTriggers,
	eventFamilyKey string,
	event webhook.NormalizedEvent,
) (TriggerEvaluation, error) {
	triggerType, err := r.TriggerTypeByKey(trigger.Type)
	if err != nil {
		return TriggerEvaluation{}, err
	}

	triggerSnapshotJSON, err := buildRepositoryTriggerSnapshot(trigger, triggerType)
	if err != nil {
		return TriggerEvaluation{}, err
	}

	if triggerType.EventFamilyKey != eventFamilyKey {
		return TriggerEvaluation{
			Matched:             false,
			Reason:              "event_family_mismatch",
			TriggerSnapshotJSON: triggerSnapshotJSON,
		}, nil
	}

	matchResult := triggerType.Match(event)
	evaluation := TriggerEvaluation{
		Matched:             matchResult.Matched,
		Reason:              matchResult.Reason,
		TriggerSnapshotJSON: triggerSnapshotJSON,
	}
	if matchResult.Matched {
		evaluation.OperationType = triggerType.StartsOperation
	}
	return evaluation, nil
}

func buildRepositoryTriggerSnapshot(
	trigger sqlc.RepositoryTriggers,
	triggerType TriggerTypeDefinition,
) ([]byte, error) {
	return json.Marshal(struct {
		ID             int64  `json:"id"`
		RepositoryID   int64  `json:"repository_id"`
		Type           string `json:"type"`
		Name           string `json:"name"`
		EventFamilyKey string `json:"event_family_key"`
		Enabled        bool   `json:"enabled"`
	}{
		ID:             trigger.ID,
		RepositoryID:   trigger.RepositoryID,
		Type:           trigger.Type,
		Name:           triggerType.Name,
		EventFamilyKey: triggerType.EventFamilyKey,
		Enabled:        trigger.Enabled,
	})
}

func repositoryEventFamilyDisabledReason(eventFamilyKey string) string {
	return "repository_event_family_disabled:" + eventFamilyKey
}

func clonePatterns(patterns []GitHubEventPattern) []GitHubEventPattern {
	if len(patterns) == 0 {
		return nil
	}
	cloned := make([]GitHubEventPattern, len(patterns))
	copy(cloned, patterns)
	return cloned
}

func cloneTriggerTypes(triggerTypes []TriggerTypeDefinition) []TriggerTypeDefinition {
	if len(triggerTypes) == 0 {
		return nil
	}
	cloned := make([]TriggerTypeDefinition, len(triggerTypes))
	copy(cloned, triggerTypes)
	return cloned
}
