package pipeline

import (
	"testing"
)

func TestIsRetriableJSONError_True(t *testing.T) {
	tests := []string{
		"invalid JSON: invalid character 'B' looking for beginning of value",
		"invalid JSON: invalid character 'I' looking for beginning of value",
		"invalid JSON: unexpected end of JSON input",
	}
	for _, msg := range tests {
		if !isRetriableJSONError(msg) {
			t.Errorf("isRetriableJSONError(%q) = false, want true", msg)
		}
	}
}

func TestIsRetriableJSONError_False(t *testing.T) {
	tests := []string{
		"invoking classifier: claude timed out after 2m0s",
		"invoking classifier: claude exited with code 1: ",
		"session not found in store",
		"reading evaluator prompt: file not found",
	}
	for _, msg := range tests {
		if isRetriableJSONError(msg) {
			t.Errorf("isRetriableJSONError(%q) = true, want false", msg)
		}
	}
}
