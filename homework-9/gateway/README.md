# Gateway

HTTP-сервер на :8080. Фабрика Ledger создаёт Service, который передаётся через конструктор обработчиков. Gateway не импортирует SQL/pgx и репозитории.

TimeoutMiddleware применяется ко всем API-маршрутам, создаёт context.WithTimeout и передаёт его через r.WithContext(ctx). Таймаут REQUEST_TIMEOUT по умолчанию 2s. При отмене/таймауте возвращаются 504 и `{"error":"request timeout"}`. Частичный ответ буферизуется до проверки контекста. Отдельная горутина HTTP-обработчика не создаётся.

Эндпоинты: GET/POST /api/budgets, GET/POST /api/transactions, GET /api/reports/summary?from=YYYY-MM-DD&to=YYYY-MM-DD. Формат отчёта: from, to, total, categories с category/amount. Ошибки дат — 400, превышение бюджета — 409, внутренние ошибки — 500. Все ответы — application/json; charset=utf-8.

Запуск из homework-9: `powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\start.ps1`. Docker Desktop должен работать. Остановите предыдущий Gateway и контейнеры ДЗ 8, чтобы освободить порты.

Для проверки 504 задайте REQUEST_TIMEOUT=50ms и REPORT_WORK_DELAY=800ms перед запуском; нужен расход в периоде отчёта. После проверки восстановите REQUEST_TIMEOUT=2s и REPORT_WORK_DELAY=0s.

Из gateway: `go test ./... -v`, `go vet ./...`; при настроенном C-компиляторе — `go test -race ./...`.

[Полная инструкция, PowerShell/cURL, успешный отчёт, heartbeat и таймаут](../README.md).
