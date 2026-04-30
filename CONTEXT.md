# Toolshed

Toolshed is a self-hosted GitHub automation bot for personal-scale repositories. Its first concrete capability is creating and tearing down pull-request preview environments.

## Language

**Operator**:
The person who installs, configures, and maintains Toolshed on the host.
_Avoid_: Admin, maintainer, owner

**Repository**:
A GitHub repository registered in Toolshed and managed as a unit of automation and preview behavior.
_Avoid_: Project, app, codebase

**Preview Environment**:
An ephemeral runtime environment created for one pull request and removed when it is no longer needed.
_Avoid_: Deployment, stack, sandbox

**Trigger**:
A repository-specific rule that authorizes Toolshed to start work for a pull request.
_Avoid_: Hook, command, automation

**Server Configuration**:
Install-time system configuration loaded at startup and edited outside the panel.
_Avoid_: Repo config, runtime settings

**Repository Configuration**:
Per-repository behavior managed through Toolshed and applied to later preview actions.
_Avoid_: Server config, app config

## Relationships

- An **Operator** manages one Toolshed installation
- A Toolshed installation manages one or more **Repositories**
- A **Repository** can define multiple **Triggers**
- A **Preview Environment** belongs to exactly one **Repository** and one pull request
- **Server Configuration** applies to the whole installation, while **Repository Configuration** applies to one **Repository**

## Example dialogue

> **Dev:** "If I change a repository's trigger, do I need to restart the server?"
> **Domain expert:** "No — that's **Repository Configuration**, so it should affect later preview actions without changing the install-time **Server Configuration**."

## Flagged ambiguities

- "Phase 1 foundation" was being used to include GitHub App auth, OAuth, ACME, and panel/public-domain concerns. Resolved: Phase 1 only covers the walking skeleton needed for local bootstrap, database connectivity, migrations, queue startup, health checks, and a mock ingress-to-job path.
