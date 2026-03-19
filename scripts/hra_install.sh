#!/usr/bin/env bash
set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HRA_PATH="$PROJECT_ROOT/elevatorControl/hallRequestAssigner"
D_SRC_PATH="$PROJECT_ROOT/external/Project-resources/cost_fns/hall_request_assigner"
JSONX_PATH="$D_SRC_PATH/d-json"

if [ ! -d "$D_SRC_PATH" ]; then
    echo "Error: Missing hall_request_assigner sources at:"
    echo "  $D_SRC_PATH"
    echo
    echo "This archive must include the contents of the submodule:"
    echo "  external/Project-resources"
    exit 1
fi

mkdir -p "$HRA_PATH"

echo "Cleaning old binaries and object files..."
rm -f "$HRA_PATH/hall_request_assigner"
rm -f "$HRA_PATH/hall_request_assigner.o"
rm -f "$JSONX_PATH/jsonx.o"

echo "Building jsonx library..."
dmd -c "$JSONX_PATH/jsonx.d" -of="$JSONX_PATH/jsonx.o"

echo "Building hall_request_assigner..."
dmd "$D_SRC_PATH/main.d" \
    "$D_SRC_PATH/elevator_algorithm.d" \
    "$D_SRC_PATH/optimal_hall_requests.d" \
    "$D_SRC_PATH/config.d" \
    "$D_SRC_PATH/elevator_state.d" \
    "$JSONX_PATH/jsonx.o" \
    -I"$D_SRC_PATH" -I"$JSONX_PATH" \
    -of="$HRA_PATH/hall_request_assigner"

chmod +x "$HRA_PATH/hall_request_assigner"

echo "hall_request_assigner built successfully and placed in $HRA_PATH"
