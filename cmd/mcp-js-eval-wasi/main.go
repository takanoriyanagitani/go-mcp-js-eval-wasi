package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/takanoriyanagitani/go-mcp-js-eval-wasi/jseval"
)

const (
	defaultPort         = 12040
	readTimeoutSeconds  = 10
	writeTimeoutSeconds = 10
	maxHeaderExponent   = 20
	maxBodyBytes        = 1 * 1024 * 1024 // 1 MiB
	wasmPageSizeKiB     = 64
	kiBytesInMiByte     = 1024
	wasmPagesInMiB      = kiBytesInMiByte / wasmPageSizeKiB
)

var (
	port       = flag.Int("port", defaultPort, "port to listen")
	enginePath = flag.String(
		"path2engine",
		os.ExpandEnv("${HOME}/.cargo/bin/js-eval-boa.wasm"),
		"path to the WASM JavaScript engine",
	)
	mem         = flag.Uint("mem", 64, "WASM memory limit in MiB")
	timeout     = flag.Uint("timeout", 100, "WASM execution timeout in milliseconds")
	maxWasmSize = flag.Uint("max-wasm-size", 16, "Maximum WASM file size in MiB")
)

func main() {
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wasmBinary, err := jseval.LoadWasmBinary(*enginePath, *maxWasmSize)
	if err != nil {
		log.Fatalf("failed to load WASM binary: %v", err)
	}

	memoryLimitPages := uint32(*mem) * wasmPagesInMiB
	evaluator, cleanup, err := jseval.NewEvaluator(ctx, wasmBinary, memoryLimitPages)
	if err != nil {
		log.Fatalf("failed to create WASI JavaScript evaluator: %v", err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			log.Printf("failed to cleanup WASI evaluator: %v", err)
		}
	}()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "js-eval",
		Version: "v0.1.0",
		Title:   "JavaScript Evaluator",
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:         "eval-js",
		Title:        "Evaluate JavaScript",
		Description:  "Tool to evaluate JavaScript code, provided as a raw string inside an object.",
		InputSchema:  nil,
		OutputSchema: nil,
	}, func(toolCtx context.Context, req *mcp.CallToolRequest, input jseval.JsEvalToolInput) (
		*mcp.CallToolResult,
		jseval.JsEvalResultDto,
		error,
	) {
		timeoutCtx, cancelTimeout := context.WithTimeout(toolCtx, time.Duration(*timeout)*time.Millisecond)
		defer cancelTimeout()

		result := evaluator(timeoutCtx, input.Code)
		if result.Error != nil {
			log.Printf("Error evaluating JavaScript: %v", result.Error.Message)
		}
		return nil, result, nil
	})

	address := fmt.Sprintf(":%d", *port)
	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(req *http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true},
	)

	httpServer := &http.Server{
		Addr:           address,
		Handler:        http.MaxBytesHandler(mcpHandler, maxBodyBytes),
		ReadTimeout:    readTimeoutSeconds * time.Second,
		WriteTimeout:   writeTimeoutSeconds * time.Second,
		MaxHeaderBytes: 1 << maxHeaderExponent,
	}

	log.Printf("Ready to start HTTP MCP server. Listening on %s\n", address)
	err = httpServer.ListenAndServe()
	if err != nil {
		log.Fatalf("Failed to listen and serve: %v", err)
	}
}
