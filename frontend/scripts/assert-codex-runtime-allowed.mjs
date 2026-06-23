import fs from 'node:fs';

const source = fs.readFileSync(new URL('../lib/hooks/use-cli-detection.ts', import.meta.url), 'utf8');

if (!source.match(/ALLOWED_RUNTIMES\s*=\s*new Set\([^)]*"codex"/s)) {
  throw new Error('Expected Codex to be included in the frontend runtime allowlist');
}
