# Gateway

HTTP-сервер на :8080. Контракты создания и чтения транзакций и бюджетов сохранены; Ledger теперь читает и пишет PostgreSQL. Ошибки БД возвращаются как 500 с JSON `{"error":"internal server error"}`.

## Запуск

Сначала из `homework-7` запустите `docker compose up -d --wait`, установите Goose и примените миграции из ledger/migrations, как описано в [корневом README](../README.md). Затем в том же окне:

```powershell
$env:DATABASE_URL = 'postgres://postgres:postgres@localhost:5432/cashapp?sslmode=disable'
$env:REDIS_ADDR = 'localhost:6379'
$env:REDIS_DB = '0'
$env:REDIS_PASSWORD = ''
cd .\gateway
go run .
```

Gateway инициализирует Ledger при старте; отдельно запускать Ledger-сервер не нужно. Ctrl+C останавливает HTTP-сервер и закрывает подключения. Создание БД, `goose up/down` и полный порядок запуска приведены в корневой инструкции.

## API и проверка cURL (Bash)

```sh
curl -i -X POST http://localhost:8080/api/budgets -H 'Content-Type: application/json' -d '{"category":"food","limit":500}'
curl -i -X POST http://localhost:8080/api/transactions -H 'Content-Type: application/json' -d '{"amount":450,"category":"food","description":"lunch","date":"2026-10-25"}'
curl -i -X POST http://localhost:8080/api/transactions -H 'Content-Type: application/json' -d '{"amount":51,"category":"food","date":"2026-10-25"}'
curl -i http://localhost:8080/api/budgets
curl -i http://localhost:8080/api/transactions
curl -i 'http://localhost:8080/api/reports/summary?from=2026-10-01&to=2026-10-31'
curl -i 'http://localhost:8080/api/reports/summary?from=2026-10-01&to=2026-10-31'
```

На пустой БД первые два POST вернут 201, третий — 409 без вставки. GET вернёт 200. Ошибки валидации — 400. Заголовок всех ответов: `application/json; charset=utf-8`. JSON бюджета содержит category и limit, транзакции — id, amount, category, description и date.

Отчёт возвращает from, to, total и categories с category/amount. В логе на чистом кеше будет miss, при повторе — hit. TTL отчёта 30 секунд. Данные PostgreSQL сохраняются после перезапуска Gateway; для проверки повторите GET после Ctrl+C и `go run .`.

Из `gateway`: `go test ./... -v`, `go vet ./...`. Полные HTTP-тесты с БД требуют TEST_DATABASE_URL на отдельную мигрированную базу `*_test`; инструкции и PowerShell-примеры — в корневом README.
