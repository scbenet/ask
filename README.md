# Ask CLI

A terminal-based chat interface for interacting with LLMs through the OpenRouter API. Ask CLI provides a clean, intuitive TUI (Text User Interface) that lets you select different models and chat with them directly from your terminal.

## Features

- Terminal-based chat interface built with [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- Support for multiple LLM providers through [OpenRouter](https://openrouter.ai)
- Model selection with quick access via Ctrl+K
- Full conversation history maintained between interactions
- Clean, intuitive UI with keyboard navigation

## Installation

### Prerequisites

- Go 1.19 or higher
- An OpenRouter API key

### Build from Source

```bash
# Clone the repository
git clone https://github.com/scbenet/ask.git
cd ask

# Build the binary
go build -o ask main.go

# Optional: Install to your $GOPATH/bin
go install
```

## Usage

### Setting up your OpenRouter API Key

OpenRouter is a service that makes many models from various providers available through a single, unified API. Ask looks for an OPENROUTER_API_KEY variable set in your environment to make requests. Currently, it will not work without this.

In the future we plan to add support for local models and alternative API providers, but for now you will need to visit [OpenRouter](https://openrouter.ai) and create an API key.

Once you have your key created, set it as an environment variable from your shell.

```bash
export OPENROUTER_API_KEY="your_api_key_here"
```

### Running Ask CLI

```bash

# run the application
./ask

# or if you installed it with 'go install'
ask
```

## Keyboard Shortcuts

- Enter: Send message
- Shift+Enter (only with terminals that support the Kitty protocol) or Ctrl+J: Insert newline in the message input
- Ctrl+K: Open model selector
- Ctrl+C: Quit application
- Up/Down: Scroll through chat history when focused on history

## Development

Ask CLI is built with:

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) terminal UI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) for terminal styling

Ask is in the early stages of development, so bugs are expected and many features are still in the works. If you try it out and encounter an issue or have some feedback, feel free to create an issue and let me know!

Planned features:

- Context picker: select files/folders to include in your conversation as context
- Config file: easily add/change models, backends, system prompts, and inference parameters through a single unified configuration file
- Rich text formatting for both prompts and responses
- Response streaming
- Support for multiple conversations and persisted conversations
- CLI arguments to select a model and inital prompt on startup

