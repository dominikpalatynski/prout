package webhook

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/dominikpalatynski/toolshed/internal/jobs"
)

type Decision int

const (
	DecisionIgnore Decision = iota
	DecisionEnqueue
)

type githubPayload struct {
	Action     string `json:"action"`
	Number     int    `json:"number"`
	Repository struct {
		ID int64 `json:"id"`
	} `json:"repository"`
	PullRequest struct {
		Number int `json:"number"`
		Head   struct {
			SHA string `json:"sha"`
		} `json:"head"`
	} `json:"pull_request"`
}

func NormalizeGitHubDelivery(eventHeader, deliveryHeader string, body io.Reader) (jobs.RecordPingArgs, Decision, error) {
	eventHeader = strings.TrimSpace(eventHeader)
	deliveryHeader = strings.TrimSpace(deliveryHeader)

	if eventHeader == "" {
		return jobs.RecordPingArgs{}, DecisionIgnore, fmt.Errorf("missing X-GitHub-Event header")
	}
	if deliveryHeader == "" {
		return jobs.RecordPingArgs{}, DecisionIgnore, fmt.Errorf("missing X-GitHub-Delivery header")
	}

	var payload githubPayload
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&payload); err != nil {
		return jobs.RecordPingArgs{}, DecisionIgnore, fmt.Errorf("decode webhook payload: %w", err)
	}

	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		return jobs.RecordPingArgs{}, DecisionIgnore, fmt.Errorf("webhook payload must contain exactly one JSON object")
	}

	if eventHeader != "pull_request" {
		return jobs.RecordPingArgs{}, DecisionIgnore, nil
	}
	if payload.Action == "" {
		return jobs.RecordPingArgs{}, DecisionIgnore, fmt.Errorf("pull_request payload missing action")
	}
	if payload.Action != "opened" {
		return jobs.RecordPingArgs{}, DecisionIgnore, nil
	}

	prNumber := payload.Number
	if prNumber == 0 {
		prNumber = payload.PullRequest.Number
	}

	normalized := jobs.RecordPingArgs{
		DeliveryID:   deliveryHeader,
		Event:        eventHeader,
		RepositoryID: payload.Repository.ID,
		PRNumber:     prNumber,
		SHA:          strings.TrimSpace(payload.PullRequest.Head.SHA),
	}

	switch {
	case normalized.RepositoryID <= 0:
		return jobs.RecordPingArgs{}, DecisionIgnore, fmt.Errorf("pull_request payload missing repository.id")
	case normalized.PRNumber <= 0:
		return jobs.RecordPingArgs{}, DecisionIgnore, fmt.Errorf("pull_request payload missing number")
	case normalized.SHA == "":
		return jobs.RecordPingArgs{}, DecisionIgnore, fmt.Errorf("pull_request payload missing pull_request.head.sha")
	default:
		return normalized, DecisionEnqueue, nil
	}
}
