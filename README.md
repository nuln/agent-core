# Agent Core

A powerful Go AI Agent core library providing session management, message routing, scheduling, and voice processing capabilities.

English | [简体中文](./README-zh.md)

## Features

- **Session Management** - Flexible session tracking and management system
- **Multi-Platform Support** - Support for multiple dialog platforms (extensible interface)
- **LLM Integration** - Support for multiple LLM providers with flexible switching
- **Scheduling** - Cron-based job scheduling system
- **Voice Processing** - Integrated Speech-to-Text (STT) and Text-to-Speech (TTS)
- **Card Support** - Build and render structured card messages
- **Internationalization** - Built-in English and Chinese language support
- **Permission Management** - Permission request and approval workflow
- **Message Relay** - Bot-to-bot message relay and collaboration
- **Unix Socket API** - Local Unix Socket interface for inter-process communication
- **Diagnostics** - System health checks and diagnostic tools

## Quick Start

### Installation

```bash
go get github.com/nuln/agent-core
```

## Core Components

### Engine

The core engine responsible for coordinating dialog platforms and LLMs, managing message routing.

```go
engine := agent.NewEngine(sessions, translator, stt, tts, dataDir)
engine.RegisterDialog(platform)
engine.RegisterLLM(llm)
```

### Session

The session manager tracks conversation history between users and bots.

```go
session := sessionProvider.GetOrCreateSession(sessionID)
session.AppendMessage(message)
```

### Dialog

Dialog platform interface for integrating with various messaging applications.

```go
type Dialog interface {
    Name() string
    Start(handler MessageHandler) error
    Send(ctx context.Context, replyCtx any, content string) error
    Stop() error
}
```

### LLM

LLM interface for integrating different large language models.

```go
type LLM interface {
    Name() string
    Description() string
    Chat(ctx context.Context, msgs []*Message) (string, error)
}
```

### CronScheduler

Manages and executes scheduled tasks.

```go
scheduler := agent.NewCronScheduler(store, engine)
scheduler.Start()
scheduler.AddJob(cronJob)
```

### Pipes

Configurable message processing pipelines for implementing permission checks, logging, and more.

## Configuration

### Environment Variables

- `AGENT_DATA_DIR` - Data storage directory path
- `AGENT_LOG_LEVEL` - Log level (debug, info, warn, error)

### Data Directory Structure

```
data_dir/
├── crons/
│   └── jobs.json       # Cron job configuration
├── sessions/
│   └── *.json          # Session data 
├── relay_bindings.json # Message relay configuration
└── run/
    └── api.sock        # Unix Socket
```

## API

### Start Engine

```go
if err := engine.Start(ctx); err != nil {
    log.Fatal(err)
}
```

### Handle Messages

```go
engine.handleMessage(dialog, &agent.Message{
    Content: "User input",
    Role: agent.RoleUser,
})
```

### Scheduled Tasks

```go
job := &agent.CronJob{
    ID: "task-1",
    CronExpr: "0 9 * * *",      // Daily at 9:00 AM
    Project: "my-agent",
    SessionKey: "account:chat:user",
    Prompt: "Task prompt",
    Enabled: true,
}
scheduler.AddJob(job)
```

### Send Card Messages

```go
card := agent.NewCard().
    Title("Card Title", "#2196F3").
    Markdown("**Bold content**").
    Buttons(
        agent.PrimaryBtn("Button", "action_value"),
    ).
    Build()

if err := dialog.SendCard(ctx, replyCtx, card); err != nil {
    log.Printf("Failed to send card: %v", err)
}
```

## Testing

Run all tests:

```bash
make test
```

Generate coverage report:

```bash
make coverage
```

## Code Quality

### Format Code

```bash
make fmt
```

### Lint Code

```bash
make lint
```

### Check Dependencies

```bash
make check
```

## CI/CD

The project uses GitHub Actions for automated testing and checks:

- **Continuous Integration** - Run tests on Linux, macOS, and Windows
- **Code Quality** - golangci-lint for static analysis
- **Security Checks** - Go vulnerability scanning
- **Dependency Checks** - Verify go.mod integrity

See [.github/workflows/ci.yml](./.github/workflows/ci.yml) for details.

## Contributing

Pull requests and issues are welcome!

## License

MIT License - See [LICENSE](./LICENSE) file for details

## Related Resources

- [Go Documentation](https://pkg.go.dev/github.com/nuln/agent-core)
- [robfig/cron](https://github.com/robfig/cron) - Cron scheduling library
- [stretchr/testify](https://github.com/stretchr/testify) - Testing framework
