// Package testutil holds helpers shared by the gateway tests.
package testutil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// Recorded is a request captured by a Server.
type Recorded struct {
	Method string
	Path   string
	Header http.Header
	Body   []byte
}

// JSON decodes the recorded body.
func (r Recorded) JSON(t *testing.T) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(r.Body, &decoded); err != nil {
		t.Fatalf("decode request body %q: %v", r.Body, err)
	}
	return decoded
}

// Reply is a canned response.
type Reply struct {
	Status int
	Body   any
}

// Server replies with canned responses in order and records every request.
type Server struct {
	*httptest.Server
	mu       sync.Mutex
	replies  []Reply
	Requests []Recorded
}

// NewServer starts a Server that answers with replies in order.
func NewServer(t *testing.T, replies ...Reply) *Server {
	t.Helper()
	s := &Server{replies: replies}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.Requests = append(s.Requests, Recorded{Method: r.Method, Path: r.URL.Path, Header: r.Header.Clone(), Body: body})
		var reply Reply
		if len(s.replies) > 0 {
			reply, s.replies = s.replies[0], s.replies[1:]
		} else {
			reply = Reply{Status: http.StatusInternalServerError, Body: map[string]any{"message": "no canned reply"}}
		}
		s.mu.Unlock()

		status := reply.Status
		if status == 0 {
			status = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(reply.Body)
	}))
	t.Cleanup(s.Close)
	return s
}

// Last returns the most recent request.
func (s *Server) Last(t *testing.T) Recorded {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Requests) == 0 {
		t.Fatal("no request was sent")
	}
	return s.Requests[len(s.Requests)-1]
}

// Fixture decodes testdata/<path> from the module root.
func Fixture(t *testing.T, path string) map[string]any {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "testdata", path))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var decoded map[string]any
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return decoded
}
