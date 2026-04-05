# This script automates the process of applying a patch (provided as a zip file) to the oai_cli directory,
# then stages and commits the changes to git, and finally gives you a shortcut for opening a pull request if the GitHub CLI is available.

#!/bin/bash

# Usage: ./apply_patch.sh patch.zip "Your commit message"
# Arguments:
#   $1 - Path to the patch zip file
#   $2 - Commit message for git

PATCH_ZIP="$1"
COMMIT_MSG="$2"

# Check for required arguments
if [ -z "$PATCH_ZIP" ] || [ -z "$COMMIT_MSG" ]; then
  echo "Usage: $0 path/to/patch.zip \"Commit message\""
  exit 1
fi

# Create a temporary directory for extracting the patch
TMP_DIR=$(mktemp -d)
unzip -o "$PATCH_ZIP" -d "$TMP_DIR"

# Copy the extracted patch contents into the oai_cli directory, overwriting as needed
cp -R "$TMP_DIR"/* ./oai_cli/

# Remove the temporary directory to clean up
rm -rf "$TMP_DIR"

# Change directory to oai_cli (using a custom function or alias 'gacd')
gacd oai_cli || exit 1

# Stage all changes and commit with the provided message
git add .
git commit -m "$COMMIT_MSG"

# If the GitHub CLI ('gh') is installed, suggest a shortcut to push and create a PR
if command -v gh &> /dev/null; then
  echo "✅ Patch applied and committed."
  echo "➡️  To push and create PR: git push -u origin HEAD && gh pr create --fill"
else
  echo "✅ Patch applied and committed. Push manually if desired."
fi

# In summary:
# - This script is a one-stop shop for patching, committing, and (optionally) PR creation.
# - It expects a zip file containing patch contents and a commit message.
# - It handles temp files, git workflow, and even gives you a PR shortcut if you're a GitHub CLI power user.
