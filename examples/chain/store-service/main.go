// store-service is the ONLY service in the chain example that talks to
// PostgreSQL. It receives requests from catalog-service, embeds the tracing
// context into every transaction, and performs the actual DML.
//
// Chain:  loadgen → api-gateway → catalog-service → store-service → PostgreSQL
//
// ──────────────────────────────────────────────────────────────────────────
// [waltracer] integration:
//   - waltracer.OpFromRequest recovers the Op (component, operation, entity,
//     trace_id) from the HTTP headers propagated through the chain.
//   - waltracer.ApplyContext writes the Op into each transaction's WAL record
//     so the analyzer can attribute WAL bytes to the originating component.
// ──────────────────────────────────────────────────────────────────────────
package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/lib/pq"

	// [waltracer]
	"waltracer/waltracer"
)

type Store struct {
	db *sql.DB
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s is not set", key)
	}
	return v
}

func connect(dsn string) *sql.DB {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal(err)
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(20)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}
	return db
}

func (s *Store) initDB() {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS items (
  id         SERIAL PRIMARY KEY,
  name       TEXT NOT NULL,
  category   TEXT NOT NULL,
  price      INT  NOT NULL DEFAULT 0,
  updated_at TIMESTAMP NOT NULL DEFAULT now()
)`)
	if err != nil {
		log.Fatal(err)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Store) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Store) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := r.URL.Query().Get("name")
	category := r.URL.Query().Get("category")
	if name == "" {
		name = "item"
	}
	if category == "" {
		category = "general"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()

	// [waltracer] recover the Op from headers set by api-gateway and
	// propagated through catalog-service, then embed it into the transaction.
	op := waltracer.OpFromRequest(r)
	if op.Name == "" {
		op.Name = "create_item"
	}
	if op.Entity == "" {
		op.Entity = "item"
	}
	if err := waltracer.ApplyContext(ctx, tx, op); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	var id int
	err = tx.QueryRowContext(ctx,
		`INSERT INTO items(name, category, price) VALUES ($1, $2, 100) RETURNING id`,
		name, category,
	).Scan(&id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func (s *Store) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil || id <= 0 {
		http.Error(w, "bad id", 400)
		return
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()

	// [waltracer]
	op := waltracer.OpFromRequest(r)
	if op.Name == "" {
		op.Name = "update_item"
	}
	if op.Entity == "" {
		op.Entity = "item:" + strconv.Itoa(id)
	}
	if err := waltracer.ApplyContext(ctx, tx, op); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE items SET price=price+10, updated_at=now() WHERE id=$1`, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if aff, _ := res.RowsAffected(); aff == 0 {
		http.Error(w, "not found", 404)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Store) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil || id <= 0 {
		http.Error(w, "bad id", 400)
		return
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()

	// [waltracer]
	op := waltracer.OpFromRequest(r)
	if op.Name == "" {
		op.Name = "delete_item"
	}
	if op.Entity == "" {
		op.Entity = "item:" + strconv.Itoa(id)
	}
	if err := waltracer.ApplyContext(ctx, tx, op); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM items WHERE id=$1`, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if aff, _ := res.RowsAffected(); aff == 0 {
		http.Error(w, "not found", 404)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func main() {
	dsn := mustEnv("DB_DSN")
	db := connect(dsn)

	s := &Store{db: db}
	s.initDB()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/items/create", s.create)
	mux.HandleFunc("/items/update", s.update)
	mux.HandleFunc("/items/delete", s.delete)

	addr := ":8093"
	log.Println("store-service listening on", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
