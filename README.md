# Telegram Auth Extractor

![CI](https://github.com/mkaascs/TelegramAuthExtractor/actions/workflows/ci.yaml/badge.svg)
![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)

Микросервис, который извлекает данные авторизации Telegram — `auth_key`, `dc_id`, `user_id` — из клиента Telegram, запущенного внутри виртуального Android. Сервис управляет устройством через ADB + uiautomator: логинится, забирает файл сессии с устройства, достаёт из него permanent-ключи авторизации и проверяет их, переавторизуясь по MTProto.

Это то, что под капотом делает любой userbot-фреймворк или платформа управления аккаунтами: вход в настоящий мобильный клиент и извлечение полученных учётных данных, чтобы переиспользовать их программно — без устройства, из чистого Go.

---

## Запуск

### Требования

- Docker + Docker Compose
- (для локального запуска) Go 1.26+, [Task](https://taskfile.dev), **PostgreSQL 18+** (схема использует `uuidv7()`) и AVD в Android Studio (образ **без** Google Play — чтобы работал `adb root`)
- (для redroid) **Linux-хост с модулями ядра `binder` / `ashmem`** — redroid запускает Android на ядре хоста и не работает на стоковом Docker Desktop для macOS / Windows

### Переменные окружения

Секреты и креды передаются только через окружение (в конфиг не коммитятся). Адрес БД задаётся в YAML (`postgres.addr`), а имя/пользователь/пароль — через env.

| Переменная | Описание | Обязательна |
|-----------------------|------------------------------------------------------|-------------|
| `CONFIG_PATH` | Путь к YAML-конфигу | Да |
| `API_KEY` | Ключ API, проверяется в заголовке `X-API-Key` | Да |
| `JWT_SECRET` | Секрет для подписи per-session токенов (HS256) | Да |
| `TELEGRAM_API_ID` | API id Telegram (с my.telegram.org) | Да |
| `TELEGRAM_API_HASH` | API hash Telegram | Да |
| `POSTGRES_DBNAME` | Имя базы | Да |
| `POSTGRES_USERNAME` | Пользователь БД | Да |
| `POSTGRES_PASSWORD` | Пароль БД | Да |
| `TELEGRAM_DC` | DC извлечённого ключа для теста переавторизации | Нет |
| `TELEGRAM_AUTH_KEY` | Извлечённый ключ для теста переавторизации (base64) | Нет |

Передаются через файл `.env` или напрямую через окружение. Для Docker используется `config/redroid.yaml` (его `adb.serial` указывает на сервис `redroid`, `postgres.addr` — на сервис `postgres`), для локального запуска — `config/local.yaml` (`emulator-5554`, `localhost:5432`).

### Запуск через Docker

```bash
cp .env.example .env          # заполнить API_KEY, JWT_SECRET, TELEGRAM_*, POSTGRES_*
docker compose up --build
```

Поднимает три сервиса: `redroid` (виртуальный Android), `postgres` (PostgreSQL 18 с томом `pg-data`) и `extractor`. Сервис стартует только после того, как Android догрузился (healthcheck по `sys.boot_completed`) и БД готова (`pg_isready`); миграции накатываются автоматически при коннекте. API отдаётся на `localhost:8080`.

Порт ADB redroid проброшен на `127.0.0.1:5555` — к устройству можно подключиться через scrcpy:

```bash
adb connect localhost:5555
```
```bash
scrcpy -s localhost:5555
```

> И gotd-клиент, и клиент Telegram в Android должны достучаться до серверов Telegram. В сетях с ограничениями запускайте хост под системным (full-tunnel) VPN, чтобы трафик контейнеров тоже шёл через туннель.

### Локальный запуск

```bash
adb devices                   # убедиться, что эмулятор подключён
# PostgreSQL 18 должен быть запущен на адресе из config/local.yaml (localhost:5432)
task run                      # запустить сервис (слушает :8080)
```

Другие полезные таски: `task test` (unit), `task test:cover` (покрытие), `task test:integration` (интеграционные тесты репозиториев на реальном Postgres через testcontainers, нужен Docker), `task lint`, `task ci` (воспроизводит CI локально), `task generate:api` (перегенерировать REST-слой), `task test:reauth` (интеграционный тест переавторизации).

### Пример конфига (`config/local.yaml`)

```yaml
env: "local" # local, dev, prod
http_server:
  port: 8080
  write_timeout: 5s
  read_timeout: 10s
service:
  session_ttl: 720h          # после этого сессия удаляется reaper'ом
  worker:
    code_wait: 2m
    cleanup_timeout: 30s
    queue_size: 10
  reaper:
    interval: 1m             # как часто чистить истёкшие сессии
  dispatcher:
    poll_interval: 5s        # как часто забирать pending-задачи
    batch_size: 10
adb:
  wait_timeout: 30s
  phone_check_timeout: 1m
  wait_interval: 0.8s
  package: "org.telegram.messenger.web"
  apk_url: "https://telegram.org/dl/android/apk"
  binary: "adb"              # или абсолютный путь, если adb не в PATH
  serial: "emulator-5554"    # "redroid:5555" для redroid
  needs_connect: false       # true для redroid (TCP)
postgres:
  addr: "localhost:5432"     # "postgres:5432" в Docker
  connection_timeout: 5s
```

---

## Возможности

- **Полная автоматизация логина** — сервис запускает Telegram, вводит номер телефона и код подтверждения через ADB, определяя экраны и закрывая первичные попапы через uiautomator (без слепых координат)
- **Ручной ввод кода** — код подтверждения передаётся через API и вводится в поле на экране; неверный код не фатален, а повторяется (`code_attempts` отслеживается и ограничен лимитом)
- **Извлечение сессии** — `tgnet.dat` читается прямо из приватного хранилища приложения с root-доступом
- **Структурный парсинг с версионным guard** — `tgnet.dat` разбирается по структуре: для каждого датацентра из файла читаются `dc_id` и permanent-ключ, а целостность каждого ключа подтверждается криптографической сверкой `auth_key_id`
- **DC из файла** — `dc_id` каждого ключа берётся из файла, домашний датацентр известен из `currentDatacenterId`; verifier лишь подтверждает ключ, подключаясь к его DC (одна попытка на ключ, без перебора)
- **Шифрование at rest** — извлечённый `auth_key` запечатывается под одноразовый ключ сессии (age/X25519) до записи в БД; сервер умеет только шифровать, расшифровать может лишь владелец `decryption_key`
- **Двухслойная авторизация** — все ручки требуют `X-API-Key`; ручки `/sessions/me` дополнительно требуют per-session capability-токен (JWT)
- **Асинхронный пайплайн** — создание задачи возвращается сразу; фоновый worker ведёт её через жизненный цикл, клиент опрашивает статус
- **Идемпотентное создание задач** — повторный запрос по номеру с активной задачей возвращает существующую, а не создаёт дубликат
- **Очистка истёкших сессий** — фоновый reaper удаляет сессии старше `session_ttl`
- **Сброс устройства** — после извлечения устройство возвращается в чистое состояние (`pm clear`), готовое к следующему номеру
- **Авто-провижининг** — если Telegram не установлен на устройстве, сервис скачивает и ставит APK при старте, с ретраями на нестабильной сети
- **OpenAPI-контракт** — REST-слой генерируется из spec-first OpenAPI 3 определения (`oapi-codegen`)
- **Graceful shutdown** — корректная остановка worker'а и HTTP-сервера по `SIGINT` / `SIGTERM`

---

## Стек технологий

| Компонент | Технология |
|----------------------------|--------------------------------------------------------|
| **Язык** | Go 1.26+ |
| **HTTP Router** | [chi](https://github.com/go-chi/chi) |
| **Кодогенерация API** | [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) (OpenAPI 3, spec-first) |
| **Клиент Telegram** | [gotd/td](https://github.com/gotd/td) (MTProto) |
| **Хранилище** | PostgreSQL 18 ([pgx](https://github.com/jackc/pgx)) + [golang-migrate](https://github.com/golang-migrate/migrate) |
| **Шифрование** | [age](https://github.com/FiloSottile/age) (X25519) |
| **Токены** | [golang-jwt](https://github.com/golang-jwt/jwt) (HS256) |
| **Управление устройством** | ADB + uiautomator |
| **Виртуальный Android** | [redroid](https://github.com/remote-android/redroid-doc) (prod) / эмулятор Android Studio (dev) |
| **Конфиг** | [cleanenv](https://github.com/ilyakaznacheev/cleanenv) + godotenv (YAML + env) |
| **Логирование** | slog (структурированный JSON / text) |
| **Тесты** | testify + httptest + [testcontainers-go](https://github.com/testcontainers/testcontainers-go) |
| **Контейнеризация** | Docker + Docker Compose |

---

## Архитектура

Чистая слоёная архитектура с зависимостями, направленными внутрь, связанными через интерфейсы:

- **`delivery`** — HTTP-хендлеры (chi) поверх сгенерированного OpenAPI-сервера, DTO-мапперы, middleware валидации запросов и авторизации (`X-API-Key` + per-session токен). Доменные ошибки маппятся в HTTP-коды; внутренние пути вызовов не утекают клиенту.
- **`services`** — бизнес-логика. `sessions.Service` — синхронный фасад, который дёргает REST-слой (создать задачу, прочитать статус, отправить код); `workers.Worker` — асинхронный исполнитель, ведущий жизненный цикл извлечения; `workers.Dispatcher` забирает pending-задачи, `workers.Reaper` чистит истёкшие.
- **`infra`** — адаптеры за интерфейсами, которые объявляют сервисы: `device` (ADB + uiautomator), `tgnet` (структурный парсер ключей), `telegram` (verifier на gotd), `storage` (PostgreSQL через pgx), `token` (JWT capability-токены) и `crypto` (запечатывание ключей через age).
- **`domain`** — сущности, статусы жизненного цикла и ошибки, без фреймворочного кода.

Один worker владеет виртуальным устройством, поэтому одновременно выполняется одна задача; остальные ждут в буферизированном канале. Интерфейсы объявлены там, где потребляются, поэтому замена эмулятора на redroid — это изменение конфига, а не кода.

---

## API

Все маршруты смонтированы под `/api/v1`. REST-слой генерируется из [`api/openapi/v1/openapi.yaml`](api/openapi/v1/openapi.yaml) — `task generate:api` перегенерирует код сервера в `gen/rest/v1/`.

Запросы валидируются против спеки на лету (`oapi-codegen/nethttp-middleware`): некорректное тело или значение, не прошедшее `pattern`/`required`, отклоняется с `400` до того, как дойдёт до хендлера.

Интерактивная документация — Swagger UI на `/swagger/` (недоступен в `prod`), спека отдаётся на `/swagger/openapi.json`.

### Авторизация

- **`X-API-Key`** — требуется на всех ручках `/api/v1`. Сравнивается с `API_KEY` в constant-time; пусто → `401`, не совпал → `403`.
- **`Authorization: Bearer <session_token>`** — требуется дополнительно на ручках `/sessions/me`. Это per-session capability-токен (JWT, HS256), который возвращается при создании сессии и несёт её id — отдельно id в URL передавать не нужно.

Токен выдаётся один раз в ответе `POST /sessions` (`session_token`). Там же при создании новой сессии приходит одноразовый `decryption_key` для расшифровки извлечённого ключа — он нигде не хранится и повторно не выдаётся.

### Sessions

| Метод | Путь | Описание |
|----------|-----------------------------|-------------------------------------------------------|
| `POST` | `/api/v1/sessions` | Создать задачу (`202` создана + `decryption_key` / `200` идемпотентно); возвращает `session_token` |
| `GET` | `/api/v1/sessions/me` | Статус задачи, включая `code_attempts` |
| `POST` | `/api/v1/sessions/me/code` | Отправить код подтверждения (`202`, без тела — операция асинхронная) |
| `GET` | `/api/v1/sessions/me/result`| Извлечённые `dc_id` / `auth_key` (armored age) / `user_id` (`409` пока не done) |
| `DELETE` | `/api/v1/sessions/me` | Отменить задачу и очистить устройство |

### Полный цикл

```bash
API_KEY=...                   # из .env

# 1. создать задачу — вернёт session_token и (на 202) decryption_key
curl -s -X POST localhost:8080/api/v1/sessions \
  -H "X-API-Key: $API_KEY" \
  -H 'Content-Type: application/json' -d '{"phone":"+79991234567"}'

TOKEN=...                      # session_token из ответа

# 2. опрашивать, пока статус не станет "waiting_for_code"
curl -s localhost:8080/api/v1/sessions/me \
  -H "X-API-Key: $API_KEY" -H "Authorization: Bearer $TOKEN"

# 3. отправить полученный код
curl -s -X POST localhost:8080/api/v1/sessions/me/code \
  -H "X-API-Key: $API_KEY" -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"code":"12345"}'

# 4. когда статус "done" — забрать результат (auth_key зашифрован)
curl -s localhost:8080/api/v1/sessions/me/result \
  -H "X-API-Key: $API_KEY" -H "Authorization: Bearer $TOKEN"
```

Полученный `auth_key` зашифрован (armored age) — как расшифровать, см. ниже.

---

## Пайплайн извлечения

Создание задачи ставит её в очередь фонового worker'а, который ведёт её через фиксированный жизненный цикл: `pending → queued → logging_in → waiting_for_code → entering_code → extracting → cleanup → done` (`pending → queued` — когда dispatcher забирает задачу; неверный код возвращает в `waiting_for_code`; любая ошибка переводит в `failed`).

1. **Логин** — Telegram запускается, номер вводится через ADB. Переходы между экранами определяются опросом дерева UI (uiautomator), известные первичные попапы закрываются по мере появления.
2. **Код** — задача блокируется до прихода кода через `POST /sessions/me/code`, затем вводит его в поле на экране. Неверный код определяется и повторяется до лимита; отсутствие кода отваливается по таймауту.
3. **Извлечение** — как только достигнут список чатов, `tgnet.dat` читается с устройства и разбирается по структуре во все permanent-ключи с их `dc_id`.
4. **Проверка** — каждый ключ пробуется против своего датацентра (`users.getUsers(InputUserSelf)`); тот, что авторизуется, подтверждает `dc_id` и `user_id`.
5. **Запечатывание** — проверенный `auth_key` шифруется под одноразовый ключ сессии (age) перед записью в БД.
6. **Очистка** — устройство сбрасывается через `pm clear` независимо от исхода, чтобы следующая задача стартовала с чистого состояния.

---

## Извлечение ключей (формат `tgnet.dat`)

`tgnet.dat` — сериализация нативного `tgnet` клиента Telegram Android: внешняя рамка (4-байтный размер payload), конфиг (версия, флаги, `currentDatacenterId`, список датацентров) и для каждого датацентра его `dc_id`, адреса и 256-байтный permanent `auth_key` с 8-байтным `auth_key_id`, где `auth_key_id = SHA1(auth_key)[12:20]` (младшие 64 бита SHA1 ключа).

Парсер разбирает файл по структуре (little-endian, с учётом версии блока датацентра) и для каждого датацентра достаёт пару `dc_id` + `auth_key`. Целостность каждого ключа подтверждается сверкой: пересчитанный `SHA1(auth_key)[12:20]` сравнивается с хранящимся в файле `auth_key_id` — расхождение означает рассинхрон разбора, и парсер падает явно, а не возвращает мусор. Так `dc_id` берётся прямо из файла, а не подбирается сетевыми попытками.

---

## Шифрование извлечённого ключа

Извлечённый `auth_key` не хранится в открытом виде. При создании сессии сервер генерирует одноразовую пару ключей age (X25519): публичный `recipient` сохраняется в сессии, а приватный `decryption_key` **один раз** возвращается в ответе `POST /sessions` и нигде не сохраняется. После проверки worker запечатывает `auth_key` на `recipient` (ASCII-armored age) и пишет шифротекст в БД. Сервер способен только шифровать — расшифровать результат может лишь владелец `decryption_key`.

Расшифровать результат можно CLI из `cmd/decrypt`: он принимает файл с ключом и файл с зашифрованным `auth_key`, печатает расшифрованный ключ в base64.

```bash
# decryption_key (из ответа на создание сессии) -> в файл
printf '%s' "$DECRYPTION_KEY" > key.txt

# auth_key из GET /sessions/me/result (armored age) -> в файл
# (блок -----BEGIN AGE ENCRYPTED FILE----- ... -----END AGE ENCRYPTED FILE-----)

go run ./cmd/decrypt key.txt auth_key.age
```

---

## Тест переавторизации

Отдельный тест доказывает, что извлечённые данные реально рабочие: берёт настоящий `auth_key` и переавторизует аккаунт по MTProto через [gotd](https://github.com/gotd/td), замыкая цикл от извлечения до переиспользования.

Так как тест ходит на живые серверы Telegram, он закрыт build-тегом `integration` и пропускается, если учётные данные не переданы — поэтому обычный `task test` и CI в сеть не ходят. Передайте извлечённый ключ (base64, уже расшифрованный), его DC и свои API-данные в `.env`:

```dotenv
TELEGRAM_AUTH_KEY=<base64 auth_key>
TELEGRAM_DC=2
TELEGRAM_API_ID=12345678
TELEGRAM_API_HASH=api_hash
```

```bash
task test:reauth
```

Тест загружает ключ в gotd-сессию на указанном DC, подключается и вызывает `users.getUsers(InputUserSelf)`; возвращённый `user_id` подтверждает, что извлечённые данные авторизуют аккаунт.

---

## CI

Каждый push и pull request в `master` / `develop` запускает workflow **CI**:

- **test** — `go build`, `go vet` и `go test -race` по всем пакетам
- **lint** — `golangci-lint`

Локально то же самое воспроизводит `task ci`.

---

## Безопасность

- Извлечение нацелено на аккаунт, который ты авторизуешь сам — код подтверждения вводится вручную, поэтому сервис не действует от имени аккаунта, к которому нет доступа
- Доступ к API закрыт: `X-API-Key` на всех ручках, per-session capability-токен на `/sessions/me`
- `auth_key` шифруется at rest (age) и возвращается только в зашифрованном виде; `decryption_key` выдаётся один раз и не хранится
- `auth_key`, `api_hash` и секреты токенов/ключей никогда не пишутся в логи (на уровне debug может появиться только нечувствительный `auth_key_id`)
- Ответы с ошибками санитизируются — клиент получает доменное сообщение, внутренние пути вызовов остаются только в логах
- Секреты (API id / hash, креды БД, `API_KEY`, `JWT_SECRET`) передаются через окружение, никогда не коммитятся в конфиг
