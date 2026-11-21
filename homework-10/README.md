# Домашнее задание 10 — синхронизация и каналы

Добавлен пакетный импорт транзакций через POST /api/transactions/bulk. Ledger использует каналы jobs/results, пул воркеров, WaitGroup и отдельный коллектор. Валидация и проверка бюджета выполняются прежним методом AddTransaction. Счётчики и общий список ошибок защищены sync.Mutex.

## Запуск в PowerShell

Нужны Go 1.22+ и запущенный Docker Desktop. Остановите предыдущий Gateway через Ctrl+C. Из корня MIPT_HW_GO:

```powershell
docker compose -f .\homework-9\compose.yaml down
cd .\homework-10
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\start.ps1
```

## Новый эндпоинт

`POST /api/transactions/bulk?workers=4`

Тело — JSON-массив объектов с amount, category, description, date, как у одиночной транзакции. description необязателен. Размер JSON ограничен 1 МиБ. workers — целое число от 1 до 32, по умолчанию 4. Если элементов меньше, фактически запускается не больше воркеров, чем элементов.

### Пример запроса PowerShell

На новой БД сначала создайте бюджет:

```powershell
Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/budgets -ContentType 'application/json' -Body '{"category":"bulk-food","limit":500}'

$body = @'
[
  {"amount":450,"category":"bulk-food","description":"lunch","date":"2026-11-21"},
  {"amount":80,"category":"transport","description":"bus","date":"2026-11-21"},
  {"amount":10,"category":"","date":"2026-11-21"},
  {"amount":600,"category":"bulk-food","date":"2026-11-21"}
]
'@

Invoke-RestMethod -Method Post -Uri 'http://localhost:8080/api/transactions/bulk?workers=4' -ContentType 'application/json' -Body $body | ConvertTo-Json -Depth 5
```

Ожидается **200 OK**, даже если часть элементов отклонена:

```json
{
  "accepted": 2,
  "rejected": 2,
  "errors": [
    {"index": 2, "error": "категория транзакции не должна быть пустой"},
    {"index": 3, "error": "budget exceeded"}
  ],
  "unprocessed": 0
}
```

index — индекс исходного массива, начиная с 0. Ошибки отсортированы по индексу. accepted — успешно сохранённые записи, rejected — обработанные элементы с отказом, unprocessed — элементы, не завершённые из-за отмены.

Пустой массив [] возвращает 200 с accepted=0, rejected=0, errors=[] и unprocessed=0. null и объект вместо массива, синтаксически неверный JSON, неверный тип JSON-поля или некорректный workers дают 400 без запуска импорта.

Отрицательная/нулевая сумма, пустая категория, несуществующая дата и превышение бюджета конкретного элемента попадают в errors с его индексом. Внутренняя ошибка записи помечается как internal server error; технические детали остаются в серверном логе.

Отдельные записи сохраняются независимо. Общего отката всего массива нет. При параллельной обработке порядок сохранения и выдачи ID не обязан совпадать с порядком массива. Если лимита хватает только на часть корректных расходов одной категории, состав принятых элементов может зависеть от порядка выполнения, но общий лимит не превышается.

Повторный POST повторно создаёт расходы. Приведённые числа относятся к первому запуску примера на пустой БД.

### cURL (Bash)

```sh
curl -i -X POST http://localhost:8080/api/budgets \
  -H 'Content-Type: application/json' \
  -d '{"category":"bulk-food","limit":500}'

curl -i -X POST 'http://localhost:8080/api/transactions/bulk?workers=4' \
  -H 'Content-Type: application/json' \
  -d '[{"amount":450,"category":"bulk-food","date":"2026-11-21"},{"amount":80,"category":"transport","date":"2026-11-21"},{"amount":10,"category":"","date":"2026-11-21"},{"amount":600,"category":"bulk-food","date":"2026-11-21"}]'
```

## Конвейер и синхронизация

1. ImportTransactions(ctx, input, workers) создаёт буферизованные каналы jobs и results.
2. Запускается отдельная горутина-коллектор, читающая results.
3. N воркеров читают jobs. Для каждого элемента вызывается AddTransaction с переданным контекстом: валидация, бюджет, SQL-транзакция, сохранение.
4. Отправитель кладёт индекс и транзакцию в jobs. После отправки всех элементов или отмены закрывается jobs.
5. sync.WaitGroup ожидает всех воркеров. Только после этого закрывается results.
6. Коллектор дочитывает results, завершается после закрытия канала; второй WaitGroup подтверждает его завершение.
7. Итоговая сводка возвращается после завершения всех горутин.

В коллекторе принятые/отклонённые записи и errors обновляются под одним sync.Mutex. Итог читается под тем же мьютексом. Поэтому нет гонок по счётчикам или общему срезу.

Готовый результат записи отправляется в results даже при отмене сразу после COMMIT. Коллектор продолжает чтение до завершения всех воркеров, поэтому успешно сохранённая запись не теряется из сводки. Новые работы после отмены не начинаются; отменяемая задержка и SQL-операции получают ctx.

Защита бюджета по-прежнему выполняется в PostgreSQL-транзакции с advisory-блокировкой категории. Два воркера не могут одновременно проверить один и тот же старый остаток и вместе превысить лимит.

## Отмена и частичная сводка

REQUEST_TIMEOUT по умолчанию **2s**. Используется тот же код отмены, что в ДЗ 9: **504 Gateway Timeout**. После чтения массива и начала импорта ответ при отмене содержит ошибку и частичный итог:

```json
{
  "error": "request timeout",
  "accepted": 1,
  "rejected": 0,
  "errors": [],
  "unprocessed": 4
}
```

Всегда выполняется accepted + rejected + unprocessed = длина принятого массива. Отмена не удаляет уже сохранённые записи. Элементы, прерванные отменой, не выдаются за ошибки валидации и учитываются в unprocessed. Если отдельная SQL-операция завершилась своим таймаутом при ещё действующем контексте импорта, элемент учитывается как отклонённый с operation timeout.

Если запрос отменён до чтения массива, возвращается обычный JSON {"error":"request timeout"}: количество элементов ещё неизвестно. При закрытом клиентом соединении сервер завершит операции, но доставить ответ этому клиенту невозможно.

Middleware сохраняет частичную сводку, сформированную обработчиком после завершения пула, и не подменяет её пустой ошибкой при обнаружении истёкшего контекста.

### Воспроизводимая проверка частичного результата

Есть демонстрационная переменная BULK_WORK_DELAY. По умолчанию 0s — искусственной задержки нет. При 200ms каждый воркер ждёт 200 мс перед обработкой очередного элемента с возможностью отмены по ctx.

В терминале сервера нажмите Ctrl+C. Из homework-10:

```powershell
$env:REQUEST_TIMEOUT = '350ms'
$env:BULK_WORK_DELAY = '200ms'
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\start.ps1
```

Во втором PowerShell:

```powershell
$body = @'
[
  {"amount":1,"category":"cancel-demo","date":"2026-11-21"},
  {"amount":1,"category":"cancel-demo","date":"2026-11-21"},
  {"amount":1,"category":"cancel-demo","date":"2026-11-21"},
  {"amount":1,"category":"cancel-demo","date":"2026-11-21"},
  {"amount":1,"category":"cancel-demo","date":"2026-11-21"}
]
'@
Invoke-RestMethod -Method Post -Uri 'http://localhost:8080/api/transactions/bulk?workers=1' -ContentType 'application/json' -Body $body
```

Ожидается 504 с частичной сводкой. Обычно accepted=1 и unprocessed=4; точные значения зависят от скорости БД. Красное сообщение Invoke-RestMethod при 504 ожидаемо. Сервер логирует bulk import finished с итогами, затем отмену HTTP-запроса. После ответа пул и коллектор уже завершены.

Проверьте сохранённые записи:

```powershell
Invoke-RestMethod -Uri http://localhost:8080/api/transactions
```

После демонстрации остановите сервер и восстановите обычные значения:

```powershell
$env:REQUEST_TIMEOUT = '2s'
$env:BULK_WORK_DELAY = '0s'
$env:REPORT_WORK_DELAY = '0s'
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\start.ps1
```

## Остальные API

| Метод | Путь |
|---|---|
| POST / GET | /api/budgets |
| POST / GET | /api/transactions |
| GET | /api/reports/summary?from=2026-11-01&to=2026-11-30 |

Сохранены DTO и коды: 201 для одиночного создания, 200 для чтения/отчёта, 400 для валидации, 409 для превышения бюджета одиночной транзакции, 504 для отмены, 500 для внутренних ошибок. Во всех ответах Content-Type: application/json; charset=utf-8. Пустые списки — [].

Отчёт возвращает from, to, total, categories с category/amount. Категории рассчитываются параллельно через WaitGroup, heartbeat использует time.Ticker 400 мс и останавливается при завершении/отмене. Redis кеширует отчёт на 30 секунд; успешный импорт инвалидирует кеш через прежний AddTransaction. REPORT_WORK_DELAY=800ms включает демонстрационную задержку отчёта и обход кеша; по умолчанию 0s.

## Структура и конфигурация

| Путь | Назначение |
|---|---|
| ledger/internal/domain | Сущности, контракты, BulkResult и BulkError |
| ledger/internal/service/bulk.go | Worker-pool, коллектор, мьютекс, отмена |
| ledger/internal/repository/pg | SQL-операции и блокировка категории |
| ledger/internal/app | Чтение настроек, создание ресурсов и сборка сервиса |
| gateway/internal/api/bulk.go | Разбор массива/workers, DTO сводки и HTTP-коды |
| gateway/internal/api/timeout.go | Таймаут и сохранение частичной сводки |

Gateway не импортирует SQL/pgx и репозитории. Домен не зависит от HTTP или хранения. Глобального подключения к БД нет.

| Переменная | По умолчанию |
|---|---|
| DATABASE_URL | При наличии имеет приоритет над отдельными параметрами |
| DB_HOST / DB_PORT | localhost / 15432 |
| DB_USER / DB_PASS / DB_NAME | postgres / postgres / cashapp |
| REDIS_ADDR / REDIS_DB / REDIS_PASSWORD | localhost:6379 / 0 / пусто |
| REQUEST_TIMEOUT | 2s |
| REPORT_WORK_DELAY | 0s |
| BULK_WORK_DELAY | 0s |

## Ручной запуск и миграции

Из homework-10:

```powershell
docker compose up -d --wait
$env:DATABASE_URL = 'postgres://postgres:postgres@localhost:15432/cashapp?sslmode=disable'
$env:REDIS_ADDR = 'localhost:6379'
$env:REDIS_DB = '0'
go install -tags 'no_clickhouse no_libsql no_mssql no_mysql no_sqlite3 no_vertica no_ydb' github.com/pressly/goose/v3/cmd/goose@v3.22.1
$goose = Join-Path (go env GOPATH) 'bin\goose.exe'
& $goose -dir .\ledger\migrations postgres $env:DATABASE_URL up
cd .\gateway
go run .
```

При заданном GOBIN используйте Goose из этой папки. Фабрика не запускает миграции. Для отката последней миграции из homework-10: `& $goose -dir .\ledger\migrations postgres $env:DATABASE_URL down` (удалит соответствующую таблицу с данными).

Compose создаёт базу cashapp. Остановка контейнеров: `docker compose down`. Volume сохранится; `docker compose down -v` удаляет данные и для обычной остановки не требуется.