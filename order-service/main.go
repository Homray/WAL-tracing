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
	a.forwardPost(w, r, "/orders/create")
}

func (a *App) update(w http.ResponseWriter, r *http.Request) {
	if id, ok := parseID(r); !ok || id <= 0 {
		http.Error(w, "bad id", 400)
		return
	}
	a.forwardPost(w, r, "/orders/update")
}

func (a *App) del(w http.ResponseWriter, r *http.Request) {
	if id, ok := parseID(r); !ok || id <= 0 {
		http.Error(w, "bad id", 400)
		return
	}
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
	log.Fatal(http.ListenAndServe(addr, mux))
}
