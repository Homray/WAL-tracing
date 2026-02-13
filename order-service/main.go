package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type App struct {
	repoURL string
	client  *http.Client
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s is not set", key)
	}
	return v
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func ensureHeaders(component string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Component") == "" {
			r.Header.Set("X-Component", component)
		}
		if r.Header.Get("X-Trace-Id") == "" {
			r.Header.Set("X-Trace-Id", strconv.FormatInt(time.Now().UnixNano(), 16))
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) forwardPost(w http.ResponseWriter, r *http.Request, path string) {
	u := strings.TrimRight(a.repoURL, "/") + path
	if r.URL.RawQuery != "" {
		u += "?" + r.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, u, nil)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	for _, h := range []string{"X-Component", "X-Operation", "X-Entity", "X-Trace-Id"} {
		if v := r.Header.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}

	resp, err := a.client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

func (a *App) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func parseID(r *http.Request) (int, bool) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	return id, err == nil && id > 0
}

func (a *App) create(w http.ResponseWriter, r *http.Request) {
	r.Header.Set("X-Operation", "create_order")
	r.Header.Set("X-Entity", "order")
	a.forwardPost(w, r, "/orders/create")
}

func (a *App) update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		http.Error(w, "bad id", 400)
		return
	}
	r.Header.Set("X-Operation", "update_order")
	r.Header.Set("X-Entity", "order:"+strconv.Itoa(id))
	a.forwardPost(w, r, "/orders/update")
}

func (a *App) del(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		http.Error(w, "bad id", 400)
		return
	}
	r.Header.Set("X-Operation", "delete_order")
	r.Header.Set("X-Entity", "order:"+strconv.Itoa(id))
	a.forwardPost(w, r, "/orders/delete")
}

func main() {
	repoURL := mustEnv("REPO_URL")
	app := &App{
		repoURL: repoURL,
		client: &http.Client{
			Timeout: 2 * time.Second,
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", app.healthz)
	mux.HandleFunc("/create", app.create)
	mux.HandleFunc("/update", app.update)
	mux.HandleFunc("/delete", app.del)

	addr := ":8081"
	log.Println("order-service listening on", addr)
	log.Fatal(http.ListenAndServe(addr, ensureHeaders("order_service", mux)))
}
