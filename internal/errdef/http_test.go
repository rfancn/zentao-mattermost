package errdef

import (
	"strings"
	"testing"
)

func TestStatusError(t *testing.T) {
	err := &StatusError{StatusCode: 404, Body: strings.Repeat("x", 600)}

	if !IsStatus(err, 404) {
		t.Error("IsStatus(404) = false")
	}
	if IsStatus(err, 500) {
		t.Error("IsStatus(500) = true")
	}
	if got := err.Error(); len(got) != len("HTTP 404: ")+500 {
		t.Errorf("Error() 长度 = %d, 响应体未截断", len(got))
	}
}

func TestIsStatusNil(t *testing.T) {
	if IsStatus(nil, 404) {
		t.Error("IsStatus(nil) = true")
	}
}
