// Synthetic tool flows write in the Agent workspace and call the real Solo API.
// Scope these native settings to test Agents; retain Codex's approval policy.
export const codexE2EArgs = [
  '-c', 'features.plugins=false',
  '-c', 'model_reasoning_effort=low',
  '-c', 'sandbox_mode="workspace-write"',
  '-c', 'sandbox_workspace_write.network_access=true',
];
