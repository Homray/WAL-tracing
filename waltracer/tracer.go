// Package waltracer содержит утилиты для внедрения контекста приложения в
// WAL PostgreSQL, чтобы анализировать нагрузку по микросервисам.
//
// # Как работает
//
// Каждая транзакция вызывает [ApplyContext] перед DML.
// Функция пишет логическое WAL-сообщение через pg_logical_emit_message
// с JSON (компонент, операция, сущность, trace ID).
//
// Анализатор (cmd/wal-analyzer) читает replication slot и считает
// расход WAL по компонентам.
//
// # Минимальная интеграция (database/sql)
//
//	tx, err := db.BeginTx(ctx, nil)
//	// ... обработка ошибки ...
//
//	if err := waltracer.ApplyContext(ctx, tx, waltracer.Op{
//	    Component: "order_service",
//	    Name:      "create_order",
//	    Entity:    "order",
//	}); err != nil {
//	    tx.Rollback()
//	    return err
//	}
//
//	// ... DML ...
//	tx.Commit()
//
// # Передача контекста HTTP
//
// Сервисы используют [Middleware] и [PropagateHeaders]:
//
//	http.ListenAndServe(":8081", waltracer.Middleware("order_service")(mux))
//
//	req, _ := http.NewRequestWithContext(ctx, "POST", upstreamURL, nil)
//	waltracer.PropagateHeaders(r, req)
//
//	op := waltracer.OpFromRequest(r)
//	waltracer.ApplyContext(ctx, tx, op)
//
// # Требования PostgreSQL
//
//   - wal_level = logical
//   - роль с REPLICATION или superuser
//   - доступ к pg_logical_emit_message
package waltracer

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
)

// Op описывает операцию в транзакции.
type Op struct {
	// Component — сервис
	Component string

	// Name — имя операции
	Name string

	// Entity — сущность
	Entity string

	// TraceID — ID запрос
	TraceID string
}

// Msg — JSON, записываемый в WAL.
type Msg struct {
	Component string `json:"component"`
	Name      string `json:"name"`
	Entity    string `json:"entity"`
	TraceID   string `json:"trace_id"`
}

// MsgPrefix — префикс WAL-сообщения для фильтрации.
const MsgPrefix = "wal_tracer"

// ApplyContext записывает Op в WAL перед DML.
// Должна вызываться в начале транзакции.
//
// Если TraceID пустой — генерируется.
//
// SQL:
//
//	SELECT pg_logical_emit_message(true, 'wal_tracer', '<json>')
func ApplyContext(ctx context.Context, tx *sql.Tx, op Op) error {
	if os.Getenv("WALTRACER_ENABLED") == "false" {
		return nil
	}
	if op.TraceID == "" {
		op.TraceID = newTraceID()
	}

	payload, err := json.Marshal(Msg{
		Component: op.Component,
		Name:      op.Name,
		Entity:    op.Entity,
		TraceID:   op.TraceID,
	})
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx,
		"SELECT pg_logical_emit_message(true, $1, $2)",
		MsgPrefix, string(payload),
	)
	return err
}

// newTraceID генерирует 16-символьный hex ID.
func newTraceID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
