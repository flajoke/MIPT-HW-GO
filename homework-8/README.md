# Домашнее задание 8 — архитектура приложения

Ledger разделён на домен, репозитории, сервис и слой сборки зависимостей. Gateway получает интерфейс сервиса через фабрику и передаёт его в HTTP-обработчики. Глобального подключения к БД нет. HTTP-контракты из предыдущей лабы сохранены.

## Структура

| Путь | Ответственность |
|---|---|
| `ledger/internal/domain` | Transaction, Budget, отчёт, валидация, ошибки, интерфейсы репозиториев, кеша и атомарной операции |
| `ledger/internal/repository/pg` | SQL: UPSERT, SELECT, SUM, INSERT RETURNING id; транзакции и блокировка категории |
| `ledger/internal/service` | Валидация входных данных, правило бюджета, работа с отчётами и кешем через интерфейсы |
| `ledger/internal/app` | Создание соединений, репозиториев и сервиса, функция закрытия ресурсов |
| `ledger/internal/db`, `ledger/internal/cache` | Инфраструктурные адаптеры PostgreSQL и Redis |
| `ledger/ledger.go` | Публичный интерфейс Service, псевдонимы доменных типов и фабрика Open |
| `ledger/migrations` | Прежняя схема PostgreSQL и миграции Goose |
| `gateway/internal/api` | DTO, конвертеры, HTTP-обработчики и middleware |
| `gateway/main.go` | Получение сервиса, передача в обработчики, запуск и остановка сервера |

Домен не зависит от HTTP, SQL и реализаций хранения. Сервис импортирует домен и использует его интерфейсы. PostgreSQL-репозитории реализуют эти интерфейсы и не знают о Gateway. Слой app знает о конкретных адаптерах и собирает зависимости.

Gateway — отдельный Go-модуль и не может импортировать `ledger/internal/...`. Поэтому пакет `ledger` экспортирует псевдонимы доменных типов и интерфейса Service; детали реализации остаются в internal.

## Инициализация

В `gateway/main.go`:

```go
service, closeFn, err := ledger.Open(startupContext)
if err != nil {
    return err
}
defer closeFn()
handler := api.NewHandler(service)
```

Фабрика читает конфигурацию через инфраструктурные адаптеры, открывает PostgreSQL и Redis, выполняет Ping с переданным контекстом, проверяет наличие таблиц, создаёт репозитории и передаёт их сервису через конструктор. Миграции внутри фабрики не запускаются. При ошибке ресурсы освобождаются, а Gateway завершает запуск с сообщением.

Обработчики вызывают методы переданного Service с `r.Context()`. Валидация и проверка лимита выполняются в сервисе. HTTP-слой преобразует ValidationError в 400, ErrBudgetExceeded в 409, другие ошибки — в 500 без раскрытия деталей БД.

Проверка бюджета и запись остаются одной атомарной операцией. Сервис использует интерфейс UnitOfWork, а его PostgreSQL-реализация открывает транзакцию, блокирует категорию и передаёт сервису репозитории этой транзакции. Проверка лимита остаётся в сервисе; после отказа транзакция откатывается. Обновление бюджета использует ту же блокировку.

Суммы принимаются с точностью до двух знаков после точки. В PostgreSQL используется NUMERIC(14,2); в сервисе сравнение сумм выполняется в целых копейках, чтобы 0.10 + 0.20 не давало ошибку на лимите 0.30.

## Запуск в Windows

Требуются Go 1.22+ и запущенный Docker Desktop. Используется внешний порт PostgreSQL **15432**, Redis **6379**, Gateway **8080**.

Остановите предыдущий Gateway через Ctrl+C. Если контейнеры ДЗ 7 ещё работают, остановите их из корня репозитория (данные в volume сохранятся):

```powershell
docker compose -f .\homework-7\compose.yaml down
```

Распакуйте `homework-8` рядом с предыдущими лабами и из корня репозитория выполните:

```powershell
cd .\homework-8
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\start.ps1
```

Скрипт запускает PostgreSQL и Redis, применяет внешние миграции Goose и запускает Gateway. Разрешение PowerShell относится только к этому процессу. Контейнеры ДЗ 8 используют отдельный volume; прежние данные ДЗ 7 не копируются и не удаляются.

Ожидаемые сообщения: подключение Ledger к PostgreSQL, подключение к Redis и `Gateway listening on http://localhost:8080`.

### Ручной запуск

Из `homework-8`:

```powershell
docker compose up -d --wait
$env:DATABASE_URL = 'postgres://postgres:postgres@localhost:15432/cashapp?sslmode=disable'
$env:REDIS_ADDR = 'localhost:6379'
$env:REDIS_DB = '0'
$env:REDIS_PASSWORD = ''
go install -tags 'no_clickhouse no_libsql no_mssql no_mysql no_sqlite3 no_vertica no_ydb' github.com/pressly/goose/v3/cmd/goose@v3.22.1
$goose = Join-Path (go env GOPATH) 'bin\goose.exe'
& $goose -dir .\ledger\migrations postgres $env:DATABASE_URL up
cd .\gateway
go run .
```

Если задан GOBIN, используйте `goose.exe` из этой папки; при наличии Goose в PATH достаточно команды `goose`.

Compose создаёт базу `cashapp` с пользователем/паролем `postgres/postgres`. Для PostgreSQL, установленного отдельно, можно создать базу командой `createdb -h localhost -p 15432 -U postgres cashapp` и указать свои реквизиты. Значения локального учебного Compose не предназначены для публичного сервера.

Проверка подключений без запуска HTTP: из `ledger` выполните `go run ./cmd/check` с теми же переменными окружения.

Откат последней миграции из `homework-8`:

```powershell
& $goose -dir .\ledger\migrations postgres $env:DATABASE_URL down
```

Откат удаляет соответствующую таблицу с данными. После отката перед запуском приложения примените `up`.

### Конфигурация

| Переменная | Значение по умолчанию |
|---|---|
| DATABASE_URL | При наличии имеет приоритет над DB-параметрами |
| DB_HOST | localhost |
| DB_PORT | 15432 |
| DB_USER / DB_PASS | postgres / postgres |
| DB_NAME | cashapp |
| REDIS_ADDR | localhost:6379 |
| REDIS_DB | 0 |
| REDIS_PASSWORD | пусто |

При сборке DSN из отдельных параметров используется sslmode=disable. Пул PostgreSQL: 10 открытых и 5 простаивающих соединений, время жизни 30 минут. Переменные PowerShell действуют в текущем окне; `start.ps1` задаёт значения по умолчанию, если они отсутствуют.

## API и ручная проверка

| Метод | Адрес | Успешный статус |
|---|---|---|
| POST | /api/budgets | 201 |
| GET | /api/budgets | 200 |
| POST | /api/transactions | 201 |
| GET | /api/transactions | 200 |
| GET | /api/reports/summary?from=2026-10-01&to=2026-10-31 | 200 |

Формат бюджета: category, limit. Формат транзакции: id, amount, category, description, date. Пустой список — `[]`. Все ответы имеют Content-Type `application/json; charset=utf-8`. Ошибки — `{"error":"описание"}`: 400 для валидации/неверного JSON, 409 для превышения бюджета, 500 для внутренней ошибки, 404 для неизвестного пути и 405 для неподдерживаемого метода.

На пустой БД во втором окне PowerShell:

```powershell
Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/budgets -ContentType 'application/json' -Body '{"category":"food","limit":500}'
Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/transactions -ContentType 'application/json' -Body '{"amount":450,"category":"food","description":"lunch","date":"2026-10-25"}'
Invoke-RestMethod -Uri http://localhost:8080/api/budgets
Invoke-RestMethod -Uri http://localhost:8080/api/transactions
```

Превышение оставшихся 50 рублей вернёт 409 без вставки:

```powershell
Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/transactions -ContentType 'application/json' -Body '{"amount":51,"category":"food","date":"2026-10-25"}'
```

Проверка Redis:

```powershell
Invoke-RestMethod -Uri 'http://localhost:8080/api/reports/summary?from=2026-10-01&to=2026-10-31' | ConvertTo-Json -Depth 5
Invoke-RestMethod -Uri 'http://localhost:8080/api/reports/summary?from=2026-10-01&to=2026-10-31' | ConvertTo-Json -Depth 5
```

В консоли Gateway на чистом кеше будет `report cache miss`, затем `report cache hit`. Отчёт содержит from, to, total и categories с category/amount. Интервал дат включительный. TTL — 30 секунд, после успешной записи расхода версия кеша обновляется. Если Redis пропадёт после старта, отчёт читается из PostgreSQL.

Перезапустите Gateway и повторите GET: данные сохранятся. POST при каждом вызове создаёт новую транзакцию, поэтому повторение примеров на той же БД может дать ожидаемый отказ по бюджету. Для категории без бюджета расходы разрешены; обновление бюджета не обнуляет расходы.

### cURL для Bash

```sh
curl -i -X POST http://localhost:8080/api/budgets -H 'Content-Type: application/json' -d '{"category":"food","limit":500}'
curl -i -X POST http://localhost:8080/api/transactions -H 'Content-Type: application/json' -d '{"amount":450,"category":"food","description":"lunch","date":"2026-10-25"}'
curl -i -X POST http://localhost:8080/api/transactions -H 'Content-Type: application/json' -d '{"amount":51,"category":"food","date":"2026-10-25"}'
curl -i http://localhost:8080/api/budgets
curl -i http://localhost:8080/api/transactions
curl -i 'http://localhost:8080/api/reports/summary?from=2026-10-01&to=2026-10-31'
curl -i 'http://localhost:8080/api/reports/summary?from=2026-10-01&to=2026-10-31'
```

## Тесты

Из `homework-8`:

```powershell
cd .\ledger
go test ./... -v
go vet ./...
cd ..\gateway
go test ./... -v
go vet ./...
```

Доменные табличные тесты и сценарии сервиса запускаются без Docker. Репозитории внедряются через интерфейсы; в unit-тестах используются тестовые реализации. Проверяются валидация, UPSERT, лимиты, расход ровно до лимита, точность копеек, отсутствие записи при ошибке, конкурентные операции, кеш и ошибки репозиториев. Независимые тесты используют t.Parallel, сценарии оформлены через t.Run.

HTTP-тесты используют httptest и переданный Service: проверяют DTO, JSON, статусы, пустые списки, ошибки 400/409/500 и заголовки. Тест направления зависимостей проверяет импорты производственного кода. Сообщение `[no test files]` для пакета запуска Gateway нормально: HTTP-тесты лежат в internal/api.

При настроенном C-компиляторе: `go test -race ./...`. Покрытие из каждой папки модуля:

```sh
go test ./... -cover -coverprofile=cover.out
go tool cover -html=cover.out -o cover.html
```

### Интеграционные тесты

Без TEST_DATABASE_URL они отмечаются SKIP. Нужны PostgreSQL с применёнными миграциями и Redis. Используйте отдельную базу `cashapp_test`; тесты Ledger очищают её таблицы. Из `homework-8`:

```powershell
docker compose exec postgres createdb -U postgres cashapp_test
$env:TEST_DATABASE_URL = 'postgres://postgres:postgres@localhost:15432/cashapp_test?sslmode=disable'
$env:REDIS_ADDR = 'localhost:6379'
$env:TEST_REDIS_DB = '14'
& $goose -dir .\ledger\migrations postgres $env:TEST_DATABASE_URL up
cd .\ledger
go test ./... -v
cd ..\gateway
go test ./... -v
```