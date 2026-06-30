// api-gateway is the entry point that routes requests to catalog-service.
// It has no database access of its own.
//
// Chain:  loadgen → api-gateway → catalog-service → store-service → PostgreSQL
//
// ──────────────────────────────────────────────────────────────────────────
// [waltracer] integration:
//   - waltracer.Middleware sets X-WAL-Component and generates X-WAL-Trace-Id.
//   - waltracer.PropagateHeaders forwards tracing headers to catalog-service.
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

type Gateway struct {
	catalogURL string
	client     *http.Client
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s is not set", key)
	}
	return v
}

func (g *Gateway) forward(w http.ResponseWriter, r *http.Request, path string) {
	u := strings.TrimRight(g.catalogURL, "/") + path
	if r.URL.RawQuery != "" {
		u += "?" + r.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, u, nil)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// [waltracer] propagate tracing context to the downstream service
	waltracer.PropagateHeaders(r, req)

	resp, err := g.client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

func (g *Gateway) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// Route /items/create → catalog-service /items/create
func (g *Gateway) itemsCreate(w http.ResponseWriter, r *http.Request) {
	// [waltracer] annotate with operation metadata before forwarding
	r.Header.Set(waltracer.HeaderOperation, "create_item")
	r.Header.Set(waltracer.HeaderEntity, "item")
	g.forward(w, r, "/items/create")
}

// Route /items/update → catalog-service /items/update
func (g *Gateway) itemsUpdate(w http.ResponseWriter, r *http.Request) {
	r.Header.Set(waltracer.HeaderOperation, "update_item")   // [waltracer]
	r.Header.Set(waltracer.HeaderEntity, r.URL.Query().Get("id"))
	g.forward(w, r, "/items/update")
}

// Route /items/delete → catalog-service /items/delete
func (g *Gateway) itemsDelete(w http.ResponseWriter, r *http.Request) {
	r.Header.Set(waltracer.HeaderOperation, "delete_item")   // [waltracer]
	r.Header.Set(waltracer.HeaderEntity, r.URL.Query().Get("id"))
	g.forward(w, r, "/items/delete")
}

func main() {
	catalogURL := mustEnv("CATALOG_URL")
	gw := &Gateway{
		catalogURL: catalogURL,
		client:     &http.Client{Timeout: 3 * time.Second},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", gw.healthz)
	mux.HandleFunc("/items/create", gw.itemsCreate)
	mux.HandleFunc("/items/update", gw.itemsUpdate)
	mux.HandleFunc("/items/delete", gw.itemsDelete)

	addr := ":8091"
	log.Println("api-gateway listening on", addr)

	// [waltracer] inject X-WAL-Component="api_gateway" and X-WAL-Trace-Id for every request
	log.Fatal(http.ListenAndServe(addr, waltracer.Middleware("api_gateway")(mux)))
}
