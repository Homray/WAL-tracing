package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"strconv"
)

type ExecContext struct {
	Component string `json:"component"`
	Operation string `json:"operation"`
	Entity    string `json:"entity"`
	TraceID   string `json:"trace_id"`
}

func ensureTraceID(c ExecContext) ExecContext {
	if c.TraceID != "" {
		return c
	}
	var b [16]byte
	_, _ = rand.Read(b[:])
	c.TraceID = hex.EncodeToString(b[:])
	return c
}

func maybeAcquireWalTraceLock(ctx context.Context, tx *sql.Tx) error {
	if os.Getenv("WAL_TRACE_SERIALIZE") != "1" {
		return nil
	}

	project := 42
	lockID := 1

	if v := os.Getenv("WAL_TRACE_LOCK_PROJECT"); v != "" {
		if x, err := strconv.Atoi(v); err == nil {
			project = x
		}
	}
	if v := os.Getenv("WAL_TRACE_LOCK_ID"); v != "" {
		if x, err := strconv.Atoi(v); err == nil {
			lockID = x
		}
	}

	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1, $2)`, project, lockID)
	return err
}

func applyContext(ctx context.Context, tx *sql.Tx, c ExecContext) error {
	if err := maybeAcquireWalTraceLock(ctx, tx); err != nil {
		return err
	}

	c = ensureTraceID(c)

	stmts := []struct {
		name  string
		value string
	}{
		{"application_name", c.Component},
		{"app.component", c.Component},
		{"app.operation", c.Operation},
		{"app.entity", c.Entity},
		{"app.trace_id", c.TraceID},
	}

	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, `SELECT set_config($1, $2, true)`, s.name, s.value); err != nil {
			return err
		}
	}

	payload, _ := json.Marshal(c)
	_, err := tx.ExecContext(
		ctx,
		`SELECT pg_logical_emit_message(true, 'app_context', $1)`,
		string(payload),
	)
	return err
}
