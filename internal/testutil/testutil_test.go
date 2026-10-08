package testutil

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func post(t *testing.T, url string, body string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Test", "yes")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { response.Body.Close() })
	return response
}

func TestServerRepliesInOrderAndRecordsRequests(t *testing.T) {
	s := NewServer(t,
		Reply{Body: map[string]any{"result": "first"}},
		Reply{Status: http.StatusTeapot, Body: map[string]any{"result": "second"}},
	)

	first := post(t, s.URL+"/one", `{"order_id":"ORDER_1"}`)
	if first.StatusCode != http.StatusOK || first.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("first reply: status %d, content type %q", first.StatusCode, first.Header.Get("Content-Type"))
	}
	var decoded map[string]any
	if err := json.NewDecoder(first.Body).Decode(&decoded); err != nil || decoded["result"] != "first" {
		t.Fatalf("first reply body = %v (%v), want result first", decoded, err)
	}

	second := post(t, s.URL+"/two", `{"order_id":"ORDER_2"}`)
	if second.StatusCode != http.StatusTeapot {
		t.Fatalf("second reply status = %d, want 418", second.StatusCode)
	}

	exhausted := post(t, s.URL+"/three", `{}`)
	if exhausted.StatusCode != http.StatusInternalServerError {
		t.Fatalf("exhausted reply status = %d, want 500", exhausted.StatusCode)
	}

	if len(s.Requests) != 3 {
		t.Fatalf("recorded %d requests, want 3", len(s.Requests))
	}
	last := s.Last(t)
	if last.Method != http.MethodPost || last.Path != "/three" || last.Header.Get("X-Test") != "yes" {
		t.Fatalf("Last = %+v, want POST /three with the X-Test header", last)
	}
	if s.Requests[0].JSON(t)["order_id"] != "ORDER_1" {
		t.Fatalf("first recorded body = %s, want order ORDER_1", s.Requests[0].Body)
	}
}

func TestLastFailsWithoutRequests(t *testing.T) {
	s := NewServer(t)
	probe := &testing.T{}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Last(probe)
	}()
	<-done

	if !probe.Failed() {
		t.Fatal("Last on an idle server should fail the test")
	}
}

func TestRecordedJSONFailsOnInvalidBodies(t *testing.T) {
	probe := &testing.T{}

	done := make(chan struct{})
	go func() {
		defer close(done)
		Recorded{Body: []byte("not json")}.JSON(probe)
	}()
	<-done

	if !probe.Failed() {
		t.Fatal("JSON on an invalid body should fail the test")
	}
}

func TestFixtureReadsSharedTestVectors(t *testing.T) {
	fixture := Fixture(t, "kbz_pay/sign_string.json")
	if len(fixture) == 0 {
		t.Fatal("fixture decoded to an empty object")
	}

	for _, value := range fixture {
		if _, isFloat := value.(float64); isFloat {
			t.Fatal("Fixture must keep numbers as json.Number, got float64")
		}
	}
}

func TestFixtureFailsForMissingFiles(t *testing.T) {
	probe := &testing.T{}

	done := make(chan struct{})
	go func() {
		defer close(done)
		Fixture(probe, "missing/file.json")
	}()
	<-done

	if !probe.Failed() {
		t.Fatal("Fixture on a missing file should fail the test")
	}
}
