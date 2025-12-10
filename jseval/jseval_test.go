package jseval

import (
	"context"
	"strings"
	"testing"
)

func TestNewEvaluator(t *testing.T) {
	// Dummy WASM bytecode for an empty module: `(module)`
	dummyWasm := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}

	memoryLimitPages := uint32(1) // 64 KiB

	t.Run("SuccessfulCreation", func(t *testing.T) {
		ctx := context.Background()
		evaluator, cleanup, err := NewEvaluator(ctx, dummyWasm, memoryLimitPages)

		if err != nil {
			t.Fatalf("NewEvaluator() returned an unexpected error: %v", err)
		}
		if evaluator == nil {
			t.Fatal("NewEvaluator() returned a nil evaluator")
		}
		if cleanup == nil {
			t.Fatal("NewEvaluator() returned a nil cleanup function")
		}

		if err := cleanup(); err != nil {
			t.Errorf("cleanup() returned an unexpected error: %v", err)
		}
	})

	t.Run("ExecutionFailsAsExpectedForEmptyModule", func(t *testing.T) {
		ctx := context.Background()
		evaluator, cleanup, err := NewEvaluator(ctx, dummyWasm, memoryLimitPages)
		if err != nil {
			t.Fatalf("Test setup failed: NewEvaluator() returned an unexpected error: %v", err)
		}
		defer func() {
			if err := cleanup(); err != nil {
				t.Errorf("cleanup() failed: %v", err)
			}
		}()

		// This will "succeed" from wazero's perspective (exit 0) but produce no output,
		// causing a JSON parsing error in our wrapper.
		result := evaluator(ctx, "1+1")

		if result.Error == nil {
			t.Fatal("evaluator() was expected to return an error, but it did not")
		}

		expectedErrMsg := "Failed to parse successful WASM output as JSON"
		if !strings.Contains(result.Error.Message, expectedErrMsg) {
			t.Errorf("evaluator() returned an unexpected error message.\nGot: %s\nWant contains: %s", result.Error.Message, expectedErrMsg)
		}

		t.Logf("Received expected error: %s", result.Error.Message)
	})
}
