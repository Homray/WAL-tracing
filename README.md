# WAL tracing

Сервисы:
- repo: единственный сервис, который ходит в Postgres и выполняет SQL.
- order-service: CRUD заказов (проксирует запросы в repo).
- executor-service: CRUD исполнителей (проксирует запросы в repo).
- logistics-service: связывает/разрывает связь заказ-исполнитель (проксирует запросы в repo).

Нагрузчики:
- loadgen-orders: нагружает order-service
- loadgen-executors: нагружает executor-service
- loadgen-logistics: нагружает logistics-service

## Запуск приложения
```bash
docker compose up --build
```

## Запуск трассировщика
```bash
docker compose run --rm wal-tracer
```

## Запуск нагрузчиков

### loadgen-orders
```bash
docker compose run --rm loadgen-orders
```

### loadgen-executors
```bash
docker compose run --rm loadgen-executors
```

### loadgen-logistics
```bash
docker compose run --rm loadgen-logistics
```