# Ledger

Ledger — импортируемый Go-пакет. Транзакции и бюджеты хранятся в PostgreSQL, отчёты за интервал дат кэшируются в Redis на 30 секунд.

## Запуск и миграции

Из `homework-7` запустите PostgreSQL и Redis и задайте переменные:

```powershell
docker compose up -d --wait
$env:DATABASE_URL = 'postgres://postgres:postgres@localhost:5432/cashapp?sslmode=disable'
$env:REDIS_ADDR = 'localhost:6379'
$env:REDIS_DB = '0'
$env:REDIS_PASSWORD = ''
go install -tags 'no_clickhouse no_libsql no_mssql no_mysql no_sqlite3 no_vertica no_ydb' github.com/pressly/goose/v3/cmd/goose@v3.22.1
$goose = Join-Path (go env GOPATH) 'bin\goose.exe'
& $goose -dir .\ledger\migrations postgres $env:DATABASE_URL up
cd .\ledger
go run ./cmd/check
```

Compose создаёт базу cashapp. Для отдельной установки PostgreSQL: `createdb -h localhost -U postgres cashapp`. При заданном GOBIN используйте Goose из этой папки. Откат из `homework-7`: `& $goose -dir .\ledger\migrations postgres $env:DATABASE_URL down` (удаляет таблицу и её данные).

DATABASE_URL имеет приоритет. Без неё DSN собирается из DB_HOST=localhost, DB_PORT=5432, DB_USER=postgres, DB_PASS=postgres, DB_NAME=cashapp. Redis: REDIS_ADDR=localhost:6379, REDIS_DB=0, REDIS_PASSWORD пустой. Настройки пула: 10 открытых и 5 простаивающих соединений, срок жизни 30 минут.

`cmd/check` проверяет PostgreSQL, Redis и миграции, затем завершается. Gateway самостоятельно вызывает `ledger.Init` и закрывает соединения при завершении. Отдельного HTTP-сервера Ledger нет.

## Методы

- `SetBudget(Budget) error` — валидация и UPSERT по категории.
- `ListBudgets() ([]Budget, error)` — бюджеты из БД.
- `AddTransaction(Transaction) (Transaction, error)` — валидация, проверка лимита и INSERT RETURNING id в SQL-транзакции.
- `ListTransactions() ([]Transaction, error)` — расходы из БД, дата и ID по убыванию.
- `GetReportSummary(ctx, from, to)` — SUM по категориям за включительный интервал, cache-aside в Redis.

Бюджет и все его расходы синхронизируются транзакционной advisory-блокировкой категории. Для категории без бюджета расходы разрешены. Суммы имеют точность до копеек, лимит проверяется арифметикой NUMERIC в PostgreSQL. После записи расхода версия кеша отчётов увеличивается, устаревшие ключи истекают через 30 секунд.

## Тесты

Из `ledger`: `go test ./... -v` и `go vet ./...`. Интеграционные тесты требуют отдельной мигрированной базы TEST_DATABASE_URL с именем `*_test` и Redis (TEST_REDIS_DB по умолчанию 14). Они очищают тестовые таблицы.

[Полная инструкция, cURL и проверка кеша через Gateway](../README.md).
