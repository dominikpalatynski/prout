package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dominikpalatynski/prout/internal/config"
)

type RepositoryAction string

const (
	RepositoryActionPreviewLabel   RepositoryAction = "preview_label"
	RepositoryActionPreviewUnlabel RepositoryAction = "preview_unlabel"
)

type RepositoryRole string

const (
	RepositoryRoleNone     RepositoryRole = "none"
	RepositoryRoleRead     RepositoryRole = "read"
	RepositoryRoleTriage   RepositoryRole = "triage"
	RepositoryRoleWrite    RepositoryRole = "write"
	RepositoryRoleMaintain RepositoryRole = "maintain"
	RepositoryRoleAdmin    RepositoryRole = "admin"
)

var repositoryRolePriority = map[RepositoryRole]int{
	RepositoryRoleNone:     0,
	RepositoryRoleRead:     1,
	RepositoryRoleTriage:   2,
	RepositoryRoleWrite:    3,
	RepositoryRoleMaintain: 4,
	RepositoryRoleAdmin:    5,
}

func ValidateRepositoryPermission(action RepositoryAction, role RepositoryRole) error {

	requiredRole, err := minimumRepositoryRole(action)
	if err != nil {
		return err
	}

	normalizedRole, err := normalizeRepositoryRole(string(role), "")
	if err != nil {
		return err
	}

	if repositoryRolePriority[normalizedRole] < repositoryRolePriority[requiredRole] {
		return fmt.Errorf("repository role %q is insufficient for action %q: requires %q or higher", normalizedRole, action, requiredRole)
	}

	return nil
}

func (gh *GithubClient) ValidateRepositoryActionPermission(cfg *config.Config, action RepositoryAction, sender string) error {

	if cfg.Environment.Name == config.DevEnvironment {
		// Skip permission validation in development environment for easier testing and iteration.
		return nil
	}

	role, err := gh.GetRepositoryRole(context.Background(), sender)
	if err != nil {
		return fmt.Errorf("get repository role for %q: %w", sender, err)
	}

	if err := ValidateRepositoryPermission(action, role); err != nil {
		return fmt.Errorf("validate repository permission for %q: %w", sender, err)
	}

	return nil
}

func (gh *GithubClient) GetRepositoryRole(ctx context.Context, sender string) (RepositoryRole, error) {
	sender = strings.TrimSpace(sender)
	if sender == "" {
		return "", fmt.Errorf("sender is required")
	}

	owner := strings.TrimSpace(gh.cfg.GitHub.Repository.Owner)
	name := strings.TrimSpace(gh.cfg.GitHub.Repository.Name)
	if owner == "" || name == "" {
		return "", fmt.Errorf("github repository owner and name are required")
	}

	appJWT, err := gh.AppJWT(time.Now().UTC())
	if err != nil {
		return "", fmt.Errorf("create github app jwt: %w", err)
	}

	installationID, err := gh.GetInstallationID(ctx, appJWT, owner, name)
	if err != nil {
		return "", fmt.Errorf("get github installation id: %w", err)
	}

	installationToken, err := gh.CreateInstallationToken(ctx, appJWT, installationID)
	if err != nil {
		return "", fmt.Errorf("create github installation token: %w", err)
	}

	req, err := gh.NewRequest(ctx, http.MethodGet, gh.EndpointURL(
		fmt.Sprintf("/repos/%s/%s/collaborators/%s/permission",
			url.PathEscape(owner),
			url.PathEscape(name),
			url.PathEscape(sender),
		),
	), http.NoBody, installationToken)
	if err != nil {
		return "", fmt.Errorf("build repository permission request: %w", err)
	}

	var response struct {
		Permission string `json:"permission"`
		RoleName   string `json:"role_name"`
	}
	if err := gh.SendRequest(req, &response); err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return RepositoryRoleNone, nil
		}

		return "", fmt.Errorf("fetch repository permission: %w", err)
	}

	role, err := normalizeRepositoryRole(response.RoleName, response.Permission)
	if err != nil {
		return "", fmt.Errorf("parse repository role: %w", err)
	}

	return role, nil
}

func minimumRepositoryRole(action RepositoryAction) (RepositoryRole, error) {
	switch action {
	case RepositoryActionPreviewLabel, RepositoryActionPreviewUnlabel:
		return RepositoryRoleWrite, nil
	default:
		return "", fmt.Errorf("unsupported repository action %q", action)
	}
}

func normalizeRepositoryRole(roleName, permission string) (RepositoryRole, error) {
	for _, candidate := range []string{roleName, permission} {
		switch RepositoryRole(strings.ToLower(strings.TrimSpace(candidate))) {
		case RepositoryRoleNone:
			return RepositoryRoleNone, nil
		case RepositoryRoleRead:
			return RepositoryRoleRead, nil
		case RepositoryRoleTriage:
			return RepositoryRoleTriage, nil
		case RepositoryRoleWrite:
			return RepositoryRoleWrite, nil
		case RepositoryRoleMaintain:
			return RepositoryRoleMaintain, nil
		case RepositoryRoleAdmin:
			return RepositoryRoleAdmin, nil
		}
	}

	if strings.TrimSpace(roleName) != "" {
		return "", fmt.Errorf("unknown repository role %q", roleName)
	}
	if strings.TrimSpace(permission) != "" {
		return "", fmt.Errorf("unknown repository role %q", permission)
	}

	return "", fmt.Errorf("repository role is required")
}
