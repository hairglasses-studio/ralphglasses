# Shell Completions

ralphglasses generates shell completions for Bash, Zsh, and Fish.

## Generating Completions

```bash
# Bash
ralphglasses completion bash > /path/to/completions/ralphglasses.bash

# Zsh
ralphglasses completion zsh > /path/to/completions/_ralphglasses

# Fish
ralphglasses completion fish > ~/.config/fish/completions/ralphglasses.fish
```

## Installation by Shell

### Bash

```bash
# Option 1: Source in .bashrc
echo 'source <(ralphglasses completion bash)' >> ~/.bashrc

# Option 2: Place in completions directory
# Linux
ralphglasses completion bash > /etc/bash_completion.d/ralphglasses
# macOS (with bash-completion@2 via Homebrew)
ralphglasses completion bash > $(brew --prefix)/etc/bash_completion.d/ralphglasses
```

### Zsh

```bash
# Option 1: Place in fpath directory
ralphglasses completion zsh > "${fpath[1]}/_ralphglasses"

# Option 2: Create a local completions directory
mkdir -p ~/.zsh/completions
ralphglasses completion zsh > ~/.zsh/completions/_ralphglasses
# Add to .zshrc:
echo 'fpath=(~/.zsh/completions $fpath)' >> ~/.zshrc
echo 'autoload -Uz compinit && compinit' >> ~/.zshrc

# Option 3: Oh My Zsh
ralphglasses completion zsh > ~/.oh-my-zsh/completions/_ralphglasses
```

### Fish

```bash
ralphglasses completion fish > ~/.config/fish/completions/ralphglasses.fish
```

Fish automatically loads completions from `~/.config/fish/completions/`.

## Goreleaser Distribution

Completions are automatically generated during release builds and included in
the release archives under `completions/`. The goreleaser config runs:

```yaml
before:
  hooks:
    - sh -c "mkdir -p completions && go run . completion bash > completions/ralphglasses.bash && go run . completion zsh > completions/_ralphglasses && go run . completion fish > completions/ralphglasses.fish"
```

## Supported Completions

- **Flags**: All global and command-specific flags
- **Commands**: All subcommands including `mcp`, `marathon`, `budget`, `tmux`, etc.
- **Flag values**: `--scan-path` completes directory paths, `--theme` completes theme names
- **Session IDs**: Commands like `budget set` and `budget reset` complete session IDs
