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
}

// SessionProvider provides session management and retrieval.
type SessionProvider interface {
	// GetOrCreateActive returns the active session for a given user key, creating one if none exists.
	GetOrCreateActive(key string) Session
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
	Name string
	Desc string
}

// ProviderConfig holds API provider settings for an LLM.
type ProviderConfig struct {
	Name     string
	APIKey   string
	BaseURL  string
	Model    string
	Thinking string
	Env      map[string]string
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
