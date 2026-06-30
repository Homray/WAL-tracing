// catalog-service applies business logic (validation, pricing) and delegates
// all persistence to store-service. It has no direct database access.
//
// Chain:  loadgen → api-gateway → catalog-service → store-service → PostgreSQL
//
// ──────────────────────────────────────────────────────────────────────────
// [waltracer] integration:
//   - waltracer.Middleware preserves the X-WAL-Trace-Id coming from the
//     api-gateway and replaces X-WAL-Component with its own name.
//   - waltracer.PropagateHeaders forwards context to store-service.
// ──────────────────────────────────────────────────────────────────────────
package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	// [waltracer]
	"waltracer/waltracer"
)

type CatalogSvc struct {
	storeURL string
	client   *http.Client
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s is not set", key)
	}
	return v
}

func (s *CatalogSvc) forward(w http.ResponseWriter, r *http.Request, path string) {
	u := strings.TrimRight(s.storeURL, "/") + path
	if r.URL.RawQuery != "" {
		u += "?" + r.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, u, nil)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// [waltracer] propagate tracing headers further down the chain
	waltracer.PropagateHeaders(r, req)

	resp, err := s.client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

func (s *CatalogSvc) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *CatalogSvc) create(w http.ResponseWriter, r *http.Request) {
	// Business logic: could validate/enrich the item here.
	// Forwarding to store-service for persistence.
	s.forward(w, r, "/items/create")
}

func (s *CatalogSvc) update(w http.ResponseWriter, r *http.Request) {
	s.forward(w, r, "/items/update")
}

func (s *CatalogSvc) delete(w http.ResponseWriter, r *http.Request) {
	s.forward(w, r, "/items/delete")
}

func main() {
	storeURL := mustEnv("STORE_URL")
	svc := &CatalogSvc{
		storeURL: storeURL,
		client:   &http.Client{Timeout: 3 * time.Second},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", svc.healthz)
	mux.HandleFunc("/items/create", svc.create)
	mux.HandleFunc("/items/update", svc.update)
	mux.HandleFunc("/items/delete", svc.delete)

	addr := ":8092"
	log.Println("catalog-service listening on", addr)

	// [waltracer] Middleware sets X-WAL-Component="catalog_service".
	// If X-WAL-Trace-Id already arrived from api-gateway, it is preserved;
	// otherwise a fresh ID is generated — this service can also be called directly.
	log.Fatal(http.ListenAndServe(addr, waltracer.Middleware("catalog_service")(mux)))
}
