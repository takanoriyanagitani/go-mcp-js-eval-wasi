package jseval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

type JsEvalToolInput struct {
	Code string `json:"code"`
}

type JsEvalResultDto struct {
	Result interface{} `json:"result"`
	Error  *ErrorDto   `json:"error,omitempty"`
}

type ErrorDto struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Evaluator is the function type that will execute the WASM module.
type Evaluator func(context.Context, string) JsEvalResultDto

// NewEvaluator sets up wazero runtime and returns an Evaluator function.
// It takes the WASM binary directly to be unit test friendly.
func NewEvaluator(ctx context.Context, wasmBinary []byte, memoryLimitPages uint32) (Evaluator, func() error, error) {
	rConfig := wazero.NewRuntimeConfig().WithCloseOnContextDone(true).WithMemoryLimitPages(memoryLimitPages)
	r := wazero.NewRuntimeWithConfig(ctx, rConfig)
	cleanup := func() error { return r.Close(context.Background()) }

	_, err := wasi_snapshot_preview1.Instantiate(ctx, r)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to instantiate wasi_snapshot_preview1: %w", err)
	}

	compiled, err := r.CompileModule(ctx, wasmBinary)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to compile WASM module: %w", err)
	}

	log.Printf("WASM module compiled successfully.")

	evalFn := func(evalCtx context.Context, jsCode string) JsEvalResultDto {
		var stdoutBuf, stderrBuf bytes.Buffer
		moduleConfig := wazero.NewModuleConfig().
			WithSysWalltime().
			WithSysNanotime().
			WithSysNanosleep().
			WithStdin(strings.NewReader(jsCode)).
			WithStdout(&stdoutBuf).
			WithStderr(&stderrBuf)

		instance, e := r.InstantiateModule(evalCtx, compiled, moduleConfig)
		if instance != nil {
			defer func() { _ = instance.Close(evalCtx) }()
		}

		if e != nil {
			var exitErr *sys.ExitError
			if errors.As(e, &exitErr) {
				errorMsg := stderrBuf.String()
				log.Printf("WASM execution failed with exit code %d: %s", exitErr.ExitCode(), errorMsg)
				return JsEvalResultDto{Error: &ErrorDto{Code: int(exitErr.ExitCode()), Message: errorMsg}}
			}
			log.Printf("Failed to instantiate WASM module: %v", e)
			return JsEvalResultDto{Error: &ErrorDto{Code: -1, Message: fmt.Sprintf("WASM execution failed: %v", e)}}
		}

		var rawJsonOutput interface{}
		outputBytes := stdoutBuf.Bytes()
		if err := json.Unmarshal(outputBytes, &rawJsonOutput); err != nil {
			log.Printf("Failed to parse raw JSON from WASM stdout: %v. Raw output: %s", err, string(outputBytes))
			return JsEvalResultDto{Error: &ErrorDto{Code: -1, Message: "Failed to parse successful WASM output as JSON"}}
		}

		return JsEvalResultDto{Result: rawJsonOutput, Error: nil}
	}

	return evalFn, cleanup, nil
}

// LoadWasmBinary reads the WASM file from the given path with a size limit.
const bytesInMiB = 1024 * 1024

func LoadWasmBinary(wasmFilePath string, maxWasmSize uint) ([]byte, error) {
	f, err := os.Open(wasmFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open WASM file from %s: %w", wasmFilePath, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Printf("warning: failed to close wasm file %s: %v", wasmFilePath, err)
		}
	}()

	fileInfo, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get WASM file info from %s: %w", wasmFilePath, err)
	}

	maxBytes := int64(maxWasmSize) * bytesInMiB
	if fileInfo.Size() > maxBytes {
		return nil, fmt.Errorf("WASM file %s is too large (%d bytes), exceeding max size of %d MiB", wasmFilePath, fileInfo.Size(), maxWasmSize)
	}

	wasmBinary, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read WASM file from %s: %w", wasmFilePath, err)
	}

	return wasmBinary, nil
}
