// user-service — CRUD пользователей с прямым доступом к PostgreSQL.
//
// В этом примере все три сервиса имеют собственное подключение к БД.
// Каждый сервис самостоятельно вызывает waltracer.ApplyContext, указывая
// своё имя как Component — заголовки от других сервисов не нужны.
//
// ──────────────────────────────────────────────────────────────────────────
// [waltracer] интеграция:
//   - waltracer.Middleware генерирует X-WAL-Trace-Id для каждого запроса.
//   - waltracer.ApplyContext вызывается напрямую с фиксированным Component.
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

type Server struct {
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
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}
	return db
}

func (s *Server) initDB() {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS users (
  id         SERIAL PRIMARY KEY,
  name       TEXT NOT NULL,
  email      TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT now(),
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

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := r.URL.Query().Get("name")
	email := r.URL.Query().Get("email")
	if name == "" {
		name = "user"
	}
	if email == "" {
		email = name + "@example.com"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()

	// [waltracer] компонент задаётся явно — этот сервис сам пишет в БД
	if err := waltracer.ApplyContext(ctx, tx, waltracer.Op{
		Component: "user_service",
		Name:      "create_user",
		Entity:    "user",
		TraceID:   r.Header.Get(waltracer.HeaderTraceID),
	}); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	var id int
	err = tx.QueryRowContext(ctx,
		`INSERT INTO users(name, email) VALUES ($1, $2) RETURNING id`,
		name, email,
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

func (s *Server) update(w http.ResponseWriter, r *http.Request) {
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
	if err := waltracer.ApplyContext(ctx, tx, waltracer.Op{
		Component: "user_service",
		Name:      "update_user",
		Entity:    "user:" + strconv.Itoa(id),
		TraceID:   r.Header.Get(waltracer.HeaderTraceID),
	}); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE users SET name=name||'_upd', updated_at=now() WHERE id=$1`, id)
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

func (s *Server) delete(w http.ResponseWriter, r *http.Request) {
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
	if err := waltracer.ApplyContext(ctx, tx, waltracer.Op{
		Component: "user_service",
		Name:      "delete_user",
		Entity:    "user:" + strconv.Itoa(id),
		TraceID:   r.Header.Get(waltracer.HeaderTraceID),
	}); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id=$1`, id)
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

	s := &Server{db: db}
	s.initDB()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/users/create", s.create)
	mux.HandleFunc("/users/update", s.update)
	mux.HandleFunc("/users/delete", s.delete)

	addr := ":8094"
	log.Println("user-service listening on", addr)

	// [waltracer] middleware генерирует X-WAL-Trace-Id, который попадёт в WAL
	log.Fatal(http.ListenAndServe(addr, waltracer.Middleware("user_service")(mux)))
}
