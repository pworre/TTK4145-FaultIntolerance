#!/usr/bin/env bash

set -e

# Move into scripts directory if not already
cd "$(dirname "$0")"

# Check that ID argument is provided
if [ -z "$1" ]; then
    echo "Usage: $0 <x_id>"
    exit 1
fi

X_ID=$1
X_PORT=1567


echo "Starting elevator server in new terminal..."
gnome-terminal -- bash -c "elevatorserver --port=$X_PORT; exec bash"

sleep 1

echo "Running Go program..."
go run main.go --id=$X_ID --port=$X_PORT
