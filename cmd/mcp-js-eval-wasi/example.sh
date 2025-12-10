#!/bin/sh

# You can install the wasi bytecode like this:
#   cargo install --locked rs-js-eval-boa --target wasm32-wasip1  --profile release-wasi
# Note: To prevent supply chain attack, use ephemeral environment to build the wasi byte code

./mcp-js-eval-wasi \
	-port 12098 \
	-mem 32 \
	-timeout 200 \
	-path2engine "${HOME}/.cargo/bin/js-eval-boa.wasm"
