# Gateway

Добавлен POST /api/transactions/bulk?workers=4. Тело — массив прежних DTO транзакций. workers по умолчанию 4, допустимо 1–32.

После полного импорта возвращается 200 с accepted, rejected, errors (index/error, индексация с 0), unprocessed. Ошибочные элементы не мешают обработке остальных. При отмене — 504, error=request timeout и частичная сводка. Middleware сохраняет эту сводку после завершения пула. Невалидный JSON/формат массива/workers — 400, без запуска импорта.

Контекст Request передаётся в Service.ImportTransactions. Gateway не импортирует SQL/pgx или репозитории. Таймаут REQUEST_TIMEOUT=2s, для демонстрации — 350ms вместе с BULK_WORK_DELAY=200ms и workers=1.

Запуск из homework-10: `powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\start.ps1`. Docker Desktop должен работать, предыдущий Gateway и контейнеры ДЗ 9 должны быть остановлены.

Из gateway: `go test ./... -race`, `go vet ./...`. Для -race на Windows требуется C-компилятор; обычные тесты — `go test ./...`.

[Форматы запросов/ответов, PowerShell/cURL, частичный импорт и конфигурация](../README.md).
