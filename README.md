# Telegram Auth Extractor

![CI](https://github.com/mkaascs/TelegramAuthExtractor/actions/workflows/ci.yaml/badge.svg)
![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)

A microservice that extracts Telegram authorization data — `auth_key`, `dc_id`, `user_id` — from a Telegram client running inside a virtual Android. The service drives the device over ADB + uiautomator to log in, pulls the session file off the device, parses the permanent authorization keys out of it, and verifies them by re-authenticating over MTProto.

This is the kind of session extraction a userbot framework or an account-management platform does under the hood: logging into a real mobile client and lifting the resulting credentials so they can be reused programmatically, without a device, from pure Go.

---

## Features

- **Full login automation** — the service launches Telegram, enters the phone number, and submits the confirmation code via ADB, detecting screens and dismissing first-run pop-ups through uiautomator (never blind coordinates)
- **Manual code entry** — the confirmation code is supplied through the API and typed into the on-screen field; wrong codes are retried, not fatal (`code_attempts` is tracked, capped at a limit)
- **Session extraction** — `tgnet.dat` is read straight from the app's private storage with root access
- **Version-independent key parsing** — permanent auth keys are located by a cryptographic invariant rather than fixed byte offsets, so the parser survives client-format changes with zero false positives
- **Home-DC resolution** — extracted keys are tried against Telegram's datacenters in parallel; the one that authenticates yields the home `dc_id` and the account's `user_id`
- **Asynchronous pipeline** — publishing a task returns immediately; a single background worker drives it through its lifecycle and the client polls for status
- **Idempotent task creation** — a second request for a phone number with an active task returns the existing one instead of creating a duplicate
- **Device reset** — after extraction the device is returned to a clean state (`pm clear`), ready for the next number
- **Auto-provisioning** — if Telegram is not installed on the device, the service downloads and installs the APK on startup, with retries for unstable networks
- **OpenAPI contract** — the REST layer is generated from a spec-first OpenAPI 3 definition (`oapi-codegen`)
- **Graceful shutdown** — clean drain of the worker and the HTTP server on `SIGINT` / `SIGTERM`

---

## Tech Stack

| Component | Technology |
|-----------------------|--------------------------------------------------------|
| **Language** | Go 1.26+ |
| **HTTP Router** | [chi](https://github.com/go-chi/chi) |
| **API codegen** | [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) (OpenAPI 3, spec-first) |
| **Telegram client** | [gotd/td](https://github.com/gotd/td) (MTProto) |
| **Device control** | ADB + uiautomator |
| **Virtual Android** | [redroid](https://github.com/remote-android/redroid-doc) (prod) / Android Studio emulator (dev) |
| **Config** | [cleanenv](https://github.com/ilyakaznacheev/cleanenv) + godotenv (YAML + env) |
| **Logging** | slog (structured JSON / text) |
| **Testing** | testify + httptest |
| **Containerization** | Docker + Docker Compose |

---

## Architecture

Clean layering with dependencies pointing inward, wired through interfaces:

- **`delivery`** — HTTP handlers (chi) over the generated OpenAPI server, DTO mappers, and request validation middleware. Domain errors are mapped to HTTP codes; internal call paths never leak to clients.
- **`services`** — business logic. `sessions.Service` is the synchronous facade the REST layer calls (create task, read status, submit code); `workers.Worker` is the asynchronous executor that drives the extraction lifecycle. Both share a single session repository.
- **`infra`** — adapters behind the interfaces the services declare: `device` (ADB + uiautomator), `tgnet` (the key parser), `telegram` (the gotd verifier), and `storage` (in-memory repository, swappable for a real database).
- **`domain`** — entities, lifecycle statuses, and error values, free of framework code.

A single worker owns the virtual device, so one task runs at a time; additional tasks queue on a buffered channel. Interfaces are declared where they are consumed, so swapping the emulator for redroid is a config change, not a code change.

---

## API

All routes are mounted under `/api/v1`. The REST layer is generated from [`api/openapi/v1/openapi.yaml`](api/openapi/v1/openapi.yaml) — run `task generate:api` to regenerate the server code into `gen/rest/v1/`. Open the spec in [Swagger Editor](https://editor.swagger.io) to browse it interactively.

### Sessions

| Method | Path | Description |
|----------|-------------------------------|-------------------------------------------------------|
| `POST` | `/api/v1/sessions` | Create an extraction task (`202` created / `200` idempotent) |
| `GET` | `/api/v1/sessions/{id}` | Get task status, including `code_attempts` |
| `POST` | `/api/v1/sessions/{id}/code` | Submit the confirmation code |
| `GET` | `/api/v1/sessions/{id}/result`| Get the extracted `dc_id` / `auth_key` / `user_id` (`409` until done) |
| `DELETE` | `/api/v1/sessions/{id}` | Cancel the task and clean up the device |

### Full cycle

```bash
# 1. create a task
curl -s -X POST localhost:8080/api/v1/sessions \
  -H 'Content-Type: application/json' -d '{"phone":"+79991234567"}'

# 2. poll until status is "waiting_for_code"
curl -s localhost:8080/api/v1/sessions/<id>

# 3. submit the code you received
curl -s -X POST localhost:8080/api/v1/sessions/<id>/code \
  -H 'Content-Type: application/json' -d '{"code":"12345"}'

# 4. once status is "done", fetch the result
curl -s localhost:8080/api/v1/sessions/<id>/result
```

---

## Extraction Pipeline

Creating a task enqueues it for the background worker, which drives it through a fixed lifecycle: `pending → logging_in → waiting_for_code → entering_code → extracting → cleanup → done` (a wrong code loops back to `waiting_for_code`; any failure moves to `failed`).

1. **Login** — Telegram is launched and the phone number is entered over ADB. Screen transitions are detected by polling the uiautomator UI tree, and known first-run pop-ups are dismissed as they appear.
2. **Code** — the task blocks until the code arrives via `POST /code`, then types it into the on-screen field. A wrong code is detected and retried up to a cap; a missing code times out.
3. **Extraction** — once the chat list is reached, `tgnet.dat` is read from the device and parsed into every permanent auth key it contains.
4. **Verification** — candidate keys are tried against Telegram's datacenters in parallel; the first that authenticates (`users.getUsers(InputUserSelf)`) produces the home `dc_id` and `user_id`.
5. **Cleanup** — the device is reset with `pm clear` regardless of outcome, so the next task starts from a clean state.

```yaml
worker:
  code_wait: 3m
  cleanup_timeout: 30s
  queue_size: 50
adb:
  wait_timeout: 30s
  phone_check_timeout: 1m
  wait_interval: 0.8s
  needs_connect: false   # true for redroid (TCP)
```

---

## Key Extraction (cryptographic anchor)

`tgnet.dat` stores, for each datacenter, a 256-byte permanent `auth_key` followed by its 8-byte `auth_key_id`, where `auth_key_id = SHA1(auth_key)[12:20]` (the low 64 bits of the key's SHA1).

The parser scans the file and treats a 256-byte block as an auth key **iff** the next 8 bytes equal `SHA1(block)[12:20]` — recomputing the identifier from the key and matching it against the one stored in the file. A random match has probability 1/2⁶⁴, so there are effectively zero false positives, and the method depends only on this cryptographic relation, not on the file's field layout — making it robust to Telegram-client version changes. The home datacenter is then resolved empirically by the verifier, not read from the file.

---

## Re-authorization test

A dedicated test proves that the extracted data is genuinely usable: it takes a real `auth_key` and re-authenticates the account over MTProto with [gotd](https://github.com/gotd/td), closing the loop from extraction to reuse.

Because it talks to live Telegram servers, it is guarded by the `integration` build tag and skipped unless credentials are supplied — so the regular `task test` and CI never hit the network. Provide the extracted key (base64, straight from `GET /sessions/{id}/result`) and your API credentials in `.env`:

```dotenv
TELEGRAM_AUTH_KEY=<base64 auth_key>
TELEGRAM_API_ID=12345678
TELEGRAM_API_HASH=your_api_hash
```

```bash
task test:reauth
```

The test loads the key into a gotd session, connects, and calls `users.getUsers(InputUserSelf)`; a returned `user_id` confirms the extracted credentials authenticate the account.

---

## CI

Every push and pull request to `main` / `develop` runs the **CI** workflow:

- **test** — `go build`, `go vet`, and `go test -race` across all packages
- **lint** — `golangci-lint`

---

## Quick Start

### Prerequisites

- Docker + Docker Compose
- (for local runs) Go 1.26+, [Task](https://taskfile.dev), and an Android Studio AVD (an image **without** Google Play, so `adb root` works)
- (for redroid) a **Linux host with the `binder` / `ashmem` kernel modules** — redroid runs Android on the host kernel and does not work on stock Docker Desktop for macOS / Windows

### Environment variables

| Variable | Description | Required |
|----------------------|------------------------------------------------------|----------|
| `CONFIG_PATH` | Path to the YAML config file | Yes |
| `TELEGRAM_API_ID` | Telegram API id (from my.telegram.org) | Yes |
| `TELEGRAM_API_HASH` | Telegram API hash | Yes |
| `TELEGRAM_AUTH_KEY` | Extracted key for the re-auth test (base64) | No |

Provide them via a `.env` file (loaded automatically) or the environment. For Docker use `config/docker.yaml` (its `adb.serial` points at the `redroid` service); for host runs use `config/local.yaml` (`emulator-5554`).

### Run with Docker

```bash
docker-compose up --build
```

Brings up redroid and the service; the service connects to the device over ADB, installs Telegram if missing, and exposes the API on `localhost:8080`.

> Both the gotd client and the Android Telegram client must reach Telegram's servers. On restricted networks, run the host under a system-wide (full-tunnel) VPN so container traffic is tunneled too.

### Run locally

```bash
adb devices                   # confirm the emulator is attached
task run                      # start the service (serves on :8080)
```

Other handy tasks: `task test` / `task test:cover` (coverage), `task lint`, `task generate:api` (regenerate the REST layer), `task test:reauth` (re-auth integration test).

### Config file (`config/local.yaml`)

```yaml
env: "local" # prod, dev
http_server:
  port: 8080
  write_timeout: 5s
  read_timeout: 10s
worker:
  code_wait: 3m
  cleanup_timeout: 30s
  queue_size: 50
adb:
  wait_timeout: 30s
  phone_check_timeout: 1m
  wait_interval: 0.8s
  package: "org.telegram.messenger.web"
  apk_url: "https://telegram.org/dl/android/apk"
  binary: "/Users/makaasz/Library/Android/sdk/platform-tools/adb"
  serial: "emulator-5554"
  needs_connect: false
```

---

## Security

- Extraction targets an account you authorize yourself — the confirmation code is entered manually, so the service never acts on an account you cannot access
- `auth_key` and `api_hash` are secrets and are never written to logs (only the non-sensitive `auth_key_id` may appear at debug level)
- Error responses are sanitized — domain messages reach the client, internal call paths stay in the logs
- Credentials (API id / hash) are supplied via environment, never committed to the config file
- `.env`, `tgnet.dat`, extracted key files and the Telegram APK are git-ignored