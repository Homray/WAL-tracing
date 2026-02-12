package main

import (
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

func parseInt(r *http.Request, key string) (int, bool) {
	s := r.URL.Query().Get(key)
	x, err := strconv.Atoi(s)
	return x, err == nil && x > 0
}

func (a *App) link(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseInt(r, "order_id"); !ok {
		http.Error(w, "bad order_id", 400)
		return
	}
	if _, ok := parseInt(r, "executor_id"); !ok {
		http.Error(w, "bad executor_id", 400)
		return
	}
	a.forwardPost(w, r, "/links/link")
}

func (a *App) unlink(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseInt(r, "order_id"); !ok {
		http.Error(w, "bad order_id", 400)
		return
	}
	if _, ok := parseInt(r, "executor_id"); !ok {
		http.Error(w, "bad executor_id", 400)
		return
	}
	a.forwardPost(w, r, "/links/unlink")
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
	mux.HandleFunc("/link", app.link)
	mux.HandleFunc("/unlink", app.unlink)

	addr := ":8083"
	log.Println("logistics-service listening on", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
