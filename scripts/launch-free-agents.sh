#!/bin/bash

export PATH=$PATH:/home/hg/.local/bin
PROJECT_ROOT="/home/hg/hairglasses-studio/ralphglasses"
cd "$PROJECT_ROOT"
LOG_DIR="$PROJECT_ROOT/.ralph/logs"
mkdir -p "$LOG_DIR"

echo "Starting Orchestrator..."
echo "Project Root: $PROJECT_ROOT"

TASK_DIR="docs/ralph-roadmap/free-agent-tasks"
MODELS=("gemini-2.5-flash" "gemini-2.5-flash-lite" "gemini-2.5-pro")
MAX_RETRIES=3

if [ ! -d "$TASK_DIR" ]; then
    echo "Error: Task directory $TASK_DIR not found."
    exit 1
fi

TASK_FILES=$(ls "$TASK_DIR"/task-*.md | sort)

for task_file in $TASK_FILES; do
    task_name=$(basename "$task_file")
    echo "--------------------------------------------------------"
    echo "Processing $task_name..."
    
    task_content=$(cat "$task_file")
    
    success=false
    retry_count=0
    
    while [ $retry_count -lt $MAX_RETRIES ] && [ "$success" = false ]; do
        selected_model=${MODELS[$((retry_count % ${#MODELS[@]}))]}
        echo "Attempt $((retry_count + 1)) using model: $selected_model"
        
        log_file="$LOG_DIR/launch-${task_name%.md}.log"
        
        # Run gemini sequentially
        gemini -y -m "$selected_model" -p "$task_content" > "$log_file" 2>&1
        exit_code=$?
        
        if [ $exit_code -eq 0 ]; then
            echo "Task $task_name completed successfully."
            success=true
        else
            echo "Task $task_name failed (Exit: $exit_code). Checking for rate limits..."
            if grep -E "429|RESOURCE_EXHAUSTED" "$log_file" > /dev/null; then
                echo "Rate limit detected. Waiting 30s before retry..."
                sleep 30
            else
                echo "Other error detected. Retrying with next model..."
                sleep 5
            fi
            retry_count=$((retry_count + 1))
        fi
    done
    
    if [ "$success" = false ]; then
        echo "CRITICAL: Failed $task_name"
    fi
    
    echo "Waiting 5s before next task..."
    sleep 5
done

echo "--------------------------------------------------------"
echo "Orchestrator finished."
