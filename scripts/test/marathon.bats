#!/usr/bin/env bats

MARATHON="$BATS_TEST_DIRNAME/../../marathon.sh"

setup() {
    export ANTHROPIC_API_KEY="sk-ant-test-fake-key"
    TEST_DIR="$(mktemp -d)"
    mkdir -p "$TEST_DIR/.ralph"
    echo "# Test prompt" > "$TEST_DIR/.ralph/PROMPT.md"
    echo 'PROJECT_NAME="test"' > "$TEST_DIR/.ralphrc"
}

teardown() {
    rm -rf "$TEST_DIR"
}

@test "marathon.sh --help exits 0" {
    run bash "$MARATHON" --help
    [ "$status" -eq 0 ]
    [[ "$output" == *"Usage:"* ]]
}

@test "marathon.sh -h exits 0" {
    run bash "$MARATHON" -h
    [ "$status" -eq 0 ]
    [[ "$output" == *"Marathon Loop"* ]]
}

@test "marathon.sh --dry-run prints command" {
    export RALPH_CMD="echo"
    run bash "$MARATHON" --dry-run -p "$TEST_DIR"
    [ "$status" -eq 0 ]
    [[ "$output" == *"[dry-run]"* ]]
}

@test "marathon.sh fails without ANTHROPIC_API_KEY" {
    unset ANTHROPIC_API_KEY
    run bash "$MARATHON" --dry-run -p "$TEST_DIR"
    [ "$status" -eq 1 ]
    [[ "$output" == *"ANTHROPIC_API_KEY"* ]]
}

@test "marathon.sh fails with nonexistent project dir" {
    run bash "$MARATHON" --dry-run -p "/nonexistent/dir"
    [ "$status" -eq 1 ]
    [[ "$output" == *"not found"* ]]
}

@test "marathon.sh fails without PROMPT.md" {
    rm "$TEST_DIR/.ralph/PROMPT.md"
    run bash "$MARATHON" --dry-run -p "$TEST_DIR"
    [ "$status" -eq 1 ]
    [[ "$output" == *"PROMPT.md"* ]]
}

@test "marathon.sh rejects unknown flags" {
    run bash "$MARATHON" --bogus-flag
    [ "$status" -eq 1 ]
    [[ "$output" == *"Unknown option"* ]]
}

@test "marathon.sh custom budget" {
    export RALPH_CMD="echo"
    run bash "$MARATHON" --dry-run -p "$TEST_DIR" -b 50
    [ "$status" -eq 0 ]
    [[ "$output" == *"$50"* ]]
}

@test "marathon.sh custom duration" {
    export RALPH_CMD="echo"
    run bash "$MARATHON" --dry-run -p "$TEST_DIR" -d 6
    [ "$status" -eq 0 ]
    [[ "$output" == *"6h"* ]]
}
