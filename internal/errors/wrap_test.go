package errors

import (
	"errors"
	"testing"
)

func TestErrorWrapper(t *testing.T) {
	wrapper := NewWrapper("course", "search_course")

	t.Run("Wrap returns nil for nil error", func(t *testing.T) {
		result := wrapper.Wrap(nil, "課程搜尋失敗")
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("Wrap creates WrappedError", func(t *testing.T) {
		baseErr := errors.New("database connection failed")
		wrapped := wrapper.Wrap(baseErr, "課程搜尋失敗")

		if wrapped == nil {
			t.Fatal("expected non-nil wrapped error")
		}

		wrappedErr, ok := wrapped.(*WrappedError)
		if !ok {
			t.Fatal("expected WrappedError type")
		}

		if wrappedErr.Module != "course" {
			t.Errorf("expected module 'course', got '%s'", wrappedErr.Module)
		}

		if wrappedErr.Operation != "search_course" {
			t.Errorf("expected operation 'search_course', got '%s'", wrappedErr.Operation)
		}

		if wrappedErr.UserMessage != "課程搜尋失敗" {
			t.Errorf("expected user message '課程搜尋失敗', got '%s'", wrappedErr.UserMessage)
		}

		if !errors.Is(wrapped, baseErr) {
			t.Error("wrapped error should unwrap to base error")
		}
	})

	t.Run("Wrapf formats message", func(t *testing.T) {
		baseErr := errors.New("not found")
		wrapped := wrapper.Wrapf(baseErr, "找不到課程：%s", "微積分")

		wrappedErr := wrapped.(*WrappedError)
		expected := "找不到課程：微積分"
		if wrappedErr.UserMessage != expected {
			t.Errorf("expected '%s', got '%s'", expected, wrappedErr.UserMessage)
		}
	})
}

func TestGetUserMessage(t *testing.T) {
	t.Run("returns empty string for nil", func(t *testing.T) {
		result := GetUserMessage(nil)
		if result != "" {
			t.Errorf("expected empty string, got '%s'", result)
		}
	})

	t.Run("returns user message from WrappedError", func(t *testing.T) {
		wrapped := &WrappedError{
			Operation:   "test",
			Module:      "test",
			Cause:       errors.New("base error"),
			UserMessage: "user friendly message",
		}

		result := GetUserMessage(wrapped)
		if result != "user friendly message" {
			t.Errorf("expected 'user friendly message', got '%s'", result)
		}
	})

	t.Run("returns error string for non-WrappedError", func(t *testing.T) {
		err := errors.New("plain error")
		result := GetUserMessage(err)
		if result != "plain error" {
			t.Errorf("expected 'plain error', got '%s'", result)
		}
	})
}

func TestWrappedError_Error(t *testing.T) {
	wrapped := &WrappedError{
		Operation:   "search",
		Module:      "course",
		Cause:       errors.New("db error"),
		UserMessage: "搜尋失敗",
	}

	errMsg := wrapped.Error()
	expected := "[course:search] 搜尋失敗: db error"
	if errMsg != expected {
		t.Errorf("expected '%s', got '%s'", expected, errMsg)
	}
}
