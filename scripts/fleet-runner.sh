#!/bin/bash

export PATH=$PATH:/home/hg/.local/bin
PROJECT_ROOT="/home/hg/hairglasses-studio/ralphglasses"
cd "$PROJECT_ROOT"
LOG_DIR="$PROJECT_ROOT/.ralph/logs"
mkdir -p "$LOG_DIR"
ORCHESTRATOR_LOG="$LOG_DIR/orchestrator.log"

# launch-free-agents.sh - Orchestrate roadmap task assignment using gemini CLI.
# Using confirmed working gemini-2.5 models.

TASK_DIR="docs/ralph-roadmap/free-agent-tasks"
# Confirmed working models for Gemini CLI 0.37.0
MODELS=("gemini-2.5-flash" "gemini-2.5-flash-lite" "gemini-2.5-pro")
MAX_RETRIES=3
INITIAL_WAIT=15

if [ ! -d "$TASK_DIR" ]; then
    echo "Error: Task directory $TASK_DIR not found."
    exit 1
fi

# Get list of task files sorted by number
TASK_FILES=$(ls "$TASK_DIR"/task-*.md | sort)

for task_file in $TASK_FILES; do
    task_name=$(basename "$task_file")
    echo "--------------------------------------------------------"
    echo "Processing $task_name..."
    
    task_content=$(cat "$task_file")
    
    success=false
    retry_count=0
    wait_time=$INITIAL_WAIT
    
    while [ $retry_count -lt $MAX_RETRIES ] && [ "$success" = false ]; do
        selected_model=${MODELS[$((retry_count % ${#MODELS[@]}))]}
        
        echo "Attempt $((retry_count + 1))/$MAX_RETRIES using gemini model: $selected_model"
        
        log_file=".ralph/logs/launch-${task_name%.md}.log"
        
        # Run gemini in non-interactive yolo mode
        # Redirecting log output to capture any failures
        gemini -y --approval-mode yolo -m "$selected_model" -p "$task_content" > "$log_file" 2>&1 &
        agent_pid=$!
        
        sleep 10
        
        if ps -p $agent_pid > /dev/null; then
            echo "Task $task_name started (PID: $agent_pid). Waiting for completion..."
            wait $agent_pid
            exit_code=$?
            
            if [ $exit_code -eq 0 ]; then
                echo "Task $task_name completed successfully."
                success=true
            else
                echo "Task $task_name failed (Exit: $exit_code). Logs:"
                tail -n 10 "$log_file"
                retry_count=$((retry_count + 1))
                if grep -E "429|RESOURCE_EXHAUSTED" "$log_file" > /dev/null; then
                    echo "Detected rate limit. Retrying in ${wait_time}s..."
                    sleep $wait_time
                    wait_time=$((wait_time * 2))
                else
                    echo "Non-rate-limit error. Moving to next retry/model..."
                    sleep 5
                fi
            fi
        else
            wait $agent_pid
            exit_code=$?
            echo "Task $task_name failed immediately (Exit: $exit_code)."
            tail -n 10 "$log_file"
            retry_count=$((retry_count + 1))
            sleep $wait_time
            wait_time=$((wait_time * 2))
        fi
    done
    
    if [ "$success" = false ]; then
        echo "CRITICAL: Failed to launch $task_name after $MAX_RETRIES attempts."
    fi
    
    echo "Waiting 5s before next task..."
    sleep 5
done

echo "--------------------------------------------------------"
echo "Launch script finished."
