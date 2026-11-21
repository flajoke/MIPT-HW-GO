# Домашнее задание 9 — конкуренция, контекст и таймауты

Gateway передаёт контекст HTTP-запроса в Ledger. На запрос действует таймаут 2 секунды. Отчёт рассчитывается параллельно: отдельная горутина считает сумму каждой категории, sync.WaitGroup координирует завершение. Во время расчёта time.Ticker пишет heartbeat раз в 400 мс.

Слои ДЗ 8 сохранены: domain, repository/pg, service, app. Gateway получает интерфейс Service через фабрику ledger.Open(ctx), передаёт его в api.NewHandler и закрывает ресурсы через defer closeFn().

## Запуск

Требуются Go 1.22+ и запущенный Docker Desktop. PostgreSQL доступен на внешнем порту 15432, Redis — 6379, Gateway — 8080.

```powershell
docker compose -f .\homework-8\compose.yaml down
cd .\homework-9
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\start.ps1
```

Скрипт запускает PostgreSQL и Redis, применяет миграции Goose и запускает Gateway. У ДЗ 9 отдельный volume: прежние данные ДЗ 8 не переносятся и не удаляются. Если контейнеры предыдущей лабы уже остановлены, начинайте с `cd .\homework-9`.

Ожидается строка `Gateway listening on http://localhost:8080 (request timeout: 2s)`. Терминал сервера оставьте открытым, запросы выполняйте во втором PowerShell.

### Ручной запуск

Из homework-9:

```powershell
docker compose up -d --wait
$env:DATABASE_URL = 'postgres://postgres:postgres@localhost:15432/cashapp?sslmode=disable'
$env:REDIS_ADDR = 'localhost:6379'
$env:REDIS_DB = '0'
$env:REDIS_PASSWORD = ''
$env:REQUEST_TIMEOUT = '2s'
$env:REPORT_WORK_DELAY = '0s'
go install -tags 'no_clickhouse no_libsql no_mssql no_mysql no_sqlite3 no_vertica no_ydb' github.com/pressly/goose/v3/cmd/goose@v3.22.1
$goose = Join-Path (go env GOPATH) 'bin\goose.exe'
& $goose -dir .\ledger\migrations postgres $env:DATABASE_URL up
cd .\gateway
go run .
```

Если задан GOBIN, используйте Goose из этой папки. Миграции выполняются до запуска приложения; фабрика Ledger их не запускает. Откат последней миграции из homework-9: `& $goose -dir .\ledger\migrations postgres $env:DATABASE_URL down`. Откат удаляет соответствующую таблицу с данными.

## API

| Метод | Адрес | Успешный статус |
|---|---|---|
| POST | /api/budgets | 201 |
| GET | /api/budgets | 200 |
| POST | /api/transactions | 201 |
| GET | /api/transactions | 200 |
| GET | /api/reports/summary?from=2026-11-01&to=2026-11-30 | 200 |

Все ответы: `Content-Type: application/json; charset=utf-8`. Ошибки: `{"error":"описание"}`. Коды: 400 — неверный JSON/валидация/даты, 409 — budget exceeded, 504 — отмена или превышение таймаута, 500 — другая внутренняя ошибка, 404 — неизвестный путь, 405 — неподдерживаемый метод.

Категория без бюджета допускается. Лимит ограничивает сумму всех расходов категории. Изменение бюджета не обнуляет расходы. Проверка и запись выполняются атомарно через UnitOfWork. Суммы — числа в рублях с точностью до двух знаков после точки. PostgreSQL использует NUMERIC(14,2), сервис сравнивает лимит в целых копейках.

### Пример проверки в PowerShell

На новой БД во втором терминале:

```powershell
Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/budgets -ContentType 'application/json' -Body '{"category":"food","limit":500}'
Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/transactions -ContentType 'application/json' -Body '{"amount":450,"category":"food","description":"lunch","date":"2026-11-21"}'
Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/transactions -ContentType 'application/json' -Body '{"amount":80,"category":"transport","date":"2026-11-21"}'
Invoke-RestMethod -Uri 'http://localhost:8080/api/reports/summary?from=2026-11-01&to=2026-11-30' | ConvertTo-Json -Depth 5
```

Ожидаемый отчёт:

```json
{
  "from": "2026-11-01",
  "to": "2026-11-30",
  "total": 530,
  "categories": [
    {"category": "food", "amount": 450},
    {"category": "transport", "amount": 80}
  ]
}
```

Интервал дат включительный, категории отсортированы. Без расходов за интервал возвращаются `total: 0` и `categories: []`. Неверная дата, пропущенный параметр или from позже to дают 400.

Повторный POST создаёт новый расход; при повторении примеров на той же БД итог может отличаться или сработает лимит. Для чтения данных:

```powershell
Invoke-RestMethod -Uri http://localhost:8080/api/budgets
Invoke-RestMethod -Uri http://localhost:8080/api/transactions
```

Эквивалент для Bash:

```sh
curl -i 'http://localhost:8080/api/reports/summary?from=2026-11-01&to=2026-11-30'
```

## Таймауты запросов

REQUEST_TIMEOUT задаётся при запуске Gateway. По умолчанию — `2s`; допустимы положительные длительности Go, например `50ms`, `1s`, `3s`. Для изменения перезапустите Gateway с новой переменной. Неверное значение завершает запуск с понятным сообщением.

TimeoutMiddleware создаёт context.WithTimeout от r.Context() и передаёт его обработчику через r.WithContext(ctx). Контекст идёт в сервис и все SQL-запросы через QueryContext, QueryRowContext, ExecContext, BeginTx. Для чтения тела запроса устанавливается read deadline, если HTTP-сервер его поддерживает.

При отмене или превышении таймаута возвращается единый статус **504 Gateway Timeout**:

```json
{"error":"request timeout"}
```

Частичный ответ буферизуется и не отправляется вместо 504. Отмена кооперативная: операции сервиса и SQL завершаются по ctx. Middleware не создаёт отдельную горутину для HTTP-обработчика. Если клиент уже закрыл соединение, сервер остановит операции, но доставить ответ такому клиенту невозможно.

### Воспроизводимая проверка heartbeat и 504

На маленькой базе SQL и чтение кеша могут завершиться быстрее 1–50 мс. Для демонстрации есть REPORT_WORK_DELAY — отменяемая задержка каждой категории **до** запроса суммы. По умолчанию `0s`: искусственной задержки нет. При ненулевой задержке отчёт обходит кеш, чтобы кеш не скрывал проверяемый таймаут.

Сначала создайте расходы в примере выше. В терминале сервера нажмите Ctrl+C. Из homework-9 запустите:

```powershell
$env:REQUEST_TIMEOUT = '2s'
$env:REPORT_WORK_DELAY = '800ms'
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\start.ps1
```

Во втором терминале запросите отчёт за ноябрь. Он вернётся с 200. В серверном логе будут `report heartbeat started`, `report calculation in progress`, `report workers finished`, `report heartbeat stopped`. Независимые задержки категорий выполняются параллельно: две категории не требуют двух последовательных ожиданий по 800 мс.

Остановите сервер через Ctrl+C, затем в том же терминале:

```powershell
$env:REQUEST_TIMEOUT = '50ms'
$env:REPORT_WORK_DELAY = '800ms'
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\start.ps1
```

Во втором терминале:

```powershell
Invoke-RestMethod -Uri 'http://localhost:8080/api/reports/summary?from=2026-11-01&to=2026-11-30'
```

Ожидается 504 и `{"error":"request timeout"}`. Красный вывод Invoke-RestMethod в этом сценарии ожидаем. В серверном логе: `report workers finished`, `report heartbeat stopped`, `report calculation canceled: context deadline exceeded`, сообщение об отмене запроса. Метод ждёт завершения всех рабочих горутин и heartbeat перед возвратом.

Проверка требует хотя бы одного расхода в указанном интервале: для пустого отчёта нечего задерживать.

После проверки остановите сервер и восстановите обычный запуск:

```powershell
$env:REQUEST_TIMEOUT = '2s'
$env:REPORT_WORK_DELAY = '0s'
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\start.ps1
```

## Конкурентный расчёт и слои

- domain.ReportRepository задаёт ListCategories(ctx, from, to) и SumForPeriod(ctx, category, from, to).
- repository/pg выполняет SELECT DISTINCT категорий за интервал и SUM одной категории; все запросы используют переданный ctx.
- service запускает по одной горутине на категорию. Каждая пишет только в свой элемент заранее выделенного среза. После sync.WaitGroup.Wait один вызывающий поток объединяет результаты, поэтому общей изменяемой map у рабочих горутин нет.
- При ошибке одного запроса дочерний контекст отменяет остальные. Исходная ошибка сохраняется. При отмене родительского контекста возвращается ctx.Err().
- Heartbeat использует time.Ticker с интервалом 400 мс. При завершении или отмене вызываются Stop и ожидание фоновой горутины.
- Gateway передаёт контекст и преобразует ошибки в JSON. SQL/pgx/репозитории Gateway не импортирует.

При обычной работе отчёты кешируются в Redis на 30 секунд. После успешной записи расхода версия кеша повышается. При cache hit SQL-расчёт и heartbeat не запускаются. Пул PostgreSQL по-прежнему ограничивает число открытых соединений десятью; горутины ожидают свободное соединение с учётом ctx.

## Конфигурация

| Переменная | По умолчанию |
|---|---|
| DATABASE_URL | При наличии имеет приоритет над отдельными DB-параметрами |
| DB_HOST / DB_PORT | localhost / 15432 |
| DB_USER / DB_PASS / DB_NAME | postgres / postgres / cashapp |
| REDIS_ADDR / REDIS_DB / REDIS_PASSWORD | localhost:6379 / 0 / пусто |
| REQUEST_TIMEOUT | 2s |
| REPORT_WORK_DELAY | 0s |

Переменные PowerShell действуют в текущем окне и наследуются start.ps1. Они не сохраняются в конфигурации Windows.