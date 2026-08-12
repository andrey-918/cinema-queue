# Cinema Queue (cinema_queue)

Мини-приложение для бронирования мест в кинотеатре с **hold-сессией** (резервом на ограниченное время) и подтверждением брони.  
Сервер написан на **Go** (net/http), данные о бронях хранятся в **Redis**. Интерфейс — статический HTML/JS.

(https://github.com/andrey-918/cinema-queue/blob/main/1.png)
https://github.com/andrey-918/cinema-queue/blob/main/2.png
https://github.com/andrey-918/cinema-queue/blob/main/3.png
https://github.com/andrey-918/cinema-queue/blob/main/4.png
---

## Возможности

- Получить список фильмов.
- Для выбранного фильма получить схему мест и их состояние:
  - свободно
  - ваш резерв
  - резерв другого пользователя
  - подтверждено
- Забронировать место на время (hold).
- Подтвердить бронирование (confirm).
- Освободить hold-сессию (release) до подтверждения или при истечении TTL.

---

## Архитектура

- `cmd/main.go` — HTTP сервер и роутинг.
- `internal/booking`:
  - `domain.go` — модели и интерфейсы хранилища
  - `service.go` — слой сервиса (thin wrapper)
  - `handler.go` — HTTP-обработчики
  - `redis_store.go` — реализация хранилища на Redis (hold/confirm/release + list)
- `internal/adapters/redis/redis.go` — создание Redis клиента и ping.
- `internal/utils/utils.go` — утилита для ответа JSON.
- `static/index.html` — фронтенд: выбор фильма, показ мест, hold/confirm/release, таймер и polling.

---

## Как запустить локально

### 1) Поднять Redis через Docker Compose
```bash
docker compose up -d
```

Compose поднимает:
- `redis` (порт `6379`)
- `redis-commander` (порт `8081`) — веб-интерфейс для просмотра ключей

Проверка:
- Redis доступен по `localhost:6379`

### 2) Запуск сервера

Вариант A (через Go):
```bash
go run ./cmd
```

Сервер слушает:
- `http://localhost:8080`

Статика отдается из папки `static`, поэтому фронтенд открывается по:
- `http://localhost:8080/`

---

## API (HTTP endpoints)

Сервер использует паттерны маршрутизации вида:

- `GET /movies`
- `GET /movies/{movieID}/seats`
- `POST /movies/{movieID}/seats/{seatID}/hold`
- `PUT /sessions/{sessionID}/confirm`
- `DELETE /sessions/{sessionID}`

### 1) Получить список фильмов
**GET** `/movies`

Ответ `200 OK`:
```json
[
  {
    "id": "inception",
    "title": "Inception",
    "rows": 5,
    "seats_per_row": 8
  }
]
```

> Список фильмов зашит в `cmd/main.go`.

---

### 2) Получить список броней/состояния мест для фильма
**GET** `/movies/{movieID}/seats`

Ответ `200 OK` (массив статусов только для занятых/held/confirmed мест):
```json
[
  {
    "seat_id": "A1",
    "user_id": "....",
    "booked": true,
    "confirmed": false
  }
]
```

Логика фронтенда:
- все места, отсутствующие в ответе, считаются “available”
- `confirmed: true` окрашивает место как confirmed
- если `user_id` совпадает с текущим пользователем — “your hold”

---

### 3) Сделать hold (зарезервировать место на TTL)
**POST** `/movies/{movieID}/seats/{seatID}/hold`

Request `201 Created` (JSON body):
```json
{ "user_id": "some-user-id" }
```

Response `201 Created`:
```json
{
  "session_id": "....",
  "movieID": "inception",
  "seat_id": "A1",
  "expires_at": "2026-01-01T12:34:56Z"
}
```

Если место уже занято другим hold/confirmed, возвращается ошибка на уровне обработчика (конкретный статус кода в текущей реализации не нормализован).

---

### 4) Подтвердить сессию брони
**PUT** `/sessions/{sessionID}/confirm`

Request body:
```json
{ "user_id": "some-user-id" }
```

Response `200 OK`:
```json
{
  "session_id": "....",
  "movie_id": "inception",
  "seat_id": "A1",
  "user_id": "some-user-id",
  "status": "confirmed",
  "expires_at": ".... (опционально)"
}
```

---

### 5) Освободить сессию
**DELETE** `/sessions/{sessionID}`

Request body:
```json
{ "user_id": "some-user-id" }
```

Response:
- `204 No Content`

---

## Хранение в Redis (ключи и TTL)

В `internal/booking/redis_store.go` используется механизм:

- `defaultHoldTTL = 2 * time.Minute`

### Ключи

- `seat:{movieID}:{seatID}` — значение JSON с полями `Booking` (без учета `expiresAt` в текущем виде)
  - создается через `SET` с `NX` (только если ключ отсутствует) и `TTL = defaultHoldTTL`
- `session:{sessionID}` — ключ, хранящий ссылку на seat-key (`seat:{...}`)
  - также создается с TTL при `hold`

Функции:
- **hold**:
  - пытаемся создать `seat:...` с `NX` + TTL
  - если ключ уже есть — возвращается `ErrSeatAlreadyBooked`
  - создается `session:{id}` → значение: seat-key
- **confirm**:
  - достает `session:{sessionID}` и затем получает seat-key
  - делает `PERSIST` для обоих ключей (убирает TTL)
  - выставляет `Status = "confirmed"` и записывает обновленное JSON значение в `seat-key`
- **release**:
  - удаляет и `seat-key`, и `session-key` (в текущей реализации удаление `seat-key` происходит через `Del(ctx, sk, sessionKey(sessionID))`)

> Важно: при подтверждении TTL hold снимается (состояние становится confirmed “навсегда”, пока ключи не удалят через release).

---

## Тесты

В проекте есть тест конкуррентности:
- `internal/booking/service_test.go`

Проверка: при 100k параллельных попыток забронировать **один и тот же seat** должен выиграть ровно **один** запрос.

Запуск:
```bash
go test ./... -race
```

---

## Фронтенд (static/index.html)

Файл `static/index.html` делает:

- Генерирует `userID` на стороне браузера:
  - `crypto.randomUUID()` → урезает до 12 символов
- Загружает список фильмов (`GET /movies`)
- При выборе фильма:
  - показывает сетку мест
  - загружает состояния мест (`GET /movies/{movieID}/seats`)
  - запускает polling каждые 2 секунды
- При клике по свободному месту:
  - отправляет hold (`POST /movies/{movieID}/seats/{seatID}/hold`)
  - получает `expires_at` и запускает таймер
- Кнопки:
  - Confirm → `PUT /sessions/{sessionID}/confirm`
  - Release → `DELETE /sessions/{sessionID}`

---

## Примечания по качеству реализации

Текущая реализация обработчиков местами не возвращает статусы и ошибки стандартизированно (например, при ошибках в `handler.go` используются `log.Println` и ранний `return` без явного `http.Error`).  
Тем не менее, основной поток работы (hold/confirm/release) и модель TTL в Redis реализованы.

---

## Структура проекта

```
.
├─ docker-compose.yaml
├─ go.mod
├─ cmd/
│  └─ main.go
├─ internal/
│  ├─ adapters/
│  │  └─ redis/
│  │     └─ redis.go
│  ├─ booking/
│  │  ├─ domain.go
│  │  ├─ handler.go
│  │  ├─ redis_store.go
│  │  ├─ service.go
│  │  └─ service_test.go
│  └─ utils/
│     └─ utils.go
└─ static/
   └─ index.html
