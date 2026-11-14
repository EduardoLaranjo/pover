# pover

A small server for the [Apprise API](https://github.com/caronc/apprise-api)'s
stateless `POST /notify`, enough for clients such as
[Doco-CD](https://github.com/kimdre/doco-cd). It sends to Pushover. It is a single
static binary on a `scratch` image, using about 10 MiB of memory where Apprise uses
over 100.

It is not affiliated with Apprise and supports only what is listed here.

## API

`POST /notify` with a JSON body:

```json
{"urls": "pover://USER_KEY@APP_TOKEN", "title": "Deployed", "body": "stack: web", "type": "success"}
```

- `urls`: one or more notify URLs, separated by commas or whitespace. Required.
- `body`: required. `title` is optional.
- `type`: `info` (default), `success`, `warning` or `failure`.

Responses are JSON. `200` means every URL was delivered, `400` a bad request,
`424` that at least one delivery failed. Error messages name URLs by position,
never by content, because URLs carry credentials.

`GET /status` answers `200` for health checks; `pover healthcheck` calls it, since
the image has no shell.

## Pushover URLs

```
pover://USER_KEY@APP_TOKEN[/DEVICE...][?priority=&sound=&url=&url_title=]
```

- `priority`: `low`, `moderate`, `normal`, `high`, or `-2` to `1`. Emergency priority
  is not supported.
- Titles over 250 characters and messages over 1024 are cut, ending with `…`.

## Configuration

- `POVER_LISTEN`: listen address, default `:8000`.

## Doco-CD

```yaml
services:
  doco-cd:
    environment:
      APPRISE_API_URL: http://pover:8000/notify
      APPRISE_NOTIFY_URLS: pover://USER_KEY@APP_TOKEN
  pover:
    image: ghcr.io/eduardolaranjo/pover:<version>
    read_only: true
    cap_drop: [ALL]
    security_opt: [no-new-privileges:true]
    restart: unless-stopped
```

## Releasing

Push a `vX.Y.Z` tag; CI tests, then publishes `ghcr.io/<owner>/pover:X.Y.Z`.
