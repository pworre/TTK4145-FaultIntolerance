#!/bin/bash
set -e  # Exit immediately if a command fails

# Paths
PROJECT_ROOT="$(pwd)"
HRA_PATH="$PROJECT_ROOT/elevatorControl/hra"
D_SRC_PATH="$PROJECT_ROOT/external/Project-resources/cost_fns/hall_request_assigner"
JSONX_PATH="$D_SRC_PATH/d-json"

# Make sure the output folder exists
mkdir -p "$HRA_PATH"

echo "Cleaning old binaries and object files..."
rm -f "$HRA_PATH/hall_request_assigner"
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
