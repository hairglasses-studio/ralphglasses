#!/usr/bin/env bats

# Tests for macOS-specific bootstrap path detection.
# Exercises bootstrap-toolchain.sh and doctor.sh under mocked PATH environments.

BOOTSTRAP="$BATS_TEST_DIRNAME/../bootstrap-toolchain.sh"
DOCTOR="$BATS_TEST_DIRNAME/../dev/doctor.sh"
ENV_SH="$BATS_TEST_DIRNAME/../dev/env.sh"
BASH_BIN="$(command -v bash)"

setup() {
    STUB_DIR="$(mktemp -d)"
    # Essential system tools needed by env.sh/doctor.sh internals
    for tool in awk grep uname dirname basename cat mkdir tar rm mv sed xcode-select; do
        real="$(PATH="/usr/bin:/bin:/usr/sbin:/sbin" command -v "$tool" 2>/dev/null)" || true
        if [ -n "$real" ] && [ -x "$real" ]; then
            ln -sf "$real" "$STUB_DIR/$tool"
        fi
    done
}

teardown() {
    rm -rf "$STUB_DIR"
}

make_stub() {
    local name="$1"
    local output="${2:-stub $name}"
    printf '#!/bin/sh\necho "%s"\n' "$output" > "$STUB_DIR/$name"
    chmod +x "$STUB_DIR/$name"
}

# --- env.sh unit tests ---

@test "rg_go_bin returns .tools path for repo root" {
    source "$ENV_SH"
    local repo_root
    repo_root="$(rg_repo_root)"
    local go_bin
    go_bin="$(rg_go_bin "$repo_root")"
    [[ "$go_bin" == *"/.tools/go/"*"/bin/go" ]]
}

@test "env.sh detects Darwin uname on macOS" {
    if [[ "$(uname -s)" != "Darwin" ]]; then
        skip "not running on macOS"
    fi
    source "$ENV_SH"
    [[ "$(uname -s)" == "Darwin" ]]
}

# --- doctor.sh path detection ---

@test "doctor exits 0 with go+make+git on PATH" {
    make_stub "go" "go version go1.24.0 darwin/arm64"
    make_stub "make" "GNU Make 4.4.1"
    make_stub "git" "git version 2.44.0"

    run env PATH="$STUB_DIR" "$BASH_BIN" "$DOCTOR"
    [ "$status" -eq 0 ]
}

@test "doctor exits non-zero when git is missing" {
    make_stub "go" "go version go1.24.0 darwin/arm64"
    make_stub "make" "GNU Make 4.4.1"

    run env PATH="$STUB_DIR" "$BASH_BIN" "$DOCTOR"
    [ "$status" -eq 1 ]
}

@test "doctor warns about missing brew on macOS" {
    if [[ "$(uname -s)" != "Darwin" ]]; then
        skip "macOS-specific test"
    fi
    make_stub "go" "go version go1.24.0 darwin/arm64"
    make_stub "make" "GNU Make 4.4.1"
    make_stub "git" "git version 2.44.0"
    # brew deliberately absent; remove xcode-select stub to avoid that check interfering
    rm -f "$STUB_DIR/xcode-select"

    run env PATH="$STUB_DIR" "$BASH_BIN" "$DOCTOR"
    [ "$status" -eq 0 ]
    [[ "$output" == *"brew"* ]]
    [[ "$output" == *"warn"* ]]
}

@test "doctor reports brew ok when present on macOS" {
    if [[ "$(uname -s)" != "Darwin" ]]; then
        skip "macOS-specific test"
    fi
    make_stub "go" "go version go1.24.0 darwin/arm64"
    make_stub "make" "GNU Make 4.4.1"
    make_stub "git" "git version 2.44.0"
    make_stub "brew" "/opt/homebrew/bin/brew"

    run env PATH="$STUB_DIR" "$BASH_BIN" "$DOCTOR"
    [ "$status" -eq 0 ]
    [[ "$output" == *"brew"* ]]
    [[ "$output" == *"ok"* ]]
}

# --- Homebrew path detection via bootstrap ---
# These tests verify that rg_has_matching_system_go correctly identifies
# go binaries at different Homebrew prefix paths (Apple Silicon vs Intel).

@test "rg_has_matching_system_go finds go at /opt/homebrew-style path" {
    source "$ENV_SH"
    local go_ver
    go_ver="$(rg_go_version)"

    # Create a go stub that reports the exact version go.mod expects
    mkdir -p "$STUB_DIR/opt-homebrew/bin"
    printf '#!/bin/sh\necho "go version go%s darwin/arm64"\n' "$go_ver" > "$STUB_DIR/opt-homebrew/bin/go"
    chmod +x "$STUB_DIR/opt-homebrew/bin/go"

    run env PATH="$STUB_DIR/opt-homebrew/bin:$STUB_DIR" "$BASH_BIN" -c \
        "source '$ENV_SH' && rg_has_matching_system_go '$go_ver'"
    [ "$status" -eq 0 ]
}

@test "rg_has_matching_system_go finds go at /usr/local-style path" {
    source "$ENV_SH"
    local go_ver
    go_ver="$(rg_go_version)"

    mkdir -p "$STUB_DIR/usr-local/bin"
    printf '#!/bin/sh\necho "go version go%s darwin/amd64"\n' "$go_ver" > "$STUB_DIR/usr-local/bin/go"
    chmod +x "$STUB_DIR/usr-local/bin/go"

    run env PATH="$STUB_DIR/usr-local/bin:$STUB_DIR" "$BASH_BIN" -c \
        "source '$ENV_SH' && rg_has_matching_system_go '$go_ver'"
    [ "$status" -eq 0 ]
}

@test "rg_has_matching_system_go rejects mismatched version" {
    mkdir -p "$STUB_DIR/wrong"
    printf '#!/bin/sh\necho "go version go0.0.0 darwin/arm64"\n' > "$STUB_DIR/wrong/go"
    chmod +x "$STUB_DIR/wrong/go"

    run env PATH="$STUB_DIR/wrong:$STUB_DIR" "$BASH_BIN" -c \
        "source '$ENV_SH' && rg_has_matching_system_go '1.26.1'"
    [ "$status" -eq 1 ]
}
