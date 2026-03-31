#!/usr/bin/env bash
# k8s-context.sh — Waybar custom module: current kubectl context + namespace
# Output: Waybar JSON {"text", "tooltip", "class"}

ctx=$(kubectl config current-context 2>/dev/null)
if [ -z "$ctx" ]; then
    echo '{"text":"⎈ —","tooltip":"no kubectl context","class":"disconnected"}'
    exit 0
fi

ns=$(kubectl config view --minify -o jsonpath='{..namespace}' 2>/dev/null)
ns="${ns:-default}"

# Shorten known context names
short="$ctx"
case "$ctx" in
    *production*|*prod*) short="prod" ;;
    *staging*|*stg*)     short="stg" ;;
    *development*|*dev*) short="dev" ;;
esac

echo "{\"text\":\"⎈ ${short}/${ns}\",\"tooltip\":\"context: ${ctx}\\nnamespace: ${ns}\",\"class\":\"connected\"}"
