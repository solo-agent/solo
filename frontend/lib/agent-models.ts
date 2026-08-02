// ============================================================================
// Agent model suggestions — per-runtime known model aliases.
// Shared by the create form (agent-form.tsx) and the runtime editor
// (agent-runtime-tab.tsx). These populate a native datalist rather than
// validating the value, so existing and future model IDs remain editable.
// Empty value = let the CLI pick its default model.
// ============================================================================

export interface ModelPreset {
  value: string;
  label: string;
}

export const MODEL_PRESETS: Record<string, ModelPreset[]> = {
  // Claude Code CLI — short aliases accepted by `claude --model`.
  claude: [
    { value: 'opus', label: 'Opus' },
    { value: 'sonnet', label: 'Sonnet' },
    { value: 'haiku', label: 'Haiku' },
    { value: 'fable', label: 'Fable' },
  ],
  // Codex CLI — model identifiers passed through to the codex thread.
  codex: [
    { value: 'gpt-5.6-sol', label: 'GPT 5.6 Sol' },
    { value: 'gpt-5.6-terra', label: 'GPT 5.6 Terra' },
    { value: 'gpt-5.6-luna', label: 'GPT 5.6 Luna' },
    { value: 'gpt-5.5', label: 'GPT 5.5' },
  ],
};
