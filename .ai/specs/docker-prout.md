# Docker deployment

`prout` can run as a container, but it still executes host Docker commands through `/var/run/docker.sock`.

## Requirements

- Docker Engine with `docker compose`
- repository checked out under `/opt/prout`
- `server.yml` and `app_config/` present under `/opt/prout`
- `server.yml` listening on `:8090`

## Expected server layout

`prout` downloads repositories into `./prout/workspaces` and then runs `docker compose` against those extracted files. Relative bind mounts and build contexts inside those workspace compose files must resolve to the same absolute path on the host and inside the `prout` container.

Use one fixed directory on the host and in the container:

```text
/opt/prout/
  Dockerfile
  docker-compose.prout.yml
  server.yml
  app_config/
    github_app_config.yaml
    github_app_private_key.pem
  prout/
    workspaces/
```

## Start

```bash
cd /opt/prout
docker compose -f docker-compose.prout.yml up -d --build
```

Or use the Makefile shortcut:

```bash
cd /opt/prout
make docker-up
```

## Logs

```bash
cd /opt/prout
docker compose -f docker-compose.prout.yml logs -f prout
```

Or:

```bash
cd /opt/prout
make docker-logs
```

## Stop

```bash
cd /opt/prout
docker compose -f docker-compose.prout.yml down
```

Or:

```bash
cd /opt/prout
make docker-down
```

## Push to GHCR

The Makefile includes a `docker-push` target for GitHub Container Registry.

Login first:

```bash
echo "$GITHUB_TOKEN" | docker login ghcr.io -u YOUR_GITHUB_USERNAME --password-stdin
```

Push with default tags:

```bash
cd /opt/prout
make docker-push
```

By default it pushes:

- `ghcr.io/dominikpalatynski/prout:latest`
- `ghcr.io/dominikpalatynski/prout:<short-git-sha>`

You can override them:

```bash
cd /opt/prout
make docker-push GHCR_IMAGE=ghcr.io/your-org/prout IMAGE_TAG=staging
```

## CI build

The repository also includes a GitHub Actions workflow at `.github/workflows/docker-image.yml`.

It does the following:

- builds the image on GitHub-hosted Linux runners
- always builds `linux/amd64`
- validates Docker build on pull requests
- pushes to GHCR on `main`, `master`, tag pushes matching `v*`, and manual runs

This is the preferred path if local development happens on Apple Silicon but production runs on x86_64 Linux. Local `make docker-push` follows the architecture of the machine that builds it.

## Traefik

The current file-provider route in `dynamic.yml` points to `http://host.docker.internal:8090`. Keeping `server.port` at `:8090` and publishing `8090:8090` means the existing Traefik routing can stay unchanged after moving `prout` into a container.
