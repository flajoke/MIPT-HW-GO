# Домашнее задание 6 — тестирование Ledger и Gateway

Gateway принимает HTTP-запросы и вызывает Ledger через локальный импорт Go-модуля. Хранилище находится в памяти и очищается после перезапуска. При запуске транзакций и бюджетов нет.

## Запуск

Требуется Go 1.22 или новее. Из корня репозитория:

```sh
cd homework-6/gateway
go run .
```

Сервер доступен по адресу http://localhost:8080. Остановка: Ctrl+C.
Модуль Ledger подключён через `require` и `replace => ../ledger` в `gateway/go.mod`.

## Эндпоинты

| Метод | Путь | Действие | Успешный статус |
|---|---|---|---|
| POST | /api/transactions | Создать транзакцию | 201 |
| GET | /api/transactions | Получить транзакции | 200 |
| POST | /api/budgets | Создать или обновить бюджет категории | 201 |
| GET | /api/budgets | Получить бюджеты | 200 |

У транзакции поля `id`, `amount`, `category`, `description`, `date`. У бюджета — `category`, `limit`. Суммы и лимиты — целые числа в рублях. Дата задаётся в формате `YYYY-MM-DD`.

Сумма и лимит должны быть больше нуля, категория не может быть пустой, дата должна существовать. Бюджет ограничивает сумму всех расходов категории. Для категории без бюджета ограничений нет. Обновление бюджета не удаляет ранее созданные транзакции.

Все ответы имеют заголовок `Content-Type: application/json; charset=utf-8`. Пустые списки возвращаются как `[]`. Ошибки имеют формат `{"error":"описание"}`: 400 — некорректный JSON или ошибка валидации, 409 — `budget exceeded`, 500 — внутренняя ошибка, 404 — неизвестный путь, 405 — неподдерживаемый метод.

Middleware выводит метод, путь и длительность каждого запроса в консоль.

## Проверка через cURL

Команды ниже предназначены для Bash. В PowerShell используйте примеры из следующего раздела.

```sh
curl -i -X POST http://localhost:8080/api/budgets -H 'Content-Type: application/json' -d '{"category":"food","limit":500}'
curl -i -X POST http://localhost:8080/api/transactions -H 'Content-Type: application/json' -d '{"amount":450,"category":"food","description":"lunch","date":"2026-10-25"}'
curl -i http://localhost:8080/api/transactions
curl -i http://localhost:8080/api/budgets
```

Первые два запроса возвращают 201, следующие — 200. Повторное создание такой же транзакции вернёт 409 и `{"error":"budget exceeded"}`. Сумма 0 вернёт 400.

## Проверка в PowerShell

Откройте второе окно PowerShell, пока сервер работает в первом:

```powershell
Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/budgets -ContentType 'application/json' -Body '{"category":"food","limit":500}'
Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/transactions -ContentType 'application/json' -Body '{"amount":450,"category":"food","description":"lunch","date":"2026-10-25"}'
Invoke-RestMethod -Uri http://localhost:8080/api/transactions
Invoke-RestMethod -Uri http://localhost:8080/api/budgets
```

## Тесты и покрытие

Тесты используют стандартную библиотеку Go без внешних зависимостей:

- `ledger/ledger_test.go`: табличные проверки транзакций и бюджетов, лимиты расходов, отказ без записи, обновление бюджета, загрузка JSON, копирование списков и очистка состояния.
- `gateway/internal/api/handlers_test.go`: HTTP-запросы через `httptest.NewRecorder` и `httptest.NewRequest`, создание и чтение бюджетов и транзакций, статусы 200/201/400/409, формат ошибок и Content-Type, пустые списки, неизвестные маршруты и методы.

Сценарии сгруппированы через `t.Run`. Независимые проверки `Validate()` выполняются с `t.Parallel()`. Тесты с общим хранилищем выполняются последовательно: в начале вызывается `ledger.Reset()`, очистка повторяется через `t.Cleanup(ledger.Reset)`.

### Запуск из корня репозитория

Ledger и Gateway — два отдельных Go-модуля. Их нужно проверять по очереди. В PowerShell:

```powershell
cd .\homework-6\ledger
go test ./... -v -cover -coverprofile=cover.out
go tool cover -html=cover.out -o cover.html
go vet ./...

cd ..\gateway
go test ./... -v -cover -coverprofile=cover.out
go tool cover -html=cover.out -o cover.html
go vet ./...
```

Важно использовать `./...` с тремя точками, чтобы запускались тесты вложенного пакета `internal/api`. Сообщение `[no test files]` для корневого пакета Gateway допустимо: HTTP-тесты находятся в `internal/api`.

Проверка гонок (при наличии настроенного C-компилятора для Go race detector):

```sh
go test -race ./...
```

Выполните эту команду отдельно из `ledger` и `gateway`.

### Достигнутое покрытие

| Область | Покрытие операторов |
|---|---:|
| Ledger | 98,5% |
| Gateway: пакет `internal/api` | 90,5% |
| Gateway: весь модуль, включая `main` | 87,0% |

Покрытие измерено командой `go test ./... -cover -coverprofile=cover.out` отдельно в каждом модуле. `main` запускает сетевой сервер и не вызывается тестами; обработчики проверяются через `httptest`. Некоторые защитные ветки внутренних ошибок остаются непокрытыми.

В каждой папке модуля находятся `cover.out` и `cover.html`. Для просмотра откройте `cover.html` в браузере, например `Start-Process .\cover.html` из PowerShell. Отчёты можно пересоздать приведёнными командами; они исключены из Git через `.gitignore`.
