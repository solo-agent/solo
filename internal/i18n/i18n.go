// Package i18n provides lightweight internationalization for Solo.
// English is the default language. All user-facing strings in the backend
// are defined here as exported constants with semantic names.
//
// To add a new language, create a Language constant set and a constructor
// that populates the same fields.
package i18n

// T holds all user-facing backend strings for a given language.
type T struct {
	// ---- System Messages (broadcast to channels) ----
	SysTaskCreated        string
	SysTaskCreatedFromMsg string
	SysTaskDeleted        string
	SysTaskStatusUpdated  string
	SysTaskUpdated        string

	// ---- Auth ----
	DefaultChannelDesc string
	DefaultDisplayName string

	// ---- Agent Activity Pills ----
	PillThinking    string
	PillGenerating  string
	PillCallingTool string
	PillUsingTool   string
	PillToolFailed  string
	PillToolDone    string
	PillToolResult  string
	PillError       string
}

// English is the default locale.
var English = T{
	SysTaskCreated:        "created",
	SysTaskCreatedFromMsg: "created (from message)",
	SysTaskDeleted:        "deleted",
	SysTaskStatusUpdated:  "status updated to %s",
	SysTaskUpdated:        "updated",

	DefaultChannelDesc: "Welcome to Solo! This is your first channel.",
	DefaultDisplayName: "New User",

	PillThinking:   "thinking...",
	PillGenerating: "generating...",
	PillCallingTool: "%s running",
	PillUsingTool:  "using tool",
	PillToolFailed: "%s failed",
	PillToolDone:   "%s done",
	PillToolResult: "tool result",
	PillError:      "error",
}

// Active is the current locale. Set at startup.
var Active = &English
