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

func parseInt(r *http.Request, key string) (int, bool) {
	s := r.URL.Query().Get(key)
	x, err := strconv.Atoi(s)
	return x, err == nil && x > 0
}

func (a *App) link(w http.ResponseWriter, r *http.Request) {
	oid, ok1 := parseInt(r, "order_id")
	eid, ok2 := parseInt(r, "executor_id")
	if !ok1 || !ok2 {
		http.Error(w, "bad order_id/executor_id", 400)
		return
	}

	r.Header.Set("X-Operation", "link_executor")
	r.Header.Set("X-Entity", "link:order:"+strconv.Itoa(oid)+":executor:"+strconv.Itoa(eid))
	a.forwardPost(w, r, "/links/link")
}

func (a *App) unlink(w http.ResponseWriter, r *http.Request) {
	oid, ok1 := parseInt(r, "order_id")
	eid, ok2 := parseInt(r, "executor_id")
	if !ok1 || !ok2 {
		http.Error(w, "bad order_id/executor_id", 400)
		return
	}

	r.Header.Set("X-Operation", "unlink_executor")
	r.Header.Set("X-Entity", "link:order:"+strconv.Itoa(oid)+":executor:"+strconv.Itoa(eid))
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
	log.Fatal(http.ListenAndServe(addr, ensureHeaders("logistics_service", mux)))
}
