package waltracer

import (
	"net/http"
)

// HTTP-заголовки для передачи контекста трассировки между сервисами.
const (
	HeaderComponent = "X-WAL-Component"
	HeaderOperation = "X-WAL-Operation"
	HeaderEntity    = "X-WAL-Entity"
	HeaderTraceID   = "X-WAL-Trace-Id"
)

// Middleware добавляет недостающие заголовки:
//   - X-WAL-Component
//   - X-WAL-Trace-Id
//
// Если TraceID уже существует, он сохраняется.
func Middleware(component string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get(HeaderComponent) == "" {
				r.Header.Set(HeaderComponent, component)
			}
			if r.Header.Get(HeaderTraceID) == "" {
				r.Header.Set(HeaderTraceID, newTraceID())
			}
			next.ServeHTTP(w, r)
		})
	}
}

// PropagateHeaders копирует контекст трассировки
// из входящего HTTP-запроса в исходящий.
func PropagateHeaders(src, dst *http.Request) {
	for _, h := range []string{HeaderComponent, HeaderOperation, HeaderEntity, HeaderTraceID} {
		if v := src.Header.Get(h); v != "" {
			dst.Header.Set(h, v)
		}
	}
}

// OpFromRequest восстанавливает Op из HTTP-заголовков.
func OpFromRequest(r *http.Request) Op {
	return Op{
		Component: r.Header.Get(HeaderComponent),
		Name:      r.Header.Get(HeaderOperation),
		Entity:    r.Header.Get(HeaderEntity),
		TraceID:   r.Header.Get(HeaderTraceID),
	}
}
