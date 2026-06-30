// executor-service handles executor CRUD requests and proxies them to repo.
// It does not access PostgreSQL directly.
//
// [waltracer] integration points are marked with [waltracer] comments.
package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	// [waltracer]
	"waltracer/waltracer"
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
	// [waltracer]
	waltracer.PropagateHeaders(r, req)
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
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	return id, err == nil && id > 0
}

func (a *App) create(w http.ResponseWriter, r *http.Request) {
	r.Header.Set(waltracer.HeaderOperation, "create_executor") // [waltracer]
	r.Header.Set(waltracer.HeaderEntity, "executor")
	a.forwardPost(w, r, "/executors/create")
}

func (a *App) update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		http.Error(w, "bad id", 400)
		return
	}
	r.Header.Set(waltracer.HeaderOperation, "update_executor") // [waltracer]
	r.Header.Set(waltracer.HeaderEntity, "executor:"+strconv.Itoa(id))
	a.forwardPost(w, r, "/executors/update")
}

func (a *App) del(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		http.Error(w, "bad id", 400)
		return
	}
	r.Header.Set(waltracer.HeaderOperation, "delete_executor") // [waltracer]
	r.Header.Set(waltracer.HeaderEntity, "executor:"+strconv.Itoa(id))
	a.forwardPost(w, r, "/executors/delete")
}

func main() {
	repoURL := mustEnv("REPO_URL")
	app := &App{
		repoURL: repoURL,
		client:  &http.Client{Timeout: 2 * time.Second},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", app.healthz)
	mux.HandleFunc("/create", app.create)
	mux.HandleFunc("/update", app.update)
	mux.HandleFunc("/delete", app.del)

	addr := ":8082"
	log.Println("executor-service listening on", addr)
	// [waltracer]
	log.Fatal(http.ListenAndServe(addr, waltracer.Middleware("executor_service")(mux)))
}
