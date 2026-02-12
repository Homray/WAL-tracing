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
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(20)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}
	return db
}

func (s *Server) initDB() {
	ddl := `
CREATE TABLE IF NOT EXISTS orders (
  id SERIAL PRIMARY KEY,
  status TEXT NOT NULL,
  amount INT NOT NULL,
  updated_at TIMESTAMP NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS executors (
  id SERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  role TEXT NOT NULL,
  active BOOLEAN NOT NULL DEFAULT true,
  updated_at TIMESTAMP NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS order_executors (
  order_id INT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  executor_id INT NOT NULL REFERENCES executors(id) ON DELETE CASCADE,
  created_at TIMESTAMP NOT NULL DEFAULT now(),
  PRIMARY KEY (order_id, executor_id)
);

CREATE INDEX IF NOT EXISTS idx_order_executors_executor_id ON order_executors(executor_id);
`
	if _, err := s.db.Exec(ddl); err != nil {
		log.Fatal(err)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func parseIntQuery(r *http.Request, key string) (int, bool) {
	v := r.URL.Query().Get(key)
	if v == "" {
		return 0, false
	}
	x, err := strconv.Atoi(v)
	if err != nil || x <= 0 {
		return 0, false
	}
	return x, true
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// --- Orders ---

func (s *Server) ordersCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()

	var id int
	err = tx.QueryRowContext(
		ctx,
		`INSERT INTO orders(status, amount) VALUES ('new', 100) RETURNING id`,
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

func (s *Server) ordersUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := parseIntQuery(r, "id")
	if !ok {
		http.Error(w, "bad id", 400)
		return
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE orders
		 SET status='updated',
		     amount = amount + 1,
		     updated_at = now()
		 WHERE id = $1`, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		http.Error(w, "not found", 404)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) ordersDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := parseIntQuery(r, "id")
	if !ok {
		http.Error(w, "bad id", 400)
		return
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `DELETE FROM orders WHERE id=$1`, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		http.Error(w, "not found", 404)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Executors ---

func (s *Server) executorsCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	name := r.URL.Query().Get("name")
	role := r.URL.Query().Get("role")
	if name == "" {
		name = "executor"
	}
	if role == "" {
		role = "worker"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()

	var id int
	err = tx.QueryRowContext(ctx,
		`INSERT INTO executors(name, role, active) VALUES ($1, $2, true) RETURNING id`,
		name, role,
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

func (s *Server) executorsUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := parseIntQuery(r, "id")
	if !ok {
		http.Error(w, "bad id", 400)
		return
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE executors
		 SET active = NOT active,
		     updated_at = now()
		 WHERE id=$1`, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		http.Error(w, "not found", 404)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) executorsDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := parseIntQuery(r, "id")
	if !ok {
		http.Error(w, "bad id", 400)
		return
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `DELETE FROM executors WHERE id=$1`, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		http.Error(w, "not found", 404)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Links ---

func (s *Server) link(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	oid, ok1 := parseIntQuery(r, "order_id")
	eid, ok2 := parseIntQuery(r, "executor_id")
	if !ok1 || !ok2 {
		http.Error(w, "bad order_id/executor_id", 400)
		return
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()

	var tmp int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM orders WHERE id=$1`, oid).Scan(&tmp); err != nil {
		http.Error(w, "order not found", 404)
		return
	}
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM executors WHERE id=$1`, eid).Scan(&tmp); err != nil {
		http.Error(w, "executor not found", 404)
		return
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO order_executors(order_id, executor_id) VALUES ($1,$2)
		 ON CONFLICT DO NOTHING`, oid, eid)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) unlink(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	oid, ok1 := parseIntQuery(r, "order_id")
	eid, ok2 := parseIntQuery(r, "executor_id")
	if !ok1 || !ok2 {
		http.Error(w, "bad order_id/executor_id", 400)
		return
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`DELETE FROM order_executors WHERE order_id=$1 AND executor_id=$2`, oid, eid)
	if err != nil {
		http.Error(w, err.Error(), 500)
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

	// Orders
	mux.HandleFunc("/orders/create", s.ordersCreate)
	mux.HandleFunc("/orders/update", s.ordersUpdate)
	mux.HandleFunc("/orders/delete", s.ordersDelete)

	// Executors
	mux.HandleFunc("/executors/create", s.executorsCreate)
	mux.HandleFunc("/executors/update", s.executorsUpdate)
	mux.HandleFunc("/executors/delete", s.executorsDelete)

	// Links
	mux.HandleFunc("/links/link", s.link)
	mux.HandleFunc("/links/unlink", s.unlink)

	addr := ":8090"
	log.Println("repo listening on", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
