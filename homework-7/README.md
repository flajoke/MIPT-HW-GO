# Домашнее задание 7 — PostgreSQL и Redis

Ledger хранит бюджеты и расходы в PostgreSQL. Gateway использует Ledger как Go-пакет через `require` и локальный `replace`. При старте Gateway открываются подключения к PostgreSQL и Redis; отдельный сервер Ledger не требуется. Данные переживают перезапуск приложения и контейнеров PostgreSQL благодаря Docker volume.

Контракты `/api/budgets` и `/api/transactions` сохранены. Добавлен отчёт `GET /api/reports/summary?from=YYYY-MM-DD&to=YYYY-MM-DD` с кешем Redis на 30 секунд.

## Что понадобится

- Go 1.22 или новее.
- Docker Desktop с запущенным Docker Engine и поддержкой Docker Compose.
- Интернет при первой загрузке Go-зависимостей и Docker-образов.
- Свободные порты 5432, 6379 и 8080. Старый Gateway из предыдущей лабы остановите через Ctrl+C.

## Быстрый запуск в PowerShell

Из корня репозитория:

```powershell
cd .\homework-7
.\start.ps1
```

Скрипт запускает PostgreSQL и Redis, при необходимости устанавливает Goose, применяет миграции и запускает Gateway. При первом старте загрузка зависимостей может занять несколько минут. Если выполнение локальных PowerShell-скриптов запрещено, используйте ручные команды ниже.

Ожидаемые сообщения: подключение Ledger к PostgreSQL, подключение к Redis, `Gateway listening on http://localhost:8080`.

## Ручной запуск

Все команды этого раздела выполняются из `homework-7`, если явно не указана другая папка.

### 1. PostgreSQL и Redis

```powershell
docker compose up -d --wait
docker compose ps
```

Compose создаёт базу `cashapp`, пользователя `postgres` с паролем `postgres` и запускает Redis без пароля. Порты доступны только на локальном компьютере. Для локального учебного запуска используются следующие переменные:

```powershell
$env:DATABASE_URL = 'postgres://postgres:postgres@localhost:5432/cashapp?sslmode=disable'
$env:REDIS_ADDR = 'localhost:6379'
$env:REDIS_DB = '0'
$env:REDIS_PASSWORD = ''
```

Если PostgreSQL уже установлен отдельно, базу можно создать командой `createdb -h localhost -U postgres cashapp`; укажите в `DATABASE_URL` свои реквизиты. Redis можно запустить отдельно: `docker run -d --name cashapp-redis -p 127.0.0.1:6379:6379 redis:7-alpine`.

### 2. Goose и миграции

```powershell
go install -tags 'no_clickhouse no_libsql no_mssql no_mysql no_sqlite3 no_vertica no_ydb' github.com/pressly/goose/v3/cmd/goose@v3.22.1
$goose = Join-Path (go env GOPATH) 'bin\goose.exe'
& $goose -dir .\ledger\migrations postgres $env:DATABASE_URL up
& $goose -dir .\ledger\migrations postgres $env:DATABASE_URL status
```

При заданном `GOBIN` используйте `goose.exe` из этой папки. Если Goose уже находится в PATH, вместо `& $goose` можно писать `goose`.

В каталоге миграций два файла: создание `budgets` и создание `expenses`. Каждый содержит секции `-- +goose Up` и `-- +goose Down`, как требует Goose. Откат последней миграции:

```powershell
& $goose -dir .\ledger\migrations postgres $env:DATABASE_URL down
```

`down` удаляет таблицу расходов вместе с её данными; повторный `down` удаляет бюджеты. После отката выполните `up`, прежде чем запускать приложение.

### 3. Проверка Ledger и запуск Gateway

В том же окне PowerShell, где заданы переменные:

```powershell
cd .\ledger
go run ./cmd/check
cd ..\gateway
go run .
```

`cmd/check` проверяет подключения и наличие таблиц, затем завершается. Gateway сам инициализирует тот же пакет Ledger при запуске. Если БД, Redis или схема недоступны, запуск завершается с понятной ошибкой.

## Переменные окружения

| Переменная | Значение по умолчанию |
|---|---|
| DATABASE_URL | Если задана, используется вместо отдельных DB-параметров |
| DB_HOST | localhost |
| DB_PORT | 5432 |
| DB_USER | postgres |
| DB_PASS | postgres |
| DB_NAME | cashapp |
| REDIS_ADDR | localhost:6379 |
| REDIS_DB | 0 |
| REDIS_PASSWORD | пусто |

При сборке DSN из DB-параметров используется `sslmode=disable`. Переменные PowerShell действуют только в текущем окне; при новом запуске задайте их снова или используйте `start.ps1`.

## API

| Метод | Адрес | Успешный статус |
|---|---|---|
| POST | /api/budgets | 201 |
| GET | /api/budgets | 200 |
| POST | /api/transactions | 201 |
| GET | /api/transactions | 200 |
| GET | /api/reports/summary?from=2026-10-01&to=2026-10-31 | 200 |

JSON транзакции: `id`, `amount`, `category`, `description`, `date`. JSON бюджета: `category`, `limit`. Суммы — числа в рублях, допустимо до двух знаков после точки, например `450.25`. Они должны быть больше 0 и меньше 1000000000000. Дата — существующая дата формата YYYY-MM-DD. Все ответы имеют `Content-Type: application/json; charset=utf-8`. Ошибки: `{"error":"описание"}`; 400 — JSON/валидация, 409 — `budget exceeded`, 500 — ошибка БД/сервера, 404 — неизвестный путь, 405 — неподдерживаемый метод.

Категория без бюджета допускается. Лимит относится к сумме всех расходов категории, а не к отдельной покупке. Обновление бюджета не сбрасывает расходы. Проверка и запись транзакции защищены SQL-транзакцией и блокировкой категории. Денежная арифметика лимита выполняется в PostgreSQL с типом NUMERIC. Список расходов сортируется по дате и ID по убыванию, бюджеты — по категории.

## Ручная проверка в PowerShell

Откройте второе окно PowerShell. На пустой БД выполните:

```powershell
Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/budgets -ContentType 'application/json' -Body '{"category":"food","limit":500}'
Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/transactions -ContentType 'application/json' -Body '{"amount":450,"category":"food","description":"lunch","date":"2026-10-25"}'
Invoke-RestMethod -Uri http://localhost:8080/api/budgets
Invoke-RestMethod -Uri http://localhost:8080/api/transactions
```

Превышение оставшихся 50 рублей — ожидается 409; красное сообщение `Invoke-RestMethod` в этом случае нормально:

```powershell
Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/transactions -ContentType 'application/json' -Body '{"amount":51,"category":"food","date":"2026-10-25"}'
```

Отчёт за включительный интервал дат, два запроса подряд:

```powershell
Invoke-RestMethod -Uri 'http://localhost:8080/api/reports/summary?from=2026-10-01&to=2026-10-31' | ConvertTo-Json -Depth 5
Invoke-RestMethod -Uri 'http://localhost:8080/api/reports/summary?from=2026-10-01&to=2026-10-31' | ConvertTo-Json -Depth 5
```

На чистом кеше в консоли Gateway будут `report cache miss`, затем `report cache hit`. JSON одинаковый: `from`, `to`, `total`, `categories` (элементы с `category` и `amount`). Отсутствие записей даёт `total: 0` и `categories: []`.

После успешного добавления расхода версия кеша повышается, следующий отчёт пересчитывается. Старые ключи удаляются по TTL 30 секунд. Изменения через SQL напрямую могут отражаться в отчёте с задержкой до TTL. Если Redis пропадёт после запуска, отчёт читается из PostgreSQL; ошибка кеша логируется.

### Эквивалентные cURL-команды для Bash

```sh
curl -i -X POST http://localhost:8080/api/budgets -H 'Content-Type: application/json' -d '{"category":"food","limit":500}'
curl -i -X POST http://localhost:8080/api/transactions -H 'Content-Type: application/json' -d '{"amount":450,"category":"food","description":"lunch","date":"2026-10-25"}'
curl -i http://localhost:8080/api/transactions
curl -i http://localhost:8080/api/budgets
curl -i 'http://localhost:8080/api/reports/summary?from=2026-10-01&to=2026-10-31'
curl -i 'http://localhost:8080/api/reports/summary?from=2026-10-01&to=2026-10-31'
```

POST создаёт новый расход при каждом вызове. Повторение примеров на той же БД может привести к ожидаемому превышению бюджета.

## Проверка сохранности и таблиц

В окне Gateway нажмите Ctrl+C, затем снова `go run .`. GET-запросы должны вернуть те же бюджеты и расходы.

Из папки `homework-7` можно посмотреть строки напрямую:

```powershell
docker compose exec postgres psql -U postgres -d cashapp -c 'SELECT * FROM budgets;'
docker compose exec postgres psql -U postgres -d cashapp -c 'SELECT * FROM expenses ORDER BY id;'
```

Остановка контейнеров: `docker compose down`. При следующем `docker compose up -d --wait` PostgreSQL использует прежний volume. Команда `docker compose down -v` удалит volume и данные; для обычной остановки она не нужна.

## Тесты

Без внешних сервисов запускаются unit-тесты валидации и проверки HTTP-ошибок. Интеграционные тесты явно помечаются SKIP, если не задана TEST_DATABASE_URL.

Из `homework-7`:

```powershell
cd .\ledger
go test ./... -v
go vet ./...
cd ..\gateway
go test ./... -v
go vet ./...
```

## Документация

- [Goose: формат SQL-миграций](https://pressly.github.io/goose/documentation/annotations/)
- [pgx: совместимость с database/sql](https://pkg.go.dev/github.com/jackc/pgx/v5/stdlib)
- [Официальный Go-клиент Redis](https://redis.io/docs/latest/develop/clients/go/)
