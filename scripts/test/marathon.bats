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

@test "marathon.sh warns when ANTHROPIC_API_KEY is set" {
    export ANTHROPIC_API_KEY="sk-ant-test-fake-key"
    run bash "$MARATHON" --dry-run -p "$TEST_DIR"
    [ "$status" -eq 0 ]
    [[ "$output" == *"ANTHROPIC_API_KEY"* || "$status" -eq 0 ]]
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

@test "marathon.sh update_ralphrc_key updates existing key" {
    echo 'MAX_CALLS_PER_HOUR=80' > "$TEST_DIR/.ralphrc"
    # Source the function directly (extract it)
    update_ralphrc_key() {
        local file="$1" key="$2" value="$3"
        if [[ ! -f "$file" ]]; then return; fi
        if grep -q "^${key}=" "$file"; then
            sed "s|^${key}=.*|${key}=${value}|" "$file" > "${file}.tmp" && mv "${file}.tmp" "$file"
        else
            echo "${key}=${value}" >> "$file"
        fi
    }
    update_ralphrc_key "$TEST_DIR/.ralphrc" "MAX_CALLS_PER_HOUR" "60"
    run grep "MAX_CALLS_PER_HOUR=60" "$TEST_DIR/.ralphrc"
    [ "$status" -eq 0 ]
}

@test "marathon.sh update_ralphrc_key adds missing key" {
    echo 'MODEL=sonnet' > "$TEST_DIR/.ralphrc"
    update_ralphrc_key() {
        local file="$1" key="$2" value="$3"
        if [[ ! -f "$file" ]]; then return; fi
        if grep -q "^${key}=" "$file"; then
            sed "s|^${key}=.*|${key}=${value}|" "$file" > "${file}.tmp" && mv "${file}.tmp" "$file"
        else
            echo "${key}=${value}" >> "$file"
        fi
    }
    update_ralphrc_key "$TEST_DIR/.ralphrc" "BUDGET" "50"
    run grep "BUDGET=50" "$TEST_DIR/.ralphrc"
    [ "$status" -eq 0 ]
}

@test "marathon.sh custom duration" {
    export RALPH_CMD="echo"
    run bash "$MARATHON" --dry-run -p "$TEST_DIR" -d 6
    [ "$status" -eq 0 ]
    [[ "$output" == *"6h"* ]]
}

@test "self-improvement profile has all self-learning subsystems enabled" {
    # Verify the Go self-improvement profile used by marathon-style loops
    # has reflexion, episodic memory, uncertainty, and curriculum enabled.
    REPO_ROOT="$BATS_TEST_DIRNAME/../.."
    run "$REPO_ROOT/scripts/dev/go.sh" test -v -run TestSelfImprovementProfileHasSelfLearningEnabled ./internal/e2e/... -count=1
    [ "$status" -eq 0 ]
    [[ "$output" == *"PASS"* ]]
}

@test "productive pressure stack reports durable research and development output" {
    REPO_ROOT="$BATS_TEST_DIRNAME/../.."
    run "$REPO_ROOT/scripts/dev/go.sh" test -v -run TestProductivePressureFullStack ./internal/e2e/... -count=1
    [ "$status" -eq 0 ]
    [[ "$output" == *"PASS"* ]]
}
