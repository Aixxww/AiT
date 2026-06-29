package safe

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// ReadAllLimited
// ---------------------------------------------------------------------------

func TestReadAllLimited_SmallInput(t *testing.T) {
	data, err := ReadAllLimited(strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("got %q, want %q", data, "hello")
	}
}

func TestReadAllLimited_ExactLimit(t *testing.T) {
	input := strings.Repeat("a", 100)
	data, err := ReadAllLimited(strings.NewReader(input), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 100 {
		t.Fatalf("len = %d, want 100", len(data))
	}
}

func TestReadAllLimited_ExceedsLimit(t *testing.T) {
	input := strings.Repeat("x", 200)
	_, err := ReadAllLimited(strings.NewReader(input), 100)
	if err == nil {
		t.Fatal("expected error when exceeding limit")
	}
}

func TestReadAllLimited_DefaultLimit(t *testing.T) {
	// Should not error for small input with default limit
	data, err := ReadAllLimited(bytes.NewReader([]byte("small")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "small" {
		t.Fatalf("got %q", data)
	}
}

func TestReadAllLimited_ZeroLimit(t *testing.T) {
	// Zero limit should use default (MaxResponseBody)
	data, err := ReadAllLimited(strings.NewReader("test"), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "test" {
		t.Fatalf("got %q", data)
	}
}

func TestReadAllLimited_EmptyInput(t *testing.T) {
	data, err := ReadAllLimited(strings.NewReader(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("expected empty, got %d bytes", len(data))
	}
}

// ---------------------------------------------------------------------------
// Must
// ---------------------------------------------------------------------------

func TestMust_NoPanic(t *testing.T) {
	err := Must(func() {
		// no panic
	})
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestMust_WithPanic(t *testing.T) {
	err := Must(func() {
		panic("boom")
	})
	if err == nil {
		t.Fatal("expected error from panic")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error should contain panic message, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Go (panic recovery)
// ---------------------------------------------------------------------------

func TestGo_NoPanic(t *testing.T) {
	done := make(chan struct{})
	Go(func() {
		close(done)
	})
	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine did not complete in time")
	}
}

func TestGo_WithPanic(t *testing.T) {
	recovered := make(chan interface{}, 1)
	Go(func() {
		panic("test-panic")
	}, func(r interface{}) {
		recovered <- r
	})

	select {
	case r := <-recovered:
		if r != "test-panic" {
			t.Fatalf("recovered = %v, want %q", r, "test-panic")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("onPanic callback was not called")
	}
}

// ---------------------------------------------------------------------------
// GoNamed
// ---------------------------------------------------------------------------

func TestGoNamed_NoPanic(t *testing.T) {
	done := make(chan struct{})
	GoNamed("test-goroutine", func() {
		close(done)
	})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine did not complete")
	}
}

func TestGoNamed_WithPanic(t *testing.T) {
	recovered := make(chan interface{}, 1)
	GoNamed("named-panic", func() {
		panic("named-boom")
	}, func(r interface{}) {
		recovered <- r
	})

	select {
	case <-recovered:
		// The named panic handler was invoked (and the user callback too)
	case <-time.After(2 * time.Second):
		t.Fatal("onPanic callback not called")
	}
}

// ---------------------------------------------------------------------------
// MaxResponseBody constant
// ---------------------------------------------------------------------------

func TestMaxResponseBodyConstant(t *testing.T) {
	if MaxResponseBody != 10*1024*1024 {
		t.Fatalf("MaxResponseBody = %d, want 10MB", MaxResponseBody)
	}
}
