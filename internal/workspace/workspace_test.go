package workspace

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/dominikpalatynski/prout/internal/config"
)

func TestPrepareDomainName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		wildcardDomain string
		want           string
	}{
		{
			name:           "base wildcard suffix",
			wildcardDomain: ".app.localhost",
			want:           "owner-repo-123-abcdef1.app.localhost",
		},
		{
			name:           "legacy wildcard placeholder",
			wildcardDomain: "*.app.localhost",
			want:           "owner-repo-123-abcdef1.app.localhost",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := &WorkspaceHandler{
				cfg: &config.Config{
					GitHub: config.GitHubConfig{
						Repository: config.RepositoryConfig{
							WildcardDomain: tt.wildcardDomain,
						},
					},
				},
			}

			got, err := handler.prepareDomainName(WorkspaceLocationBuilder{
				FullName: "owner/repo",
				PRNumber: 123,
				SHA:      "abcdef1234567890",
			})
			if err != nil {
				t.Fatalf("prepareDomainName returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("prepareDomainName = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDockerComposeProjectName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		location WorkspaceLocationBuilder
		want     string
		wantErr  string
	}{
		{
			name: "sanitizes repo pr and sha",
			location: WorkspaceLocationBuilder{
				FullName: "Owner/My.Repo",
				PRNumber: 123,
				SHA:      "ABCDEF1234567890",
			},
			want: "owner-my-repo-123-abcdef1234567890",
		},
		{
			name: "rejects missing repo name",
			location: WorkspaceLocationBuilder{
				PRNumber: 123,
				SHA:      "abcdef1234567890",
			},
			wantErr: "workspace location full name is required",
		},
		{
			name: "rejects missing sha",
			location: WorkspaceLocationBuilder{
				FullName: "owner/repo",
				PRNumber: 123,
			},
			wantErr: "workspace location sha is required",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := dockerComposeProjectName(tt.location)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("dockerComposeProjectName() error = nil, want %q", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("dockerComposeProjectName() error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("dockerComposeProjectName() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("dockerComposeProjectName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCommandOutputTailKeepsLastLines(t *testing.T) {
	t.Parallel()

	tail := newCommandOutputTail(3)
	for _, line := range []string{"one", "two", "three", "four"} {
		tail.Add(line)
	}

	got := tail.String()
	want := "two\nthree\nfour"
	if got != want {
		t.Fatalf("tail.String() = %q, want %q", got, want)
	}
}

func TestCommandExecutionErrorIncludesTail(t *testing.T) {
	t.Parallel()

	err := &commandExecutionError{
		command:       "docker compose up -d",
		workspacePath: "/tmp/workspace",
		tail:          "line one\nline two",
		err:           errors.New("boom"),
	}

	got := err.Error()
	for _, part := range []string{
		"docker compose up -d",
		"/tmp/workspace",
		"line one",
		"line two",
	} {
		if !strings.Contains(got, part) {
			t.Fatalf("err.Error() = %q, want substring %q", got, part)
		}
	}
}

func TestGenerateTraefikLabelsProductionUsesUniqueRouterAndProxyNetwork(t *testing.T) {
	t.Parallel()

	domain := "owner-repo-123-abcdef1.qa.test.com"
	traefikName := traefikResourceName("app", domain)

	got := generateTraefikLabels("app", domain, config.ProdEnvironment, 3000)
	want := []string{
		"traefik.enable=true",
		"traefik.docker.network=proxy",
		fmt.Sprintf("traefik.http.routers.%s.rule=Host(`%s`)", traefikName, domain),
		fmt.Sprintf("traefik.http.routers.%s.entrypoints=websecure", traefikName),
		fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port=3000", traefikName),
		fmt.Sprintf("traefik.http.routers.%s.tls=true", traefikName),
		fmt.Sprintf("traefik.http.routers.%s.tls.certresolver=letsencrypt", traefikName),
	}

	if !slices.Equal(got, want) {
		t.Fatalf("generateTraefikLabels() = %v, want %v", got, want)
	}
}

func TestGenerateTraefikLabelsPublicDomainUsesTLSOutsideProduction(t *testing.T) {
	t.Parallel()

	domain := "owner-repo-123-abcdef1.qa.test.com"
	traefikName := traefikResourceName("app", domain)

	got := generateTraefikLabels("app", domain, config.DevEnvironment, 3000)
	want := []string{
		"traefik.enable=true",
		"traefik.docker.network=proxy",
		fmt.Sprintf("traefik.http.routers.%s.rule=Host(`%s`)", traefikName, domain),
		fmt.Sprintf("traefik.http.routers.%s.entrypoints=websecure", traefikName),
		fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port=3000", traefikName),
		fmt.Sprintf("traefik.http.routers.%s.tls=true", traefikName),
		fmt.Sprintf("traefik.http.routers.%s.tls.certresolver=letsencrypt", traefikName),
	}

	if !slices.Equal(got, want) {
		t.Fatalf("generateTraefikLabels() = %v, want %v", got, want)
	}
}

func TestGenerateTraefikLabelsLocalhostDomainStaysOnHTTPOutsideProduction(t *testing.T) {
	t.Parallel()

	domain := "owner-repo-123-abcdef1.app.localhost"
	traefikName := traefikResourceName("app", domain)

	got := generateTraefikLabels("app", domain, config.DevEnvironment, 3000)
	want := []string{
		"traefik.enable=true",
		"traefik.docker.network=proxy",
		fmt.Sprintf("traefik.http.routers.%s.rule=Host(`%s`)", traefikName, domain),
		fmt.Sprintf("traefik.http.routers.%s.entrypoints=web", traefikName),
		fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port=3000", traefikName),
	}

	if !slices.Equal(got, want) {
		t.Fatalf("generateTraefikLabels() = %v, want %v", got, want)
	}
}

func TestTraefikResourceNameVariesPerDomain(t *testing.T) {
	t.Parallel()

	first := traefikResourceName("app", "owner-repo-10-abcdef1.qa.test.com")
	second := traefikResourceName("app", "owner-repo-11-fedcba1.qa.test.com")
	if first == second {
		t.Fatalf("traefikResourceName() returned the same name for different preview domains: %q", first)
	}
}

func TestParseWorkspaceLocation(t *testing.T) {
	t.Parallel()

	got, err := parseWorkspaceLocation("dominikpalatynski", "test-github-app-12-abcdef1234567890")
	if err != nil {
		t.Fatalf("parseWorkspaceLocation() error = %v", err)
	}

	want := WorkspaceLocationBuilder{
		FullName: "dominikpalatynski/test-github-app",
		PRNumber: 12,
		SHA:      "abcdef1234567890",
	}
	if got != want {
		t.Fatalf("parseWorkspaceLocation() = %+v, want %+v", got, want)
	}
}

func TestParseWorkspaceLocationRejectsInvalidDirectory(t *testing.T) {
	t.Parallel()

	for _, workspaceName := range []string{
		"",
		"repo-only",
		"repo-no-sha-12-",
		"repo-not-a-number-sha",
	} {
		if _, err := parseWorkspaceLocation("dominikpalatynski", workspaceName); err == nil {
			t.Fatalf("parseWorkspaceLocation(%q) error = nil, want error", workspaceName)
		}
	}
}

func TestSummarizeWorkspaceStatusRunning(t *testing.T) {
	t.Parallel()

	status, reason := summarizeWorkspaceStatus([]dockerPSContainer{
		{Names: "workspace-app-1", State: "running", Status: "Up 10 minutes"},
		{Names: "workspace-db-1", State: "running", Status: "Up 10 minutes"},
	})

	if status != "running" {
		t.Fatalf("summarizeWorkspaceStatus() status = %q, want %q", status, "running")
	}
	if reason != "2 containers running" {
		t.Fatalf("summarizeWorkspaceStatus() reason = %q, want %q", reason, "2 containers running")
	}
}

func TestSummarizeWorkspaceStatusDegraded(t *testing.T) {
	t.Parallel()

	status, reason := summarizeWorkspaceStatus([]dockerPSContainer{
		{Names: "workspace-app-1", State: "running", Status: "Up 10 minutes"},
		{Names: "workspace-worker-1", State: "exited", Status: "Exited (1) 10 seconds ago"},
	})

	if status != "degraded" {
		t.Fatalf("summarizeWorkspaceStatus() status = %q, want %q", status, "degraded")
	}
	if reason != "workspace-worker-1=exited" {
		t.Fatalf("summarizeWorkspaceStatus() reason = %q, want %q", reason, "workspace-worker-1=exited")
	}
}

func TestSummarizeWorkspaceStatusStopped(t *testing.T) {
	t.Parallel()

	status, reason := summarizeWorkspaceStatus(nil)
	if status != "stopped" {
		t.Fatalf("summarizeWorkspaceStatus() status = %q, want %q", status, "stopped")
	}
	if reason != "no containers found" {
		t.Fatalf("summarizeWorkspaceStatus() reason = %q, want %q", reason, "no containers found")
	}
}
