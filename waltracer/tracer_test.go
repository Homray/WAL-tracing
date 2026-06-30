package waltracer_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"waltracer/waltracer"
)

func TestMiddlewareSetsComponent(t *testing.T) {
	called := false
	handler := waltracer.Middleware(
		"order_service",
	)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			if got := r.Header.Get(waltracer.HeaderComponent); got != "order_service" {
				t.Errorf("X-WAL-Component = %q, want %q", got, "order_service")
			}
		}),
	)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if !called {
		t.Fatal("inner handler was never called")
	}
}

func TestMiddlewarePreservesExistingComponent(t *testing.T) {
	handler := waltracer.Middleware(
		"local_service",
	)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get(waltracer.HeaderComponent); got != "upstream_service" {
				t.Errorf("X-WAL-Component = %q, want %q", got, "upstream_service")
			}
		}),
	)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(waltracer.HeaderComponent, "upstream_service")
	handler.ServeHTTP(httptest.NewRecorder(), req)
}

func TestMiddlewareSetsTraceID(t *testing.T) {
	handler := waltracer.Middleware(
		"svc",
	)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get(waltracer.HeaderTraceID) == "" {
				t.Error("X-WAL-Trace-Id should be set by Middleware")
			}
		}),
	)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
}

func TestMiddlewarePreservesExistingTraceID(t *testing.T) {
	const wantID = "fixed-trace-id"
	handler := waltracer.Middleware(
		"svc",
	)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get(waltracer.HeaderTraceID); got != wantID {
				t.Errorf("X-WAL-Trace-Id = %q, want %q", got, wantID)
			}
		}),
	)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(waltracer.HeaderTraceID, wantID)
	handler.ServeHTTP(httptest.NewRecorder(), req)
}

func TestMiddlewareGeneratesUniqueIDs(t *testing.T) {
	ids := make(map[string]bool)
	handler := waltracer.Middleware(
		"svc",
	)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(waltracer.HeaderTraceID)
			if ids[id] {
				t.Errorf("duplicate trace ID %q generated", id)
			}
			ids[id] = true
		}),
	)
	for i := 0; i < 500; i++ {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	}
}

func TestOpFromRequest(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(waltracer.HeaderComponent, "order_service")
	req.Header.Set(waltracer.HeaderOperation, "create_order")
	req.Header.Set(waltracer.HeaderEntity, "order:42")
	req.Header.Set(waltracer.HeaderTraceID, "abc123def456")

	op := waltracer.OpFromRequest(req)

	if op.Component != "order_service" {
		t.Errorf("Component = %q", op.Component)
	}
	if op.Name != "create_order" {
		t.Errorf("Name = %q", op.Name)
	}
	if op.Entity != "order:42" {
		t.Errorf("Entity = %q", op.Entity)
	}
	if op.TraceID != "abc123def456" {
		t.Errorf("TraceID = %q", op.TraceID)
	}
}

func TestOpFromRequestEmpty(t *testing.T) {
	op := waltracer.OpFromRequest(httptest.NewRequest("GET", "/", nil))
	if op.Component != "" || op.Name != "" || op.Entity != "" || op.TraceID != "" {
		t.Errorf("unexpected non-empty Op from empty request: %+v", op)
	}
}

func TestPropagateHeaders(t *testing.T) {
	src := httptest.NewRequest("GET", "/", nil)
	src.Header.Set(waltracer.HeaderComponent, "svc_a")
	src.Header.Set(waltracer.HeaderOperation, "do_thing")
	src.Header.Set(waltracer.HeaderEntity, "thing:1")
	src.Header.Set(waltracer.HeaderTraceID, "trace-xyz-789")

	dst, _ := http.NewRequest("POST", "http://svc_b/api", nil)
	waltracer.PropagateHeaders(src, dst)

	cases := []struct{ header, want string }{
		{waltracer.HeaderComponent, "svc_a"},
		{waltracer.HeaderOperation, "do_thing"},
		{waltracer.HeaderEntity, "thing:1"},
		{waltracer.HeaderTraceID, "trace-xyz-789"},
	}
	for _, c := range cases {
		if got := dst.Header.Get(c.header); got != c.want {
			t.Errorf("propagated %s = %q, want %q", c.header, got, c.want)
		}
	}
}

func TestPropagateHeadersSkipsEmpty(t *testing.T) {
	src := httptest.NewRequest("GET", "/", nil)
	src.Header.Set(waltracer.HeaderComponent, "svc_a")

	dst, _ := http.NewRequest("POST", "http://svc_b/api", nil)
	dst.Header.Set(waltracer.HeaderOperation, "dst_op")

	waltracer.PropagateHeaders(src, dst)

	if got := dst.Header.Get(waltracer.HeaderOperation); got != "dst_op" {
		t.Errorf("PropagateHeaders overwrote dst header: got %q", got)
	}
}

func TestMsgPrefix(t *testing.T) {
	if waltracer.MsgPrefix == "" {
		t.Error("MsgPrefix must not be empty")
	}
}
