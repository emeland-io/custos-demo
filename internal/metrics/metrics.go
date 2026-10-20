package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests, labeled by route and status code.",
	}, []string{"route", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "http_request_duration_seconds",
		Help: "HTTP request duration in seconds, labeled by route.",
	}, []string{"route"})

	httpInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "http_requests_in_flight",
		Help: "Number of HTTP requests currently being served.",
	})

	NotesCreatedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "notes_created_total",
		Help: "Total number of notes created.",
	})

	CommentsCreatedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "comments_created_total",
		Help: "Total number of comments created.",
	})

	CommentsDeletedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "comments_deleted_total",
		Help: "Total number of comments deleted.",
	})
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// routeLabel returns the mux path template for r (e.g. "/api/notes/{id}"),
// bounding metric cardinality to the number of registered routes. It
// falls back to "unmatched" when no mux route matched (e.g. a 404, or
// a request that never went through mux route matching at all, as in
// a standalone unit test of Middleware).
func routeLabel(r *http.Request) string {
	route := mux.CurrentRoute(r)
	if route == nil {
		return "unmatched"
	}
	tpl, err := route.GetPathTemplate()
	if err != nil {
		return "unmatched"
	}
	return tpl
}

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpInFlight.Inc()
		defer httpInFlight.Dec()

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)
		duration := time.Since(start).Seconds()

		route := routeLabel(r)
		httpRequestsTotal.WithLabelValues(route, strconv.Itoa(rec.status)).Inc()
		httpRequestDuration.WithLabelValues(route).Observe(duration)
	})
}

func Handler() http.Handler {
	return promhttp.Handler()
}
