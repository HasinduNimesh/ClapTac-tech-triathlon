package httpx

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/telemetry"
)

type contextKey string

const correlationKey contextKey = "waypoint.correlation_id"

type Options struct {
	Service string
	Logger  *slog.Logger
	Ready   func(http.ResponseWriter, *http.Request)
}

const (
	maxRequestBodyBytes = 8 << 20
	maxHeaderBytes      = 1 << 20
)

func NewRouter(opts Options) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(correlationMiddleware)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(requestBodyLimit(maxRequestBodyBytes))
	r.Use(securityHeaders)
	r.Use(accessLog(opts.Logger, opts.Service))
	r.Use(metricsMiddleware(opts.Service))
	r.Use(tracingMiddleware(opts.Service))

	r.Get("/health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "live", "service": opts.Service})
	})
	ready := opts.Ready
	if ready == nil {
		ready = func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "ready", "service": opts.Service})
		}
	}
	r.Get("/health/ready", ready)
	r.Handle("/metrics", promhttp.Handler())
	return r
}

func ListenAndServe(ctx context.Context, addr string, handler http.Handler, logger *slog.Logger) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    maxHeaderBytes,
	}
	errCh := make(chan error, 1)
	go func() {
		logger.Info("http_listen", "addr", addr)
		errCh <- srv.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func WriteJSON(w http.ResponseWriter, status int, body any) {
	writeJSON(w, status, body)
}

func CorrelationIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(correlationKey).(string); ok {
		return v
	}
	return ""
}

func correlationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Correlation-ID")
		if id == "" {
			id = middleware.GetReqID(r.Context())
		}
		if id == "" {
			id = strconv.FormatInt(time.Now().UnixNano(), 36)
		}
		w.Header().Set("X-Correlation-ID", id)
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), correlationKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func accessLog(logger *slog.Logger, service string) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			responseStatus := ww.Status()
			if responseStatus < 100 {
				responseStatus = http.StatusOK
			}
			path := routePattern(r)
			logger.Info("http_request",
				"service", service,
				"method", r.Method,
				"path", path,
				"status", responseStatus,
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds(),
				"correlation_id", CorrelationIDFrom(r.Context()),
			)
		})
	}
}

func metricsMiddleware(service string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/metrics" || r.URL.Path == "/health/live" || r.URL.Path == "/health/ready" {
				next.ServeHTTP(w, r)
				return
			}
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			responseStatus := ww.Status()
			if responseStatus < 100 {
				responseStatus = http.StatusOK
			}
			status := strconv.Itoa(responseStatus)
			path := routePattern(r)
			telemetry.HTTPRequests.WithLabelValues(service, r.Method, path, status).Inc()
			telemetry.HTTPRequestDuration.WithLabelValues(service, r.Method, path).Observe(time.Since(start).Seconds())
			if responseStatus >= 400 {
				telemetry.HTTPErrors.WithLabelValues(service, r.Method, path, status).Inc()
			}
		})
	}
}

func tracingMiddleware(service string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, span := otel.Tracer("waypoint/http").Start(r.Context(), "HTTP "+r.Method,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					attribute.String("service.name", service),
					attribute.String("http.request.method", r.Method),
					attribute.String("waypoint.correlation_id", CorrelationIDFrom(r.Context())),
				),
			)
			defer span.End()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r.WithContext(ctx))
			status := ww.Status()
			if status < 100 {
				status = http.StatusOK
			}
			route := routePattern(r)
			span.SetName(r.Method + " " + route)
			span.SetAttributes(attribute.String("http.route", route), attribute.Int("http.response.status_code", status))
			if status >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, http.StatusText(status))
			}
		})
	}
}

func routePattern(r *http.Request) string {
	if pattern := chi.RouteContext(r.Context()).RoutePattern(); pattern != "" {
		return pattern
	}
	return "/__unmatched__"
}

func requestBodyLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
