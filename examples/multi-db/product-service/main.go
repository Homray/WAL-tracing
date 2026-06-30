// product-service — CRUD товаров с прямым доступом к PostgreSQL.
//
// Каждый сервис в этом примере самостоятельно подключается к БД и
// вызывает waltracer.ApplyContext, явно указывая своё название.
//
// ──────────────────────────────────────────────────────────────────────────
// [waltracer] интеграция:
//   - waltracer.Middleware генерирует X-WAL-Trace-Id для каждого запроса.
//   - waltracer.ApplyContext вызывается напрямую с Component="product_service".
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
CREATE TABLE IF NOT EXISTS products (
  id         SERIAL PRIMARY KEY,
  name       TEXT    NOT NULL,
  category   TEXT    NOT NULL,
  price      INT     NOT NULL DEFAULT 0,
  stock      INT     NOT NULL DEFAULT 0,
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
	category := r.URL.Query().Get("category")
	if name == "" {
		name = "product"
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

	// [waltracer]
	if err := waltracer.ApplyContext(ctx, tx, waltracer.Op{
		Component: "product_service",
		Name:      "create_product",
		Entity:    "product",
		TraceID:   r.Header.Get(waltracer.HeaderTraceID),
	}); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	var id int
	err = tx.QueryRowContext(ctx,
		`INSERT INTO products(name, category, price, stock) VALUES ($1, $2, 1000, 50) RETURNING id`,
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
		Component: "product_service",
		Name:      "update_product",
		Entity:    "product:" + strconv.Itoa(id),
		TraceID:   r.Header.Get(waltracer.HeaderTraceID),
	}); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE products SET price=price+100, stock=GREATEST(stock-1,0), updated_at=now() WHERE id=$1`, id)
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
		Component: "product_service",
		Name:      "delete_product",
		Entity:    "product:" + strconv.Itoa(id),
		TraceID:   r.Header.Get(waltracer.HeaderTraceID),
	}); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM products WHERE id=$1`, id)
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
	mux.HandleFunc("/products/create", s.create)
	mux.HandleFunc("/products/update", s.update)
	mux.HandleFunc("/products/delete", s.delete)

	addr := ":8095"
	log.Println("product-service listening on", addr)

	// [waltracer]
	log.Fatal(http.ListenAndServe(addr, waltracer.Middleware("product_service")(mux)))
}
