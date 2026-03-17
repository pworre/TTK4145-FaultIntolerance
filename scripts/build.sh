#!/usr/bin/env bash

set -e

# Check that ID argument is provided
if [ -z "$1" ]; then
    echo "Usage: $0 <x_id>"
    exit 1
fi

X_ID=$1
X_PORT=1567

echo "Updating submodules..."
git submodule update --init --recursive

cd external/Project-resources/cost_fns/hall_request_assigner

echo "Updating submodule for D-compiler"
git submodule update --init --recursive

echo "Making hall_request_assigner build script executable..."
chmod +x build.sh

echo "Running hra-installer..."
./script/hra_install.sh

echo "Starting elevator server..."
elevatorserver --port=$X_PORT &

echo "Running Go program..."
go run main.go --id=$X_ID --port=$X_PORT

