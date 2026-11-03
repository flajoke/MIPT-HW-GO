# Ledger

Импортируемый модуль со слоями domain, repository/pg, service и app.

- domain содержит сущности, валидацию, ошибки и интерфейсы репозиториев.
- repository/pg реализует SQL-хранение и атомарную операцию с блокировкой категории.
- service проверяет входные данные и лимиты, использует только доменные контракты, управляет кешированием отчётов.
- app создаёт PostgreSQL/Redis, репозитории и Service, возвращает функцию закрытия ресурсов.

Публичная фабрика: `ledger.Open(ctx) (ledger.Service, func() error, error)`. Пакет ledger экспортирует псевдонимы типов, чтобы соседний модуль Gateway не импортировал internal. Глобального подключения нет. Миграции выполняются отдельно от фабрики.

## Запуск

Из `homework-8`: `powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\start.ps1`.

Для проверки подключений вручную из ledger:

```powershell
$env:DATABASE_URL = 'postgres://postgres:postgres@localhost:15432/cashapp?sslmode=disable'
$env:REDIS_ADDR = 'localhost:6379'
$env:REDIS_DB = '0'
$env:REDIS_PASSWORD = ''
go run ./cmd/check
```

PostgreSQL и Redis должны быть запущены, миграции применены. Параметры DB_HOST, DB_PORT, DB_USER, DB_PASS, DB_NAME используются, если DATABASE_URL отсутствует. Значения по умолчанию: localhost:15432, postgres/postgres, cashapp.

`go test ./... -v` запускает доменные и сервисные тесты, проверку направления зависимостей. Интеграционные тесты требуют TEST_DATABASE_URL на мигрированную базу `*_test` и отдельный Redis DB (TEST_REDIS_DB по умолчанию 14). `go vet ./...` проверяет код.

[Полная инструкция: создание БД, миграции up/down, переменные окружения и HTTP-проверки](../README.md).
