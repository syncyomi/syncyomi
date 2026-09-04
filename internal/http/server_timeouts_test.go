package http

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
)

func startTimeoutServer(t *testing.T, handler http.Handler) string {
	t.Helper()
	srv := newHTTPServer(handler)
	srv.ReadHeaderTimeout = 150 * time.Millisecond
	srv.ReadTimeout = 150 * time.Millisecond
	srv.WriteTimeout = 150 * time.Millisecond
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return ln.Addr().String()
}

func TestHTTPServer_TimeoutsSpareStreams(t *testing.T) {
	log := zerolog.Nop()
	r := chi.NewRouter()
	r.Use(LoggerMiddleware(&log))
	r.Get("/stream", func(w http.ResponseWriter, r *http.Request) {
		streamForever(w)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for i := 0; i < 5; i++ {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
			fmt.Fprint(w, "data: tick\n\n")
			w.(http.Flusher).Flush()
		}
	})
	r.Get("/slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(400 * time.Millisecond)
		_, _ = w.Write([]byte("late"))
	})
	addr := startTimeoutServer(t, r)

	resp, err := http.Get("http://" + addr + "/stream")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if ticks := strings.Count(string(body), "tick"); err != nil || ticks != 5 {
		t.Errorf("stream past the timeouts: %d ticks, %v", ticks, err)
	}

	resp, err = http.Get("http://" + addr + "/slow")
	if err == nil {
		body, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		if string(body) == "late" {
			t.Error("slow handler was not cut by the write timeout")
		}
	}
}

func TestHTTPServer_ReadHeaderTimeout(t *testing.T) {
	addr := startTimeoutServer(t, http.NotFoundHandler())
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := conn.Read(make([]byte, 1))
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Error("idle connection was answered instead of closed")
		}
	case <-ctx.Done():
		t.Error("idle connection kept open past the header timeout")
	}
}
