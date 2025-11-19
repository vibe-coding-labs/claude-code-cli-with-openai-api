#!/bin/bash

export ANTHROPIC_BASE_URL="http://localhost:8082/proxy/8fccf7f4-392d-4351-8382-c7ffc1a9de76"
export ANTHROPIC_API_KEY="test"

echo "Starting Claude CLI with input..."
echo "hi" | claude 2>&1 &
CLAUDE_PID=$!

echo "Waiting for output (5 seconds)..."
sleep 5

echo "Killing process..."
kill $CLAUDE_PID 2>/dev/null
wait $CLAUDE_PID 2>/dev/null

echo "Done"
