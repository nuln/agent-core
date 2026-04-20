package agent

import (
	"context"
	"time"
)

// ──────────────────────────────────────────────────────────────
// Core Message & Attachment Types
// ──────────────────────────────────────────────────────────────

// Message represents an incoming or outgoing message in the system.
type Message struct {
	MessageID  string            // Unique identifier for the message
	SessionKey string            // Key identifying the conversation session
	Access     string            // Access platform name (e.g., "feishu", "telegram")
	UserID     string            // Unique ID of the sender on the access platform
	UserName   string            // Display name of the sender
	Content    string            // Plain text content of the message
	ReplyCtx   any               // Access-specific context for replying (opaque)
	Images     []ImageAttachment // Optional image attachments
	Files      []FileAttachment  // Optional file attachments
	Audio      *AudioAttachment  // Optional audio/voice attachment
	FromVoice  bool              // Whether this message was transcribed from voice
	CreateTime time.Time         // When the message was created on the platform (zero = unknown)
}

// ImageAttachment represents an image sent to or from an LLM.
type ImageAttachment struct {
	Data     []byte // Raw image bytes
	Ext      string // File extension (e.g., "png", "jpg")
	MimeType string // MIME type (e.g., "image/png")
	FileName string // Original filename
}

// FileAttachment represents a general file attachment.
type FileAttachment struct {
	Data     []byte // Raw file bytes
	FileName string // Original filename
	MimeType string // MIME type
}

// AudioAttachment represents an audio or voice file.
type AudioAttachment struct {
	Data     []byte // Raw audio bytes
	Format   string // Audio format (e.g., "mp3", "wav", "silk")
	MimeType string // MIME type
	Duration int    // Duration in seconds
}

// ──────────────────────────────────────────────────────────────
// Session & Persistence Interfaces
// ──────────────────────────────────────────────────────────────

// Session abstracts a conversation's state and metadata.
type Session interface {
	GetID() string                     // Returns the unique session ID
	GetPendingAction() string          // Returns any pending security action (e.g., tool confirmation)
	SetPendingAction(action string)    // Stores a pending security action
	GetMetadata() map[string]any       // Returns key-value metadata for the session
	SetMetadata(key string, value any) // Sets a metadata value
	TryLock() bool                     // Attempt to lock the session for exclusive processing
	Unlock()                           // Release the session lock
	Save()                             // Explicitly persist the session state
	GetHistory() []HistoryEntry        // Returns the conversation history
	AppendHistory(entry HistoryEntry)  // Appends a new entry to the history
}

// SessionState stores a persistent logical stage of a conversation.
type SessionState struct {
	SessionKey  string                 `json:"session_key"`
	Stage       string                 `json:"stage"`      // e.g. "WAIT_PAYMENT"
	Action      string                 `json:"action"`     // description
	ContextVars map[string]interface{} `json:"vars"`       // temp data
	LockedBy    string                 `json:"locked_by"`  // pipe name that owns this state
	ExpiresAt   int64                  `json:"expires_at"` // unix milli
}

// SessionStateManager manages persistent session states.
type SessionStateManager interface {
	GetState(sessionKey string) (*SessionState, error)
	SetState(sessionKey string, state *SessionState) error
	ClearState(sessionKey string) error
	CleanupExpired() error
}

// SessionProvider provides session management and retrieval.
type SessionProvider interface {
	// GetOrCreateActive returns the active session for a given user key, creating one if none exists.
	GetOrCreateActive(key string) Session
}

// KVStore defines a generic persistent key-value store.
type KVStore interface {
	Get(key []byte) ([]byte, error)
	Put(key, value []byte) error
	Delete(key []byte) error
	List() (map[string][]byte, error)
}

// KVStoreProvider provides access to named KVStores.
type KVStoreProvider interface {
	GetStore(name string) (KVStore, error)
}

// ──────────────────────────────────────────────────────────────
// Platform (Dialog) Interfaces
// ──────────────────────────────────────────────────────────────

// Dialog abstracts a messaging platform (Instant Messenger).
type Dialog interface {
	Name() string                                                  // Returns the unique name of the platform (e.g., "feishu")
	Start(handler MessageHandler) error                            // Starts the platform listener with the provided message handler
	Reply(ctx context.Context, replyCtx any, content string) error // Sends a reply message
	Send(ctx context.Context, replyCtx any, content string) error  // Sends a new message
	Stop() error                                                   // Stops the platform listener
	Reload(opts map[string]any) error                              // Reloads platform configuration
}

// DialogInstanceStatus describes the health and identity of a single bot instance.
type DialogInstanceStatus struct {
	ID          string `json:"id"`
	Status      string `json:"status"` // "connected", "error", "starting", "unauthorized"
	InboundAt   int64  `json:"inbound_at"`
	Description string `json:"description"`
}

// AuthSession represents an active interactive login attempt (e.g., scanning a QR code).
type AuthSession struct {
	ID      string `json:"id"`
	QRUrl   string `json:"qr_url,omitempty"`
	Status  string `json:"status"` // "waiting", "scanned", "confirmed", "expired"
	Message string `json:"message,omitempty"`
}

// ManageableDialog is implemented by platforms that support multiple bots and interactive login.
type ManageableDialog interface {
	Dialog
	GetInstances() []DialogInstanceStatus
	StartAuth(ctx context.Context) (*AuthSession, error)
	PollAuth(ctx context.Context, sessionID string) (*AuthSession, error)
	AddInstance(ctx context.Context, params map[string]string) error
}

// MessageHandler is the callback function invoked when a new message arrives from a Dialog platform.
type MessageHandler func(p Dialog, msg *Message)

// AudioSender is implemented by Dialog platforms that support sending voice/audio messages.
type AudioSender interface {
	SendAudio(ctx context.Context, replyCtx any, audio []byte, format string) error
}

// CardNavigationHandler is called to render a card for in-place updates during navigation.
type CardNavigationHandler func(action string, sessionKey string) *Card

// CardNavigable is implemented by Dialog platforms that support in-place card navigation/updates.
type CardNavigable interface {
	SetCardNavigationHandler(h CardNavigationHandler)
}

// CardSender is implemented by Dialog platforms that support structured rich cards.
type CardSender interface {
	SendCard(ctx context.Context, replyCtx any, card *Card) error
	ReplyCard(ctx context.Context, replyCtx any, card *Card) error
}

// MessageUpdater is implemented by Dialog platforms that support editing already sent messages.
type MessageUpdater interface {
	UpdateMessage(ctx context.Context, msgID any, content string) error
}

// PreviewStarter is for Dialog platforms that support "streaming" or preview messages.
type PreviewStarter interface {
	// SendPreviewStart sends the initial version of a preview message and returns a handle for updates.
	SendPreviewStart(ctx context.Context, replyCtx any, content string) (handle any, err error)
}

// PreviewCleaner is for Dialog platforms that need to explicitly delete preview messages.
type PreviewCleaner interface {
	DeletePreviewMessage(ctx context.Context, handle any) error
}

// ReplyContextReconstructor is for Dialog platforms that can recreate a reply context from a session key.
type ReplyContextReconstructor interface {
	ReconstructReplyCtx(sessionKey string) (any, error)
}

// ButtonOption represents a clickable inline button for InlineButtonSender.
type ButtonOption struct {
	Text string // Label displayed on the button
	Data string // Callback data sent when clicked
}

// InlineButtonSender is implemented by Dialog platforms that support inline button rows.
type InlineButtonSender interface {
	SendWithButtons(ctx context.Context, replyCtx any, content string, buttons [][]ButtonOption) error
}

// DialogFactory creates a Dialog instance from a map of options.
type DialogFactory func(opts map[string]any) (Dialog, error)

// SkillManagerFactory creates a SkillManager instance from a map of options.
type SkillManagerFactory func(opts map[string]any) (SkillManager, error)

// ──────────────────────────────────────────────────────────────
// AI Model (LLM) Interfaces
// ──────────────────────────────────────────────────────────────

// LLM abstracts an AI assistant or large language model.
type LLM interface {
	Name() string                                                             // Returns the unique name of the LLM provider
	Description() string                                                      // Returns a human-readable description of the LLM
	StartSession(ctx context.Context, sessionID string) (AgentSession, error) // Starts an interactive session
	Stop() error                                                              // Stops the LLM provider
	Reload(opts map[string]any) error                                         // Reloads LLM configuration
}

// AgentSession represents a single interactive session with an LLM.
type AgentSession interface {
	Send(prompt string, images []ImageAttachment, files []FileAttachment) error // Sends a prompt with optional media
	Events() <-chan Event                                                       // Returns a channel for streaming events (text, tools, etc.)
	Close() error                                                               // Closes the session
}

// Event types emitted by the LLM.
const (
	EventText              = "text"               // Incremental text response
	EventThinking          = "thinking"           // Internal reasoning/thinking process
	EventToolUse           = "tool_use"           // Request to use a tool
	EventPermissionRequest = "permission_request" // Request for user permission
	EventResult            = "result"             // Final result/summary
	EventError             = "error"              // Error event
	EventToolResult        = "tool_result"        // Result of a tool execution
)

// Event represents an event emitted by an LLM session.
type Event struct {
	Type         string         // Event type (see constants above)
	Content      string         // Text content or reasoning
	SessionID    string         // ID of the session
	Done         bool           // Whether the interaction is complete
	Error        error          // Optional error information
	RequestID    string         // Tracking ID for the request
	ToolName     string         // Name of the tool being used/requested
	ToolInput    string         // Input parameters for the tool (stringified)
	ToolInputRaw map[string]any // Raw input parameters
	ToolResult   string         // Output/result of the tool
	Questions    []UserQuestion // Structured questions for the user
}

// PermissionResult is the user's response to a tool permission request.
type PermissionResult struct {
	Behavior     string         // "allow" | "deny"
	Message      string         // Reason for denial, if applicable
	UpdatedInput map[string]any // Optional modified tool parameters from the user
}

// AgentSessionInfo describes a historical session as reported by the LLM backend.
type AgentSessionInfo struct {
	ID           string    // Session ID
	Summary      string    // Short summary of the conversation
	MessageCount int       // Number of messages exchanged
	ModifiedAt   time.Time // Last modification timestamp
	GitBranch    string    // Optional git branch context
}

// AgentSystemPrompt returns the global system prompt used for all LLM interactions.
func AgentSystemPrompt() string {
	return "You are a helpful AI assistant. Always follow the user's instructions and use the provided tools to complete the task. Be concise and accurate."
}

// AgentFactory creates an LLM instance from a map of options.
type AgentFactory func(opts map[string]any) (LLM, error)

// ──────────────────────────────────────────────────────────────
// Pipe & Pipeline Interfaces
// ──────────────────────────────────────────────────────────────

// Pipe handles incoming messages at specific phases of the pipeline.
type Pipe interface {
	// Handle returns true if the message should be intercepted (stop processing pipeline).
	Handle(ctx context.Context, p Dialog, msg *Message) bool
}

// PipeContext provides necessary dependencies to a pipe during initialization.
type PipeContext struct {
	Sessions   SessionProvider    // Session management
	Translator Translator         // Translation services
	GetAgents  func() []AgentInfo // Callback to list available AI agents
	// Inject allows a pipe to synthesize an inbound message into a session (e.g. webhook/heartbeat).
	// When called, the engine routes the content through the full pipe-and-LLM pipeline
	// as if it were a real user message for the given sessionKey.
	Inject  func(ctx context.Context, sessionKey, content string)
	Storage KVStoreProvider     // Persistent storage provider for the pipe
	State   SessionStateManager // Persistent session state manager
}

// PipeFactory creates a Pipe instance.
type PipeFactory func(ctx PipeContext) Pipe

// ──────────────────────────────────────────────────────────────
// Supporting Types & Metadata
// ──────────────────────────────────────────────────────────────

// Translator abstracts internationalization and message translation.
type Translator interface {
	T(key string, args ...any) string // Translates a key with optional arguments
}

// BotCommandInfo represents a command for bot menu/help registration.
type BotCommandInfo struct {
	Command     string
	Description string
}

// UserQuestion represents a structured question requested by the LLM.
type UserQuestion struct {
	Question    string               `json:"question"`
	Header      string               `json:"header"`
	Options     []UserQuestionOption `json:"options"`
	MultiSelect bool                 `json:"multiSelect"`
}

// UserQuestionOption represents a single choice in a UserQuestion.
type UserQuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// ModelOption describes a selectable AI model.
type ModelOption struct {
	Name  string // model identifier passed to the CLI / API
	Desc  string // short description or display name
	Alias string // optional short alias for the /model command (e.g. "codex" for "gpt-5.3-codex")
}

// ProviderConfig holds API provider settings for an LLM.
type ProviderConfig struct {
	Name     string
	APIKey   string
	BaseURL  string
	Model    string
	Models   []ModelOption     // pre-configured list of available models for this provider
	Thinking string            // override thinking type ("disabled", "enabled", or "" for no rewrite)
	Env      map[string]string // extra env vars passed to the provider process
}

// PermissionModeInfo describes a security/permission mode.
type PermissionModeInfo struct {
	Key    string
	Name   string
	NameZh string
	Desc   string
	DescZh string
}

// AgentInfo describes an available AI model for UI display.
type AgentInfo struct {
	Name        string
	Description string
}

// HistoryEntry is one turn in a conversation.
type HistoryEntry struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
)

// ──────────────────────────────────────────────────────────────
// Extended Dialog (Platform) Interfaces
// ──────────────────────────────────────────────────────────────

// TypingIndicator is an optional interface for platforms that can show a
// "processing" indicator (typing bubble, emoji reaction, etc.) while the
// agent is working. StartTyping must not block; it returns a stop function
// that the caller must invoke when processing ends.
type TypingIndicator interface {
	StartTyping(ctx context.Context, replyCtx any) (stop func())
}

// ImageSender is an optional interface for platforms that support sending images.
type ImageSender interface {
	SendImage(ctx context.Context, replyCtx any, img ImageAttachment) error
}

// FileSender is an optional interface for platforms that support sending files.
type FileSender interface {
	SendFile(ctx context.Context, replyCtx any, file FileAttachment) error
}

// CommandRegistrar is an optional interface for platforms that support
// registering commands to the platform's native menu (e.g. Telegram setMyCommands).
type CommandRegistrar interface {
	RegisterCommands(commands []BotCommandInfo) error
}

// ChannelNameResolver is an optional interface for platforms that can resolve
// channel IDs to human-readable names.
type ChannelNameResolver interface {
	ResolveChannelName(channelID string) (string, error)
}

// ──────────────────────────────────────────────────────────────
// Extended LLM Interfaces
// ──────────────────────────────────────────────────────────────

// FormattingInstructionProvider is an optional interface for platforms that
// provide platform-specific formatting instructions for the agent system prompt
// (e.g., Slack mrkdwn vs standard Markdown).
type FormattingInstructionProvider interface {
	FormattingInstructions() string
}

// PlatformPromptInjector is an optional interface for LLMs that can receive
// platform-specific prompt fragments before a session starts.
type PlatformPromptInjector interface {
	SetPlatformPrompt(prompt string)
}

// SystemPromptSupporter is an optional marker interface for LLMs that
// natively inject the system prompt (e.g. via --append-system-prompt).
type SystemPromptSupporter interface {
	HasSystemPromptSupport() bool
}

// SessionEnvInjector is an optional interface for LLMs that accept
// per-session environment variables (e.g. CC_PROJECT, CC_SESSION_KEY).
type SessionEnvInjector interface {
	SetSessionEnv(env []string)
}

// ToolAuthorizer is an optional interface for LLMs that support dynamic tool authorization.
type ToolAuthorizer interface {
	AddAllowedTools(tools ...string) error
	GetAllowedTools() []string
}

// HistoryProvider is an optional interface for LLMs that can retrieve
// conversation history from their backend session files.
type HistoryProvider interface {
	GetSessionHistory(ctx context.Context, sessionID string, limit int) ([]HistoryEntry, error)
}

// ProviderSwitcher is an optional interface for LLMs that support multiple API providers.
type ProviderSwitcher interface {
	SetProviders(providers []ProviderConfig)
	SetActiveProvider(name string) bool
	GetActiveProvider() *ProviderConfig
	ListProviders() []ProviderConfig
}

// MemoryFileProvider is an optional interface for LLMs that support
// persistent instruction files (CLAUDE.md, AGENTS.md, GEMINI.md, etc.).
type MemoryFileProvider interface {
	ProjectMemoryFile() string // project-level instruction file
	GlobalMemoryFile() string  // user-level instruction file
}

// ModelSwitcher is an optional interface for LLMs that support runtime model switching.
// Model changes take effect on the next session start.
type ModelSwitcher interface {
	SetModel(model string)
	GetModel() string
	AvailableModels(ctx context.Context) []ModelOption
}

// ReasoningEffortSwitcher is an optional interface for LLMs that support
// runtime switching of reasoning effort.
type ReasoningEffortSwitcher interface {
	SetReasoningEffort(effort string)
	GetReasoningEffort() string
	AvailableReasoningEfforts() []string
}

// UsageReporter is an optional interface for LLMs that can report account
// or model quota usage from their backing provider.
type UsageReporter interface {
	GetUsage(ctx context.Context) (*UsageReport, error)
}

// UsageReport is a provider-neutral quota snapshot returned by UsageReporter.
type UsageReport struct {
	Provider  string
	AccountID string
	UserID    string
	Email     string
	Plan      string
	Buckets   []UsageBucket
	Credits   *UsageCredits
}

// UsageBucket groups one logical quota, such as standard requests or code review.
type UsageBucket struct {
	Name         string
	Allowed      bool
	LimitReached bool
	Windows      []UsageWindow
}

// UsageWindow describes a single quota window.
type UsageWindow struct {
	Name              string
	UsedPercent       int
	WindowSeconds     int
	ResetAfterSeconds int
	ResetAtUnix       int64
}

// UsageCredits contains optional credit/balance metadata.
type UsageCredits struct {
	HasCredits bool
	Unlimited  bool
	Balance    string
}

// ContextCompressor is an optional interface for LLMs that support
// compressing the conversation context within a running session.
// CompressCommand returns the agent's native slash command (e.g. "/compact");
// return "" if not supported.
type ContextCompressor interface {
	CompressCommand() string
}

// CommandProvider is an optional interface for LLMs that expose custom slash
// commands via local *.md files (e.g. .claude/commands/*.md).
type CommandProvider interface {
	CommandDirs() []string
}

// SkillProvider is an optional interface for LLMs that expose skills via
// local directories (each containing a SKILL.md file).
type SkillProvider interface {
	SkillDirs() []string
}

// SessionDeleter is an optional interface for LLMs that support deleting sessions.
type SessionDeleter interface {
	DeleteSession(ctx context.Context, sessionID string) error
}

// WorkDirSwitcher is an optional interface for LLMs that support runtime
// work directory switching. Takes effect on next session start.
type WorkDirSwitcher interface {
	SetWorkDir(dir string)
	GetWorkDir() string
}

// ModeSwitcher is an optional interface for LLMs that support runtime permission mode switching.
type ModeSwitcher interface {
	SetMode(mode string)
	GetMode() string
	PermissionModes() []PermissionModeInfo
}

// ──────────────────────────────────────────────────────────────
// Skill Management & Evolution
// ──────────────────────────────────────────────────────────────

// SkillManager defines how agent skills are persisted and discovered.
type SkillManager interface {
	Name() string                                                                   // Unique name of the manager implementation
	Type() string                                                                   // Type of the manager (e.g., "skill", "memory")
	Description() string                                                            // Short description of the manager
	List(ctx context.Context) ([]*Skill, error)                                     // List all available skills
	Get(ctx context.Context, name string) (*Skill, error)                           // Get a specific skill by name
	Save(ctx context.Context, s *Skill) error                                       // Create or update a skill
	Delete(ctx context.Context, name string) error                                  // Delete a skill
	Extract(ctx context.Context, llm LLM, history []HistoryEntry) ([]*Skill, error) // Analyze history and propose new skills

	// RecordExecution (Optional) records the outcome of a skill usage.
	RecordExecution(skillName string, score int) error
}

// ──────────────────────────────────────────────────────────────
// Doctor / Diagnostics Types
// ──────────────────────────────────────────────────────────────

// DoctorStatus represents the outcome of a single diagnostic check.
type DoctorStatus int

const (
	DoctorPass DoctorStatus = iota // Check passed
	DoctorWarn                     // Check passed with a warning
	DoctorFail                     // Check failed
)

// Icon returns an emoji indicator for the status.
func (s DoctorStatus) Icon() string {
	switch s {
	case DoctorPass:
		return "✅"
	case DoctorWarn:
		return "⚠️"
	default:
		return "❌"
	}
}

// DoctorCheckResult is the result of a single diagnostic check.
type DoctorCheckResult struct {
	Name    string       // Display name
	Status  DoctorStatus // Pass / Warn / Fail
	Detail  string       // Human-readable detail or error message
	Latency string       // Optional latency string (e.g. "42ms")
}

// DoctorChecker is an optional interface for LLMs that can supply
// custom diagnostic checks.
type DoctorChecker interface {
	DoctorChecks(ctx context.Context) []DoctorCheckResult
}

// AgentDoctorInfo is an optional interface for LLMs that can report
// their binary path and version for diagnostics.
type AgentDoctorInfo interface {
	BinaryPath() string
	Version(ctx context.Context) (string, error)
}

// ──────────────────────────────────────────────────────────────
// Workspace Binding
// ──────────────────────────────────────────────────────────────

// WorkspaceBindingProvider is an optional interface for LLMs that support
// binding a session to a specific workspace directory. Workspace bindings
// persist across restarts and are keyed by session key.
type WorkspaceBindingProvider interface {
	// GetWorkspaceBinding returns the workspace path bound to a session, or "" if none.
	GetWorkspaceBinding(sessionKey string) string
	// SetWorkspaceBinding binds a session key to a workspace path.
	SetWorkspaceBinding(sessionKey, workspacePath string) error
	// ClearWorkspaceBinding removes the binding for a session key.
	ClearWorkspaceBinding(sessionKey string) error
}

// ──────────────────────────────────────────────────────────────
// Interaction Logging (Distributed)
// ──────────────────────────────────────────────────────────────

// StorageAware is an optional interface for Dialog and LLM plugins that need
// persistent storage. The Engine injects a scoped KVStoreProvider during
// plugin registration, giving each plugin its own isolated bucket namespace.
type StorageAware interface {
	SetStorage(store KVStoreProvider)
}

// DialogRecorder is an optional interface for Dialog plugins that can record
// incoming messages in their own format and storage. Each Dialog plugin
// defines its OWN log data structure internally; Core never inspects it.
type DialogRecorder interface {
	RecordMessage(traceID string, msg *Message) error
}

// SessionRecorder is an optional interface for AgentSession implementations
// that can record the LLM's side of an interaction. By calling SetTraceID
// before Send, the session knows which interaction it belongs to and can
// internally record thinking, tool calls, and responses in its own format.
// The session finalizes its log entry when Close() is called.
type SessionRecorder interface {
	SetTraceID(traceID string)
}

// ──────────────────────────────────────────────────────────────
// Web & UI Extension Interfaces
// ──────────────────────────────────────────────────────────────

// WebRoute describes a single frontend route provided by a plugin.
type WebRoute struct {
	Path  string `json:"path"`  // Relative path (e.g., "/memory")
	Label string `json:"label"` // Human-readable menu label
	Icon  string `json:"icon"`  // Lucide icon identifier
	Entry string `json:"entry"` // Path to the frontend entry file (relative to www/)
}

// UIAbility describes the unified web capabilities of a plugin.
type UIAbility struct {
	Routes []WebRoute `json:"routes"` // List of pages provided by the plugin
}

// WebProvider is implemented by plugins that contribute UI pages to the Core Shell.
type WebProvider interface {
	GetUIAbility() UIAbility // Returns metadata for UI discovery
	GetWWWDir() string       // Returns the physical path to the plugin's 'www' directory
}

// Reflector is responsible for post-session evaluation of AI performance.
type Reflector interface {
	// Reflect evaluates a session's outcome and updates skill/agent metrics.
	Reflect(ctx context.Context, sessionKey, traceID string, usedSkills []string) error
}

// ──────────────────────────────────────────────────────────────
// Hook System
// ──────────────────────────────────────────────────────────────

// HookEvent describes a lifecycle hook event fired by the Engine.
type HookEvent struct {
	Name       string         `json:"event"`
	SessionKey string         `json:"session_key,omitempty"`
	UserID     string         `json:"user_id,omitempty"`
	Content    string         `json:"content,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// HookHandler is implemented by pipe plugins that want to receive lifecycle events
// fired from the Engine (e.g., message.sent).
type HookHandler interface {
	FireHook(event HookEvent)
}

// ──────────────────────────────────────────────────────────────
// Outbound Rate Limiting
// ──────────────────────────────────────────────────────────────

// OutboundLimiter controls the rate at which the Engine sends messages to a platform.
// Implemented by the outbound_rl pipe plugin.
type OutboundLimiter interface {
	Wait(ctx context.Context, platform string) error
}

// ──────────────────────────────────────────────────────────────
// Context Assembly (Memory Engine Extension)
// ──────────────────────────────────────────────────────────────

// ContextAssembler is implemented by memory SkillManager plugins.
// Engine calls AssembleContext before each LLM request to get compressed history.
type ContextAssembler interface {
	AssembleContext(sessionKey string, budget int) ([]HistoryEntry, error)
	IngestMessage(sessionKey string, entry HistoryEntry) error
}

// ──────────────────────────────────────────────────────────────
// MCP Tool Provider
// ──────────────────────────────────────────────────────────────

// LLMTool describes a callable tool (Function Call / Tool Use schema).
type LLMTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// MCPToolProvider is implemented by MCP-capable SkillManager plugins.
// Engine collects all tools before building the LLM request.
type MCPToolProvider interface {
	GetTools() []LLMTool
	CallTool(ctx context.Context, name string, args map[string]any) (string, error)
}

// ──────────────────────────────────────────────────────────────
// SubTask / Sub-Agent
// ──────────────────────────────────────────────────────────────

// SubTaskStatus is the result snapshot of a spawned sub-task.
type SubTaskStatus struct {
	ID     string `json:"id"`
	State  string `json:"state"` // "running" | "done" | "error" | "not_found"
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// SubTaskManager is implemented by the subtask SkillManager plugin.
type SubTaskManager interface {
	Spawn(ctx context.Context, sessionKey, prompt string, budget int) (string, error)
	GetStatus(taskID string) SubTaskStatus
}

// ──────────────────────────────────────────────────────────────
// StorageBackend Plugin
// ──────────────────────────────────────────────────────────────

// StorageBackend is a pluggable key-value storage backend.
// The default implementation is BoltDB (built into core).
// Optional implementations: Redis, SQLite (via plugins).
type StorageBackend interface {
	Open(namespace string) (KVStore, error)
	Close() error
}

// StorageFactory creates a StorageBackend from options.
type StorageFactory func(opts map[string]any) (StorageBackend, error)

// ──────────────────────────────────────────────────────────────
// Trigger Plugin
// ──────────────────────────────────────────────────────────────

// TriggerEvent is the event produced by a Trigger plugin and delivered to Engine.
type TriggerEvent struct {
	Source     string         `json:"source"`
	SessionKey string         `json:"session_key,omitempty"`
	Payload    string         `json:"payload"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// Trigger is a plugin type that actively generates events (unlike Pipe which reacts).
// Examples: CronTrigger, MQTTTrigger.
type Trigger interface {
	Start(ctx context.Context, handler func(TriggerEvent)) error
}

// TriggerFactory creates a Trigger from options.
type TriggerFactory func(opts map[string]any) (Trigger, error)

// ──────────────────────────────────────────────────────────────
// Lifecycle (optional interface for all plugin types)
// ──────────────────────────────────────────────────────────────

// LifecycleAware may be implemented by any plugin to participate in
// the Engine's startup/shutdown sequence and health-check loop.
type LifecycleAware interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Health(ctx context.Context) error
}

// ──────────────────────────────────────────────────────────────
// SecretProvider Plugin
// ──────────────────────────────────────────────────────────────

// SecretProvider abstracts the source of secret values (env, Vault, KMS, …).
type SecretProvider interface {
	Get(ctx context.Context, key string) (string, error)
}

// SecretProviderFactory creates a SecretProvider from options.
type SecretProviderFactory func(opts map[string]any) (SecretProvider, error)

// ──────────────────────────────────────────────────────────────
// EventBus Plugin
// ──────────────────────────────────────────────────────────────

// EventSubscription represents an active subscription to a topic.
type EventSubscription interface {
	Close() error
}

// EventBus provides publish-subscribe messaging across plugin boundaries.
type EventBus interface {
	Publish(ctx context.Context, topic string, payload []byte) error
	Subscribe(ctx context.Context, topic string, handler func([]byte)) (EventSubscription, error)
}

// EventBusFactory creates an EventBus from options.
type EventBusFactory func(opts map[string]any) (EventBus, error)

// ──────────────────────────────────────────────────────────────
// PolicyEngine Plugin
// ──────────────────────────────────────────────────────────────

// PolicyDecision is the outcome of a policy evaluation.
type PolicyDecision struct {
	Allow     bool
	Reason    string
	Overrides map[string]any
}

// PolicyEngine evaluates policies (RBAC, guardrail, routing) at runtime.
type PolicyEngine interface {
	Evaluate(ctx context.Context, policy string, input map[string]any) (PolicyDecision, error)
}

// PolicyEngineFactory creates a PolicyEngine from options.
type PolicyEngineFactory func(opts map[string]any) (PolicyEngine, error)

// ──────────────────────────────────────────────────────────────
// Engine Option Pattern
// ──────────────────────────────────────────────────────────────

// EngineOption configures the Engine during construction.
type EngineOption func(*EngineOptions)

// EngineOptions holds all injectable dependencies for the Engine.
type EngineOptions struct {
	Sessions   SessionProvider
	Store      KVStoreProvider
	Storage    StorageBackend
	Translator Translator
	STT        SpeechToText
	TTS        TextToSpeech
	Reflector  Reflector
	EventBus   EventBus
	DataDir    string
}

// WithSessions injects a SessionProvider.
func WithSessions(sp SessionProvider) EngineOption {
	return func(o *EngineOptions) { o.Sessions = sp }
}

// WithStore injects a KVStoreProvider.
func WithStore(kv interface{}) EngineOption {
	return func(o *EngineOptions) {
		if kvp, ok := kv.(KVStoreProvider); ok {
			o.Store = kvp
		}
		if sb, ok := kv.(StorageBackend); ok {
			o.Storage = sb
		}
	}
}

// WithTranslator injects a Translator.
func WithTranslator(t Translator) EngineOption {
	return func(o *EngineOptions) { o.Translator = t }
}

// WithSTT injects a SpeechToText provider.
func WithSTT(stt SpeechToText) EngineOption {
	return func(o *EngineOptions) { o.STT = stt }
}

// WithTTS injects a TextToSpeech provider.
func WithTTS(tts TextToSpeech) EngineOption {
	return func(o *EngineOptions) { o.TTS = tts }
}

// WithReflector injects a Reflector.
func WithReflector(r Reflector) EngineOption {
	return func(o *EngineOptions) { o.Reflector = r }
}

// WithEventBus injects an EventBus.
func WithEventBus(eb EventBus) EngineOption {
	return func(o *EngineOptions) { o.EventBus = eb }
}

// WithDataDir sets the data directory.
func WithDataDir(dir string) EngineOption {
	return func(o *EngineOptions) { o.DataDir = dir }
}

// ──────────────────────────────────────────────────────────────
// Note: SpeechToText and TextToSpeech interfaces are defined in voice.go
