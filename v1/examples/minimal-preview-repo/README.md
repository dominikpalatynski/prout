# Minimal preview repo

The simplest possible repository template for end-to-end Toolshed testing.

## Files

- `compose.yml`
- `compose.local.yml`
- `Dockerfile`
- `index.html`

## Runtime settings in Toolshed

- `compose_file_path`: `compose.yml`
- `exposed_service_name`: `app`
- `exposed_service_port`: `8080`

## Local build

```sh
docker compose -f compose.yml -f compose.local.yml up --build -d
```

Then open:

```text
http://localhost:8080
```

The base `compose.yml` does not publish a host port because Toolshed strips `ports` when it renders the preview compose file. `compose.local.yml` is only for local browser testing.
