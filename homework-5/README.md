# Домашнее задание 5 — REST API

Gateway принимает HTTP-запросы и вызывает Ledger через локальный импорт Go-модуля. Хранилище находится в памяти и очищается после перезапуска. При запуске транзакций и бюджетов нет.

## Запуск

Требуется Go 1.22 или новее. Из корня репозитория:

```sh
cd homework-5/gateway
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

## Тесты

Из `homework-5/gateway`:

```sh
go test ./...
go vet ./...
```

Тесты проверяют пустые списки, создание и обновление бюджета, валидацию, превышение бюджета, JSON-ответы и одновременное создание транзакций.
