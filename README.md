# WAL Tracer

Библиотека на Go для трассировки нагрузки на WAL PostgreSQL в микросервисных приложениях.

## Идея

Каждая запись в PostgreSQL порождает записи в Write-Ahead Log (WAL). Когда приложение состоит из нескольких микросервисов, все они пишут в одну базу данных, и возникает вопрос: **какой именно микросервис создаёт наибольшую нагрузку на WAL?**

`waltracer` решает эту задачу в два шага:

1. **Инструментирование приложения** — сервис, имеющий доступ к БД, вызывает `waltracer.ApplyContext` в начале каждой транзакции. Это вставляет в WAL специальное логическое сообщение с JSON-контекстом (название компонента, операция, trace ID).

2. **Анализ WAL** — `cmd/wal-analyzer` читает слот логической репликации и, встречая контекстные сообщения, связывает WAL-байты коммита с соответствующим компонентом.

Промежуточные сервисы, не имеющие доступа к БД, используют `waltracer.Middleware` и `waltracer.PropagateHeaders` для передачи контекста через HTTP-заголовки.

---

## Структура проекта

```
WAL-tracing/
├── go.mod                       # корневой модуль: waltracer (библиотека)
├── waltracer/                   # библиотечный пакет
│   ├── tracer.go                # Op, Msg, ApplyContext
│   ├── middleware.go            # Middleware, PropagateHeaders, OpFromRequest
│   └── tracer_test.go           # юнит-тесты
├── cmd/
│   └── wal-analyzer/            # анализатор WAL
│       ├── main.go
│       ├── go.mod
│       └── Dockerfile
├── examples/
│   ├── basic/                   # базовый пример (4 сервиса)
│   │   ├── docker-compose.yml
│   │   ├── repo/                # единственный сервис с доступом к БД
│   │   ├── order-service/
│   │   ├── executor-service/
│   │   ├── logistics-service/
│   │   └── loadgens/
│   ├── chain/                   # пример с цепочкой (3 сервиса)
│   │   ├── docker-compose.yml
│   │   ├── api-gateway/         # нет доступа к БД
│   │   ├── catalog-service/     # нет доступа к БД
│   │   ├── store-service/       # единственный с доступом к БД
│   │   └── loadgen/
│   └── multi-db/                # пример с прямым доступом к БД (3 сервиса)
│       ├── docker-compose.yml
│       ├── user-service/        # прямой доступ к БД
│       ├── product-service/     # прямой доступ к БД
│       ├── review-service/      # прямой доступ к БД
│       └── loadgen/
└── README.md
```

---

## Быстрый старт

### Требования к PostgreSQL

```sql
-- В postgresql.conf:
wal_level = logical
max_replication_slots = 10
max_wal_senders = 10
```

### Установка библиотеки

```bash
go get waltracer
```


### Минимальная интеграция (3 шага)

**Шаг 1.** На сервисах без прямого доступа к БД — добавить middleware:

```go
// main.go любого HTTP-сервиса
import "waltracer/waltracer"

log.Fatal(http.ListenAndServe(":8081", waltracer.Middleware("order_service")(mux)))
```

**Шаг 2.** При вызове downstream-сервисов — пробросить заголовки:

```go
req, _ := http.NewRequestWithContext(ctx, "POST", upstreamURL, nil)
waltracer.PropagateHeaders(r, req)  // r — входящий запрос
client.Do(req)
```

**Шаг 3.** На сервисе с доступом к БД — вызвать `ApplyContext` в начале транзакции:

```go
import "waltracer/waltracer"

tx, _ := db.BeginTx(ctx, nil)

// [waltracer] восстановить контекст из заголовков и записать в WAL
op := waltracer.OpFromRequest(r)
if op.Name == "" { op.Name = "create_order" }
waltracer.ApplyContext(ctx, tx, op)

// ... DML-операции ...
tx.Commit()
```

---

## API библиотеки

### `waltracer.Op` — описание операции

```go
type Op struct {
    Component string  // название микросервиса
    Name      string  // название операции
    Entity    string  // идентификатор сущности
    TraceID   string  // trace ID; генерируется автоматически, если пустой
}
```

### `waltracer.ApplyContext(ctx, tx, op)` — запись контекста в WAL

Вызывает `SELECT pg_logical_emit_message(true, 'wal_tracer', '<json>')` внутри транзакции. Сообщение транзакционное — оно появляется в WAL только при успешном коммите.

### `waltracer.Middleware(component)` — HTTP middleware

Устанавливает заголовок `X-WAL-Component` (если отсутствует) и генерирует `X-WAL-Trace-Id` (если отсутствует).

### `waltracer.PropagateHeaders(src, dst)` — проброс заголовков

Копирует все четыре WAL-заголовка из входящего запроса в исходящий.

### `waltracer.OpFromRequest(r)` — восстановление Op из заголовков

Читает `X-WAL-Component`, `X-WAL-Operation`, `X-WAL-Entity`, `X-WAL-Trace-Id` и возвращает `Op`.

### HTTP-заголовки

| Константа              | Имя заголовка      | Назначение                          |
|------------------------|--------------------|-------------------------------------|
| `HeaderComponent`      | X-WAL-Component    | Название компонента                 |
| `HeaderOperation`      | X-WAL-Operation    | Название операции                   |
| `HeaderEntity`         | X-WAL-Entity       | Идентификатор сущности              |
| `HeaderTraceID`        | X-WAL-Trace-Id     | Распределённый trace ID             |

---

## Анализатор WAL (`cmd/wal-analyzer`)

Читает слот логической репликации `test_decoding` и выводит отчёт каждые N секунд.

### Конфигурация (переменные окружения)

| Переменная |                       По умолчанию                        |         Описание           |
|------------|-----------------------------------------------------------|----------------------------|
| `DB_DSN`   | `postgres://app:app@localhost:5433/appdb?sslmode=disable` |     Строка подключения     |
| `WAL_SLOT` | `wal_tracer_slot`                                         |    Имя слота репликации    |
| `INTERVAL` | `5`                                                       | Интервал отчёта в секундах |

### Пример вывода

```
╔═══════════════════════════════════════════════════════════════════════════════════════════════════╗
║                                    WAL Tracing Report                                             ║
╠═══════════════════════════════════════════════════════════════════════════════════════════════════╣
║  Logical(B) — детерм. метрика логических изменений:  Σ len(DML data)                              ║
║  Span(B)    — приближ. оценка физического диапазона WAL: LSN(COMMIT)-LSN(ctx)                     ║
╚═══════════════════════════════════════════════════════════════════════════════════════════════════╝

  Component                  Tx  Logical(B)    Log%    Span(B)    Spn%  Log/Tx
  ─────────────────────────────────────────────────────────────────────────────────────
  executor_service          412       52314   30.8%      87420   24.1%     127
    create_executor         189
    delete_executor          41
    update_executor         182
  logistics_service         287       38901   22.9%      64200   17.7%     136
    link_executor           186
    unlink_executor         101
  order_service             619       78612   46.3%     210780   58.2%     127
    create_order            248
    delete_order             93
    update_order            278
  ─────────────────────────────────────────────────────────────────────────────────────
  TOTAL                    1318      169827             362400

  Span/Logical ratio: 2.13x

  Component                  Logical B/s
  ──────────────────────────────────────
  executor_service              10462
  logistics_service              7780
  order_service                 15722
```

### Алгоритм вычисления WAL-байт — две метрики

Анализатор вычисляет **две независимые метрики** для каждой транзакции и показывает обе:

#### Метод 1 — Logical(B)

```
Logical(tx) = Σ len(data)  для каждой DML-записи транзакции
```

Детерминированная метрика логических изменений. `test_decoding` возвращает каждую DML-запись (INSERT/UPDATE/DELETE) ровно одному транзакционному контексту. Может использоваться для сравнения компонентов и операций между собой.

#### Метод 2 — Span(B)

```
Span(tx) = LSN(COMMIT) − LSN(context message)
```

Приближённая оценка физического диапазона WAL, занятого транзакцией. Используется для понимания общего объёма WAL, но может завышаться при конкурентной нагрузке: WAL-записи параллельных транзакций перемежаются в физическом потоке, и каждая транзакция «захватывает» байты, написанные соседними.


---

## Примеры

### Basic — 4 микросервиса

```
loadgen-* → order-service  ─┐
            executor-service─┤→ repo (единственный с доступом к БД) → PostgreSQL
            logistics-service┘
```

```bash
cd examples/basic

# Запуск приложения
docker compose up --build

# Запуск нагрузчиков
docker compose --profile loadgen up

# Запуск анализатора (в отдельном терминале)
docker compose --profile tracing run wal-analyzer
```

### Chain — цепочка из 3 сервисов

```
loadgen → api-gateway → catalog-service → store-service → PostgreSQL
```

Только `store-service` имеет доступ к базе. `api-gateway` и `catalog-service` лишь пробрасывают контекст через HTTP-заголовки. Анализатор видит нагрузку, как будто создаёт её `api_gateway` — это и есть цель: атрибуция WAL к компоненту, **инициировавшему** транзакцию, а не к тому, кто физически записал данные.

```bash
cd examples/chain

docker compose up --build
docker compose --profile loadgen up
docker compose --profile tracing run wal-analyzer
```

### Multi-DB — 3 сервиса с прямым доступом к БД

```
loadgen ──► user-service    ──► PostgreSQL
        ──► product-service ──► PostgreSQL
        ──► review-service  ──► PostgreSQL
```

Все три сервиса имеют собственный пул соединений к одной БД и самостоятельно вызывают `waltracer.ApplyContext`, явно указывая своё имя как `Component`. Заголовки между сервисами не пробрасываются — каждый сервис является «инициатором» своих транзакций.

Это самый простой паттерн интеграции: нет промежуточных прокси, нет `PropagateHeaders`. Единственные изменения в коде каждого сервиса:

```go
// [waltracer] 1: обернуть mux в Middleware (генерирует trace ID)
http.ListenAndServe(addr, waltracer.Middleware("user_service")(mux))

// [waltracer] 2: вызвать ApplyContext в начале каждой транзакции
waltracer.ApplyContext(ctx, tx, waltracer.Op{
    Component: "user_service",
    Name:      "create_user",
    Entity:    "user",
    TraceID:   r.Header.Get(waltracer.HeaderTraceID),
})
```

```bash
cd examples/multi-db

docker compose up --build
docker compose --profile loadgen up
docker compose --profile tracing run wal-analyzer
```

---

## Тесты

```bash
# Юнит-тесты библиотеки (не требуют PostgreSQL)
go test ./waltracer/...

# Подробный вывод
go test ./waltracer/... -v
```