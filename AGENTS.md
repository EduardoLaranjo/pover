# AGENTS.md

Guidance for AI coding agents working in this repository.

## Purpose

pover replaces the Apprise API sidecar that runs beside Doco-CD on every Docker host
of the owner's homelab. Apprise's image uses about 134 MiB per host even with one
worker; pover should do the same job in a few MiB.

## Requirements (set by the owner)

- **Own Go implementation.** Write each notification service ourselves. Do not build
  on third-party notification libraries; Shoutrrr was considered and rejected.
- **Apprise-compatible API.** Clients that speak the Apprise API, Doco-CD first, must
  work by changing only the API URL: same `POST /notify` request, status codes and
  notify URL schemes (`pover://` for Pushover).
- **Start with Pushover.** The long-term aim is to support the services Apprise
  supports; add each one when it is needed, behind its Apprise URL scheme.
- **Small footprint.** A static binary on a `scratch` image, a few MiB of memory.
- **Distribution.** Its own GitHub repository; CI publishes a version-tagged image to
  GHCR, which the homelab pins and Renovate updates.
- **Homelab image rules.** Never use Alpine-based images, build stages included:
  use Debian, distroless or scratch. Pin exact versions, never `latest`.

## Design decisions

These follow from the requirements; change them only with the owner's agreement.

- The contract is the client's code, not Apprise's documentation alone. Doco-CD
  v0.134.0 (`internal/notification/notification.go`) sends JSON
  `{"urls","title","body","type"}`, treats `200` and `204` as success, `424` as
  partial delivery, and reads `error` or `message` from error bodies.
- Notify URLs carry credentials: never log them or return them in responses. Refer to
  URLs by position (`url 0`).
- Reject unsupported schemes and URL parameters with `400` instead of ignoring them,
  so a misconfiguration is visible.
- The image has no shell or curl, so the binary has a `healthcheck` subcommand that
  calls `GET /status`.
- Standard library only, unless a dependency is clearly justified.

## Adding a service

1. Read Apprise's plugin for the service to match its URL scheme and parameters.
2. Implement the `service` and `target` interfaces from `notify.go` in its own file,
   and register it in `main.go`.
3. Test against a fake server with `httptest`, including that credentials stay out
   of responses.
4. Document the URL format in `README.md`.

## Checks

```bash
gofmt -l .
go vet ./...
go test -race ./...
```
