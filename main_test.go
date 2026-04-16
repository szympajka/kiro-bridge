package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBridgeHolderGetReturnsNilBeforeSet(t *testing.T) {
	h := &bridgeHolder{}
	if h.Get() != nil {
		t.Fatal("expected nil before Set")
	}
}

func TestBridgeHolderSetThenGet(t *testing.T) {
	h := &bridgeHolder{}
	mock := &mockBridge{}
	h.Set(mock)
	if h.Get() != mock {
		t.Fatal("expected mock bridge after Set")
	}
}

func TestConnectWithBackoffSucceedsImmediately(t *testing.T) {
	orig := newBridgeFunc
	defer func() { newBridgeFunc = orig }()

	mock := &mockBridge{}
	newBridgeFunc = func(cfg BridgeConfig) (Bridge, error) {
		return mock, nil
	}

	holder := &bridgeHolder{}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		connectWithBackoff(BridgeConfig{}, holder, stop)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("connectWithBackoff did not return")
	}

	if holder.Get() != mock {
		t.Fatal("expected bridge to be set")
	}
}

func TestConnectWithBackoffRetriesThenSucceeds(t *testing.T) {
	orig := newBridgeFunc
	defer func() { newBridgeFunc = orig }()

	calls := 0
	mock := &mockBridge{}
	newBridgeFunc = func(cfg BridgeConfig) (Bridge, error) {
		calls++
		if calls < 3 {
			return nil, http.ErrServerClosed // any error
		}
		return mock, nil
	}

	holder := &bridgeHolder{}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		connectWithBackoff(BridgeConfig{}, holder, stop)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("connectWithBackoff did not return")
	}

	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
	if holder.Get() != mock {
		t.Fatal("expected bridge to be set after retries")
	}
}

func TestConnectWithBackoffStopsOnSignal(t *testing.T) {
	orig := newBridgeFunc
	defer func() { newBridgeFunc = orig }()

	newBridgeFunc = func(cfg BridgeConfig) (Bridge, error) {
		return nil, http.ErrServerClosed
	}

	holder := &bridgeHolder{}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		connectWithBackoff(BridgeConfig{}, holder, stop)
		close(done)
	}()

	// Let it fail at least once, then stop
	time.Sleep(100 * time.Millisecond)
	close(stop)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("connectWithBackoff did not stop")
	}

	if holder.Get() != nil {
		t.Fatal("expected bridge to remain nil after stop")
	}
}

func TestHandlerReturns503WhenBridgeNil(t *testing.T) {
	holder := &bridgeHolder{}

	handler := func(w http.ResponseWriter, r *http.Request) {
		b := holder.Get()
		if b == nil {
			http.Error(w, "bridge not ready", http.StatusServiceUnavailable)
			return
		}
		handleChatCompletions(b)(w, r)
	}

	body := `{"model":"kiro","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
	if !strings.Contains(w.Body.String(), "bridge not ready") {
		t.Errorf("body = %q, want 'bridge not ready'", w.Body.String())
	}
}
