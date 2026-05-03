package automation

import (
	"errors"
	"strings"

	"github.com/dominikpalatynski/toolshed/internal/operations"
	"github.com/dominikpalatynski/toolshed/internal/pullrequests"
	"github.com/dominikpalatynski/toolshed/internal/webhook"
)

func registryDefinitions() Definitions {
	return Definitions{
		Operations: []OperationDefinition{
			{
				Key:                        operations.TypePreviewStart,
				Name:                       "Preview Start",
				Description:                "Creates or reuses a preview runtime for the target pull request.",
				RuntimeEnvironmentType:     operations.RuntimeEnvironmentTypePreview,
				InitialStep:                mustInitialStep(operations.TypePreviewStart),
				HandlerKey:                 HandlerPreviewStart,
				BuildTriggerIntentSnapshot: operations.BuildPreviewStartSnapshot,
			},
			{
				Key:                        operations.TypePreviewDelete,
				Name:                       "Preview Delete",
				Description:                "Tears down the frozen active preview runtime targeted by preview label removal.",
				RuntimeEnvironmentType:     operations.RuntimeEnvironmentTypePreview,
				InitialStep:                mustInitialStep(operations.TypePreviewDelete),
				HandlerKey:                 HandlerPreviewDelete,
				BuildTriggerIntentSnapshot: operations.BuildPreviewDeleteSnapshot,
			},
			{
				Key:                    operations.TypePreviewCleanupSuperseded,
				Name:                   "Preview Cleanup Superseded",
				Description:            "Tears down a superseded preview runtime and removes its workspace.",
				RuntimeEnvironmentType: operations.RuntimeEnvironmentTypePreview,
				InitialStep:            mustInitialStep(operations.TypePreviewCleanupSuperseded),
				HandlerKey:             HandlerPreviewCleanupSuperseded,
			},
		},
		EventFamilies: []EventFamilyDefinition{
			{
				Key:         EventFamilyPullRequestOpened,
				Name:        "Pull Request Opened",
				Description: "GitHub pull request opened webhooks.",
				Recognizes: []GitHubEventPattern{
					{Event: "pull_request", Action: "opened"},
				},
				Normalize: normalizePullRequestOpened,
				TriggerTypes: []TriggerTypeDefinition{
					{
						Key:             TriggerTypePreviewOnPullRequestOpened,
						Name:            "Preview on pull request opened",
						Description:     "Creates a preview when a pull request is opened.",
						Match:           matchAlways("pull_request_opened_matched"),
						StartsOperation: operations.TypePreviewStart,
					},
				},
			},
			{
				Key:         EventFamilyPullRequestLabeled,
				Name:        "Pull Request Labeled",
				Description: "GitHub pull request labeled webhooks.",
				Recognizes: []GitHubEventPattern{
					{Event: "pull_request", Action: "labeled"},
				},
				Normalize: normalizePullRequestLabeled,
				TriggerTypes: []TriggerTypeDefinition{
					{
						Key:             TriggerTypePreviewOnLabelPreview,
						Name:            "Preview on label preview",
						Description:     "Creates a preview when the preview label is applied.",
						Match:           matchLabel("preview"),
						StartsOperation: operations.TypePreviewStart,
					},
				},
			},
			{
				Key:         EventFamilyPullRequestUnlabeled,
				Name:        "Pull Request Unlabeled",
				Description: "GitHub pull request unlabeled webhooks.",
				Recognizes: []GitHubEventPattern{
					{Event: "pull_request", Action: "unlabeled"},
				},
				Normalize: normalizePullRequestUnlabeled,
				TriggerTypes: []TriggerTypeDefinition{
					{
						Key:             TriggerTypePreviewDeleteOnLabelPreviewRemoved,
						Name:            "Preview delete on label preview removed",
						Description:     "Deletes the active preview when the preview label is removed.",
						Match:           matchLabel("preview"),
						StartsOperation: operations.TypePreviewDelete,
					},
				},
			},
			{
				Key:         EventFamilyPullRequestCommentCreated,
				Name:        "Pull Request Comment Created",
				Description: "GitHub pull request comment created webhooks.",
				Recognizes: []GitHubEventPattern{
					{Event: "issue_comment", Action: "created"},
				},
				Normalize: normalizePullRequestCommentCreated,
				TriggerTypes: []TriggerTypeDefinition{
					{
						Key:             TriggerTypePreviewOnCommentPreview,
						Name:            "Preview on comment preview",
						Description:     "Creates a preview when the first comment line is /preview.",
						Match:           matchCommentFirstLine("/preview"),
						StartsOperation: operations.TypePreviewStart,
					},
				},
			},
		},
	}
}

func mustInitialStep(operationType string) operations.StepStatus {
	step, err := operations.InitialStepForOperation(operationType)
	if err != nil {
		panic(err)
	}
	return step
}

func normalizePullRequestOpened(delivery webhook.Delivery) (webhook.NormalizedEvent, bool, error) {
	return normalizePullRequestEvent(delivery, false)
}

func normalizePullRequestLabeled(delivery webhook.Delivery) (webhook.NormalizedEvent, bool, error) {
	return normalizePullRequestEvent(delivery, true)
}

func normalizePullRequestUnlabeled(delivery webhook.Delivery) (webhook.NormalizedEvent, bool, error) {
	return normalizePullRequestEvent(delivery, true)
}

func normalizePullRequestEvent(
	delivery webhook.Delivery,
	requireLabel bool,
) (webhook.NormalizedEvent, bool, error) {
	payload := delivery.Payload
	if payload.Repository.ID <= 0 {
		return webhook.NormalizedEvent{}, false, errors.New("webhook payload missing repository.id")
	}

	prNumber := payload.Number
	if prNumber == 0 {
		prNumber = payload.PullRequest.Number
	}
	if prNumber <= 0 {
		return webhook.NormalizedEvent{}, false, errors.New("webhook payload missing pull request number")
	}

	headSHA := strings.TrimSpace(payload.PullRequest.Head.SHA)
	if headSHA == "" {
		return webhook.NormalizedEvent{}, false, errors.New("webhook payload missing pull_request.head.sha")
	}

	event := webhook.NormalizedEvent{
		Type:                delivery.EventType,
		GithubRepositoryID:  payload.Repository.ID,
		PRNumber:            prNumber,
		GithubPullRequestID: payload.PullRequest.ID,
		PRHeadSHA:           headSHA,
	}

	if sourceRepository := sourceRepositoryFromPayload(payload.PullRequest.Head.Repo); sourceRepository.IsComplete() {
		event.PRSourceRepository = sourceRepository
	}

	if requireLabel {
		label := strings.TrimSpace(payload.Label.Name)
		if label == "" {
			return webhook.NormalizedEvent{}, false, errors.New("webhook payload missing label.name")
		}
		event.Label = label
	}

	return event, true, nil
}

func normalizePullRequestCommentCreated(delivery webhook.Delivery) (webhook.NormalizedEvent, bool, error) {
	payload := delivery.Payload
	if payload.Issue.PullRequest == nil {
		return webhook.NormalizedEvent{}, false, nil
	}
	if payload.Repository.ID <= 0 {
		return webhook.NormalizedEvent{}, false, errors.New("webhook payload missing repository.id")
	}
	if payload.Issue.Number <= 0 {
		return webhook.NormalizedEvent{}, false, errors.New("webhook payload missing issue.number")
	}

	body := payload.Comment.Body
	return webhook.NormalizedEvent{
		Type:               delivery.EventType,
		GithubRepositoryID: payload.Repository.ID,
		PRNumber:           payload.Issue.Number,
		CommentID:          payload.Comment.ID,
		CommentBody:        body,
		CommentFirstLine:   firstLine(body),
		CommentAuthorLogin: strings.TrimSpace(payload.Comment.User.Login),
	}, true, nil
}

func sourceRepositoryFromPayload(repo webhook.GitHubRepositoryPayload) pullrequests.SourceRepository {
	fullName := strings.TrimSpace(repo.FullName)
	owner := strings.TrimSpace(repo.Owner.Login)
	name := strings.TrimSpace(repo.Name)
	if fullName == "" && owner != "" && name != "" {
		fullName = owner + "/" + name
	}
	return pullrequests.SourceRepository{
		GithubRepositoryID: repo.ID,
		Owner:              owner,
		Name:               name,
		FullName:           fullName,
	}
}

func matchAlways(reason string) func(webhook.NormalizedEvent) MatchResult {
	return func(webhook.NormalizedEvent) MatchResult {
		return MatchResult{Matched: true, Reason: reason}
	}
}

func matchLabel(expectedLabel string) func(webhook.NormalizedEvent) MatchResult {
	return func(event webhook.NormalizedEvent) MatchResult {
		if strings.TrimSpace(event.Label) == expectedLabel {
			return MatchResult{Matched: true, Reason: "label_matched"}
		}
		return MatchResult{Matched: false, Reason: "label_mismatch"}
	}
}

func matchCommentFirstLine(expected string) func(webhook.NormalizedEvent) MatchResult {
	return func(event webhook.NormalizedEvent) MatchResult {
		if strings.TrimSpace(event.CommentFirstLine) == expected {
			return MatchResult{Matched: true, Reason: "comment_command_matched"}
		}
		return MatchResult{Matched: false, Reason: "comment_command_mismatch"}
	}
}

func firstLine(body string) string {
	line, _, _ := strings.Cut(body, "\n")
	return strings.TrimSuffix(line, "\r")
}
