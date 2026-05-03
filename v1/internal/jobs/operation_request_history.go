package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/dominikpalatynski/toolshed/internal/operations"
	"github.com/dominikpalatynski/toolshed/internal/store"
	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
)

var environmentAssignmentPattern = regexp.MustCompile(`\b([A-Z][A-Z0-9_]{1,})=([^\s,;]+)`)

type operationRequestHistoryPublisher interface {
	PublishOperationRequestHistoryEntry(sqlc.OperationRequestHistoryEntries)
}

type operationRequestHistorySpec struct {
	operationRequestID int64
	kind               string
	step               *string
	message            string
	details            any
}

func (w *OperationRequestWorker) SetHistoryPublisher(publisher operationRequestHistoryPublisher) {
	w.historyPublisher = publisher
}

func (w *OperationRequestWorker) withHistoryAfterCommit(ctx context.Context) context.Context {
	return store.WithAfterCommit(ctx)
}

func (w *OperationRequestWorker) recordOperationRequestAttempt(
	ctx context.Context,
	operationRequest sqlc.OperationRequests,
	attempt, maxAttempts int,
) error {
	spec := buildOperationRequestAttemptHistorySpec(operationRequest, attempt, maxAttempts)
	_, err := w.appendOperationRequestHistoryEntry(ctx, w.store.Q(), spec)
	return err
}

func (w *OperationRequestWorker) appendOperationRequestHistoryEntry(
	ctx context.Context,
	q *sqlc.Queries,
	spec operationRequestHistorySpec,
) (sqlc.OperationRequestHistoryEntries, error) {
	detailsJSON, err := marshalOperationRequestHistoryDetails(spec.details)
	if err != nil {
		return sqlc.OperationRequestHistoryEntries{}, err
	}

	entry, err := q.InsertOperationRequestHistoryEntry(ctx, sqlc.InsertOperationRequestHistoryEntryParams{
		OperationRequestID: spec.operationRequestID,
		Kind:               spec.kind,
		Step:               spec.step,
		Message:            spec.message,
		DetailsJson:        detailsJSON,
	})
	if err != nil {
		return sqlc.OperationRequestHistoryEntries{}, fmt.Errorf("insert operation request history entry: %w", err)
	}

	store.AfterCommit(ctx, func() {
		if w.historyPublisher != nil {
			w.historyPublisher.PublishOperationRequestHistoryEntry(entry)
		}
		w.logOperationRequestHistoryEntry(ctx, entry)
	})

	return entry, nil
}

func (w *OperationRequestWorker) logOperationRequestHistoryEntry(
	ctx context.Context,
	entry sqlc.OperationRequestHistoryEntries,
) {
	attrs := []any{
		slog.Int64("history_entry_id", entry.ID),
		slog.Int64("operation_request_id", entry.OperationRequestID),
		slog.String("history_kind", entry.Kind),
	}
	if entry.Step != nil {
		attrs = append(attrs, slog.String("step", *entry.Step))
	}
	w.logger.DebugContext(ctx, "recorded operation request history entry", attrs...)
}

func buildOperationRequestAttemptHistorySpec(
	operationRequest sqlc.OperationRequests,
	attempt, maxAttempts int,
) operationRequestHistorySpec {
	details := map[string]any{
		"attempt":        attempt,
		"max_attempts":   maxAttempts,
		"operation_type": operationRequest.OperationType,
		"source":         operationRequest.Source,
	}

	if attempt > 1 {
		return operationRequestHistorySpec{
			operationRequestID: operationRequest.ID,
			kind:               operations.HistoryKindRequestRetried,
			message:            "Retrying operation request.",
			details:            details,
		}
	}

	return operationRequestHistorySpec{
		operationRequestID: operationRequest.ID,
		kind:               operations.HistoryKindRequestStarted,
		message:            "Started handling operation request.",
		details:            details,
	}
}

func buildStepTransitionHistorySpec(
	operationRequestID int64,
	nextStep operations.StepStatus,
	details any,
) *operationRequestHistorySpec {
	switch nextStep.State {
	case operations.StepStateInProgress:
		return &operationRequestHistorySpec{
			operationRequestID: operationRequestID,
			kind:               operations.HistoryKindStepEntered,
			step:               strPtr(nextStep.Name),
			message:            fmt.Sprintf("Entered step %s.", operatorStepName(nextStep.Name)),
			details:            details,
		}
	case operations.StepStateCompleted:
		return &operationRequestHistorySpec{
			operationRequestID: operationRequestID,
			kind:               operations.HistoryKindStepCompleted,
			step:               strPtr(nextStep.Name),
			message:            fmt.Sprintf("Completed step %s.", operatorStepName(nextStep.Name)),
			details:            details,
		}
	case operations.StepStateFailed:
		return &operationRequestHistorySpec{
			operationRequestID: operationRequestID,
			kind:               operations.HistoryKindStepFailed,
			step:               strPtr(nextStep.Name),
			message:            fmt.Sprintf("Step %s failed.", operatorStepName(nextStep.Name)),
			details:            details,
		}
	default:
		return nil
	}
}

func buildRequestCompletedHistorySpec(
	operationRequest sqlc.OperationRequests,
	runtimeEnvironmentID *int64,
	outcome string,
) operationRequestHistorySpec {
	details := map[string]any{
		"outcome": outcome,
	}
	if runtimeEnvironmentID != nil {
		details["runtime_environment_id"] = *runtimeEnvironmentID
	}

	var step *string
	if operationRequest.CurrentStep != "" {
		step = strPtr(operationRequest.CurrentStep)
	}

	return operationRequestHistorySpec{
		operationRequestID: operationRequest.ID,
		kind:               operations.HistoryKindRequestCompleted,
		step:               step,
		message:            completionMessageForOutcome(outcome),
		details:            details,
	}
}

func buildRequestFailedHistorySpec(
	operationRequest sqlc.OperationRequests,
	cause error,
) operationRequestHistorySpec {
	details := map[string]any{
		"error_summary": sanitizeOperationRequestHistoryString(cause.Error()),
	}
	if operationRequest.CurrentStep != "" {
		details["step"] = operationRequest.CurrentStep
	}
	if operationRequest.RuntimeEnvironmentID != nil {
		details["runtime_environment_id"] = *operationRequest.RuntimeEnvironmentID
	}

	var step *string
	if operationRequest.CurrentStep != "" {
		step = strPtr(operationRequest.CurrentStep)
	}

	return operationRequestHistorySpec{
		operationRequestID: operationRequest.ID,
		kind:               operations.HistoryKindRequestFailed,
		step:               step,
		message:            "Operation request failed permanently.",
		details:            details,
	}
}

func completionMessageForOutcome(outcome string) string {
	switch outcome {
	case operations.OutcomeNewAttemptCreated:
		return "Operation request completed. A new runtime attempt is ready."
	case operations.OutcomeAlreadyPreparing:
		return "Operation request completed. The target runtime attempt was already preparing."
	case operations.OutcomeAlreadyPrepared:
		return "Operation request completed. The target runtime attempt was already prepared."
	case operations.OutcomeAlreadyDeleted:
		return "Operation request completed. The target runtime attempt was already absent."
	case operations.OutcomeAttemptSuperseded:
		return "Operation request completed. The target runtime attempt was superseded."
	case operations.OutcomeCleanupCompleted:
		return "Operation request completed. Cleanup finished."
	default:
		return fmt.Sprintf("Operation request completed with outcome %s.", outcome)
	}
}

func operatorStepName(step string) string {
	return strings.ReplaceAll(step, "_", " ")
}

func marshalOperationRequestHistoryDetails(details any) ([]byte, error) {
	if details == nil {
		return nil, nil
	}

	sanitized := sanitizeOperationRequestHistoryValue(details)
	payload, err := json.Marshal(sanitized)
	if err != nil {
		return nil, fmt.Errorf("marshal operation request history details: %w", err)
	}
	return payload, nil
}

func sanitizeOperationRequestHistoryValue(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return sanitizeOperationRequestHistoryString(typed)
	case error:
		return sanitizeOperationRequestHistoryString(typed.Error())
	case map[string]any:
		sanitized := make(map[string]any, len(typed))
		for key, nested := range typed {
			sanitized[key] = sanitizeOperationRequestHistoryValue(nested)
		}
		return sanitized
	case []any:
		sanitized := make([]any, 0, len(typed))
		for _, nested := range typed {
			sanitized = append(sanitized, sanitizeOperationRequestHistoryValue(nested))
		}
		return sanitized
	case []string:
		sanitized := make([]string, 0, len(typed))
		for _, item := range typed {
			sanitized = append(sanitized, sanitizeOperationRequestHistoryString(item))
		}
		return sanitized
	default:
		return value
	}
}

func sanitizeOperationRequestHistoryString(value string) string {
	normalized := strings.Join(strings.Fields(value), " ")
	normalized = environmentAssignmentPattern.ReplaceAllString(normalized, "$1=[REDACTED]")

	const maxLength = 240
	if len(normalized) > maxLength {
		return normalized[:maxLength-3] + "..."
	}

	return normalized
}
