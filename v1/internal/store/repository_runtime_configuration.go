package store

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
)

type RepositoryRuntimeConfiguration struct {
	RuntimeSettings      sqlc.RepositoryRuntimeSettings
	EnvironmentVariables []sqlc.RepositoryEnvironmentVariables
}

type RepositoryEnvironmentVariableInput struct {
	Name  string
	Value string
}

func (s *Store) GetRepositoryRuntimeConfiguration(
	ctx context.Context,
	repositoryID int64,
) (RepositoryRuntimeConfiguration, error) {
	runtimeSettings, err := s.Q().GetRepositoryRuntimeSettingsByRepositoryID(ctx, repositoryID)
	if err != nil {
		return RepositoryRuntimeConfiguration{}, fmt.Errorf("load repository runtime settings: %w", err)
	}

	environmentVariables, err := s.Q().ListRepositoryEnvironmentVariablesByRepositoryID(ctx, repositoryID)
	if err != nil {
		return RepositoryRuntimeConfiguration{}, fmt.Errorf("load repository environment variables: %w", err)
	}

	return RepositoryRuntimeConfiguration{
		RuntimeSettings:      runtimeSettings,
		EnvironmentVariables: environmentVariables,
	}, nil
}

func (s *Store) ReplaceRepositoryEnvironmentVariables(
	ctx context.Context,
	repositoryID int64,
	variables []RepositoryEnvironmentVariableInput,
) ([]sqlc.RepositoryEnvironmentVariables, error) {
	normalized, err := normalizeRepositoryEnvironmentVariables(variables)
	if err != nil {
		return nil, err
	}

	var stored []sqlc.RepositoryEnvironmentVariables

	if err := s.Tx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
		if err := q.DeleteRepositoryEnvironmentVariablesByRepositoryID(ctx, repositoryID); err != nil {
			return fmt.Errorf("delete repository environment variables: %w", err)
		}

		stored = make([]sqlc.RepositoryEnvironmentVariables, 0, len(normalized))
		for _, variable := range normalized {
			record, err := q.InsertRepositoryEnvironmentVariable(ctx, sqlc.InsertRepositoryEnvironmentVariableParams{
				RepositoryID: repositoryID,
				Name:         variable.Name,
				Value:        variable.Value,
			})
			if err != nil {
				return fmt.Errorf("insert repository environment variable %q: %w", variable.Name, err)
			}
			stored = append(stored, record)
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return stored, nil
}

func normalizeRepositoryEnvironmentVariables(
	variables []RepositoryEnvironmentVariableInput,
) ([]RepositoryEnvironmentVariableInput, error) {
	normalized := make([]RepositoryEnvironmentVariableInput, 0, len(variables))
	seenNames := make(map[string]struct{}, len(variables))

	for _, variable := range variables {
		name := strings.TrimSpace(variable.Name)
		if name == "" {
			return nil, fmt.Errorf("repository environment variable name is required")
		}
		if _, exists := seenNames[name]; exists {
			return nil, fmt.Errorf("repository environment variable %q is duplicated", name)
		}
		seenNames[name] = struct{}{}

		normalized = append(normalized, RepositoryEnvironmentVariableInput{
			Name:  name,
			Value: variable.Value,
		})
	}

	slices.SortFunc(normalized, func(left, right RepositoryEnvironmentVariableInput) int {
		return strings.Compare(left.Name, right.Name)
	})

	return normalized, nil
}
