// review-service — CRUD отзывов пользователей о товарах.
// Имеет прямой доступ к PostgreSQL, ссылается на таблицы users и products.
//
// В отличие от примеров basic и chain, здесь review-service сам подключается
// к БД и вызывает waltracer.ApplyContext — точно так же, как user-service
// и product-service. Нет единого «репозитория» и нет цепочки прокси.
//
// ──────────────────────────────────────────────────────────────────────────
// [waltracer] интеграция:
//   - waltracer.Middleware генерирует X-WAL-Trace-Id для каждого запроса.
//   - waltracer.ApplyContext вызывается напрямую с Component="review_service".
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
	// Таблицы users и products создаются соответствующими сервисами,
	// review_service только создаёт таблицу reviews.
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS users (
  id         SERIAL PRIMARY KEY,
  name       TEXT NOT NULL,
  email      TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT now(),
  updated_at TIMESTAMP NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS products (
  id         SERIAL PRIMARY KEY,
  name       TEXT    NOT NULL,
  category   TEXT    NOT NULL,
  price      INT     NOT NULL DEFAULT 0,
  stock      INT     NOT NULL DEFAULT 0,
  updated_at TIMESTAMP NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS reviews (
  id         SERIAL PRIMARY KEY,
  user_id    INT  NOT NULL,
  product_id INT  NOT NULL,
  rating     INT  NOT NULL CHECK (rating BETWEEN 1 AND 5),
  body       TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT now()
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

	userID, err := strconv.Atoi(r.URL.Query().Get("user_id"))
	if err != nil || userID <= 0 {
		http.Error(w, "bad user_id", 400)
		return
	}
	productID, err := strconv.Atoi(r.URL.Query().Get("product_id"))
	if err != nil || productID <= 0 {
		http.Error(w, "bad product_id", 400)
		return
	}
	rating, err := strconv.Atoi(r.URL.Query().Get("rating"))
	if err != nil || rating < 1 || rating > 5 {
		rating = 5
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()

	// [waltracer] review_service владеет этой транзакцией и сам указывает свой компонент
	if err := waltracer.ApplyContext(ctx, tx, waltracer.Op{
		Component: "review_service",
		Name:      "create_review",
		Entity:    "review:user:" + strconv.Itoa(userID) + ":product:" + strconv.Itoa(productID),
		TraceID:   r.Header.Get(waltracer.HeaderTraceID),
	}); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	var id int
	err = tx.QueryRowContext(ctx,
		`INSERT INTO reviews(user_id, product_id, rating, body) VALUES ($1, $2, $3, 'auto review') RETURNING id`,
		userID, productID, rating,
	).Scan(&id)
	if err != nil {
		// Нарушение FK — пользователь или товар не существует
		http.Error(w, err.Error(), 422)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
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
		Component: "review_service",
		Name:      "delete_review",
		Entity:    "review:" + strconv.Itoa(id),
		TraceID:   r.Header.Get(waltracer.HeaderTraceID),
	}); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM reviews WHERE id=$1`, id)
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
	mux.HandleFunc("/reviews/create", s.create)
	mux.HandleFunc("/reviews/delete", s.delete)

	addr := ":8096"
	log.Println("review-service listening on", addr)

	// [waltracer]
	log.Fatal(http.ListenAndServe(addr, waltracer.Middleware("review_service")(mux)))
}
