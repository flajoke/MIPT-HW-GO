# Gateway

HTTP-сервер на :8080. В main вызывается `ledger.Open(ctx)`, полученный интерфейс Service передаётся в `api.NewHandler(service)`, ресурсы закрываются через `defer closeFn()`.

Gateway импортирует публичный пакет ledger. Он не открывает БД, не читает DSN, не импортирует SQL/pgx и репозитории. DTO и HTTP-контракты сохранены. Ошибки сервиса преобразуются в JSON с кодами 400/409/500.

Эндпоинты: GET/POST `/api/budgets`, GET/POST `/api/transactions`, GET `/api/reports/summary?from=YYYY-MM-DD&to=YYYY-MM-DD`. Все ответы — application/json; charset=utf-8. Пустые списки возвращаются как [].

Запуск из `homework-8`: `powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\start.ps1`. Скрипт отдельно применяет миграции и затем запускает Gateway. Можно запустить `go run .` из gateway, если инфраструктура уже готова и в текущем окне заданы DATABASE_URL/REDIS_ADDR/REDIS_DB/REDIS_PASSWORD.

`go test ./... -v` запускает HTTP-тесты с внедрённым тестовым сервисом. Интеграционный тест включается через TEST_DATABASE_URL на отдельную базу `*_test`. `go vet ./...` проверяет код.

[Полный запуск, PowerShell/cURL, проверка кеша и сохранности после перезапуска](../README.md).
