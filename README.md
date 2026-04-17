# Agent Core

`github.com/nuln/agent-core` — Go AI Agent 核心框架，提供消息路由引擎、插件注册、会话管理、配置系统、定时任务和语音处理等基础能力。

## 功能特性

- **Pipe 管道** — 有序消息处理链，每个 Pipe 可拦截或透传消息
- **插件注册** — Pipe / Dialog / LLM 三类插件均通过 `init()` 自动注册
- **配置规格系统** — 插件声明配置规格，框架提供 env→opts→default 解析链
- **会话管理** — 基于 SessionKey 的会话跟踪与 metadata
- **定时任务** — CronScheduler，持久化存储任务配置
- **消息转发** — RelayManager，多 Bot 协作
- **卡片消息** — 构建结构化交互卡片（飞书等平台）
- **国际化** — 内置中/英文，可扩展
- **语音处理** — STT/TTS 接口
- **诊断工具** — RunDoctorChecks 系统健康检查
- **Unix Socket API** — 进程间通信管理接口

## 安装

```bash
go get github.com/nuln/agent-core
```

## 核心接口

### Pipe

```go
type Pipe interface {
    Handle(ctx context.Context, p Dialog, msg *Message) bool
}

// 注册（在插件 init() 中）
agent.RegisterPipe("name", priority, func(pctx agent.PipeContext) agent.Pipe {
    return &MyPipe{}
})
```

`Handle` 返回 `true` 停止链路，`false` 继续传递。

### Dialog（IM 平台）

```go
type Dialog interface {
    Name() string
    Start(handler MessageHandler) error
    Reply(ctx context.Context, replyCtx any, content string) error
    Stop() error
}

agent.RegisterDialog("name", func(opts map[string]any) (agent.Dialog, error) {
    return &MyDialog{}, nil
})
```

### LLM

```go
type LLM interface {
    Name() string
    Description() string
    Chat(ctx context.Context, sessionKey string, msgs []*Message) (string, error)
}

agent.RegisterLLM("name", func(opts map[string]any) (agent.LLM, error) {
    return &MyLLM{}, nil
})
```

### PipeContext

```go
type PipeContext struct {
    Sessions   SessionProvider
    Translator Translator
    GetAgents  func() []AgentInfo
    // Inject 将 content 作为用户输入注入到 sessionKey 对应的会话
    Inject     func(ctx context.Context, sessionKey, content string)
}
```

### Message

```go
type Message struct {
    MessageID  string
    SessionKey string
    UserID     string
    Access     string        // "private" | "group" | ...
    Content    string
    Role       string        // "user" | "assistant"
    CreateTime time.Time
    ReplyCtx   any           // 平台回复上下文（不透明）
}
```

## 引擎

```go
engine := agent.NewEngine(sessions, translator, stt, tts, dataDir)

// 加载通过 RegisterDialog/RegisterLLM 注册的插件
if err := engine.LoadPlugins(agent.EngineConfig{
    Dialogs: []agent.PluginConfig{{Type: "lark", Options: map[string]any{"app_id": "...", "app_secret": "..."}}},
    LLMs:    []agent.PluginConfig{{Type: "claudecode", Options: map[string]any{"work_dir": "."}}},
    DefaultLLM: "claudecode",
}); err != nil {
    log.Fatal(err)
}

// 或 AutoLoad：从已注册插件中自动发现可用的（依赖环境变量）
engine.AutoLoad()
```

## 配置规格系统

### 插件声明配置（在 init() 中）

```go
agent.RegisterPluginConfigSpec(agent.PluginConfigSpec{
    PluginName:  "myplugin",
    PluginType:  "pipe",
    Description: "插件描述",
    Fields: []agent.ConfigField{
        {
            EnvVar:      "MYPLUGIN_TOKEN",
            Key:         "token",
            Description: "API Token",
            Required:    true,
            Type:        agent.ConfigFieldSecret,
        },
        {
            EnvVar:  "MYPLUGIN_PORT",
            Key:     "port",
            Default: "8080",
            Type:    agent.ConfigFieldInt,
        },
    },
})
```

### 解析配置（env → opts → default）

```go
// 在插件工厂函数中
spec, _ := agent.GetPluginConfigSpec("myplugin")
cfg := agent.ResolveAllConfig(spec, opts)
port := cfg["port"]  // 优先 env MYPLUGIN_PORT，否则 opts["port"]，否则 "8080"

// 或单字段解析
val, found := agent.ResolveConfigValue(field, opts)
```

### 加载 .env 文件

```go
// 不覆盖已有环境变量
if err := agent.AutoLoadEnvFile(".env"); err != nil {
    log.Fatal(err)
}
```

### 校验必填配置

```go
errs := agent.ValidateAllPluginConfigs(nil)
for _, e := range errs {
    log.Printf("配置缺失: %v", e)
}
```

### 生成配置模板

```go
// 生成所有插件的 .env 模板
fmt.Print(agent.GenerateEnvTemplate())

// 生成指定插件的 .env 模板
fmt.Print(agent.GenerateEnvTemplate("webhook", "lark"))
```

## 会话管理

```go
sessions := agent.NewSessionManager()

// 获取或创建会话（自动激活）
session := sessions.GetOrCreateActive(sessionKey)
session.AppendMessage(&agent.Message{Role: "user", Content: "Hello"})

// 设置元数据
session.SetMetadata("key", "value")
val := session.GetMetadata("key")

// pending action 工作流
session.SetPendingAction("CONFIRM_OP")
if session.GetPendingAction() == "CONFIRM_OP" { ... }
```

## 定时任务

```go
store, _ := agent.NewCronStore(dataDir)
scheduler := agent.NewCronScheduler(store, engine)
scheduler.Start()

job := &agent.CronJob{
    ID:         "daily-report",
    CronExpr:   "0 9 * * *",
    SessionKey: "user:12345:main",
    Prompt:     "生成今日报告",
    Enabled:    true,
}
scheduler.AddJob(job)
```

## 卡片消息

```go
card := agent.NewCard().
    Title("标题", "#2196F3").
    Markdown("**加粗内容**").
    Buttons(
        agent.PrimaryBtn("确认", "confirm"),
        agent.DangerBtn("取消", "cancel"),
    ).
    Build()

// 需要 Dialog 实现 CardSender 接口
if cs, ok := dialog.(agent.CardSender); ok {
    cs.SendCard(ctx, replyCtx, card)
}
```

## 健康检查

```go
results := agent.RunDoctorChecks()
fmt.Println(agent.FormatDoctorResults(results))
```

## 数据目录结构

```
data_dir/
├── crons/
│   └── jobs.json          定时任务配置
├── relay_bindings.json    消息转发绑定
└── run/
    └── api.sock           Unix Socket API
```

## 测试

```bash
cd agent-core && go test ./... -timeout 30s
```

## License

MIT — See [LICENSE](./LICENSE)

