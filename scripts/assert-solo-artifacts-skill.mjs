import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

const root = new URL('../', import.meta.url).pathname;
const skillDir = join(root, 'skills/solo-artifacts');
const oldCodexSkillDir = join(root, '.codex/skills/solo-artifacts');
const read = (path) => readFileSync(join(skillDir, path), 'utf8');
const assert = (condition, message) => {
  if (!condition) throw new Error(message);
};

assert(existsSync(skillDir), 'solo-artifacts skill directory should exist');
assert(!existsSync(oldCodexSkillDir), 'solo-artifacts should live in Solo skills/, not .codex/skills/');
assert(!existsSync(join(skillDir, '.git')), 'solo-artifacts skill should not contain nested git metadata');

const skill = read('SKILL.md');
const starter = read('assets/starter.html');
const css = read('assets/base.css');
const review = read('references/review-decision.md');
const appTheme = readFileSync(join(root, 'frontend/app/globals.brutal.css'), 'utf8');
const archiveTheme = appTheme.match(/:root\[data-skin="archive"\],[\s\S]*?\n}/)?.[0];

assert(skill.includes('name: solo-artifacts'), 'SKILL.md should use solo-artifacts as the skill name');
assert(!skill.includes('name: work-canvas'), 'SKILL.md should not keep the old work-canvas name');
assert(skill.includes('Solo') && skill.includes('artifact'), 'SKILL.md should describe Solo artifact usage');
assert(skill.includes('Publish Existing Deliverable'), 'SKILL.md should make existing deliverables the first workflow path');
assert(skill.includes('Deliverable: ./path/to/result.html'), 'SKILL.md should document the deliverable declaration contract');
assert(skill.includes('Do not scan the workspace by newest file'), 'SKILL.md should reject guessing from stale workspace files');
assert(skill.includes('publish it directly'), 'SKILL.md should tell agents to publish existing HTML directly');
assert(starter.includes('solo-artifacts STARTER'), 'starter should identify itself as solo-artifacts');
assert(archiveTheme, 'frontend should declare the default Warm Archive theme');
for (const token of ['canvas', 'surface', 'ink', 'rule', 'subtle-text', 'primary', 'primary-light', 'accent', 'accent-light', 'info', 'info-light', 'success', 'success-light', 'warning', 'warning-light', 'danger', 'danger-light', 'violet', 'violet-light', 'muted', 'muted-light']) {
  const appValue = archiveTheme.match(new RegExp(`--skin-${token}:\\s*([^;]+);`))?.[1].trim();
  assert(appValue, `Warm Archive should declare --skin-${token}`);
  assert(css.includes(`--solo-${token}: ${appValue};`), `base.css should mirror --skin-${token}`);
}
assert(css.includes('--radius: 0.625rem') && css.includes('0 5px 16px rgb(57 52 47 / 12%)'), 'base.css should use Warm Archive radii and soft shadows');
assert(!css.includes('[data-theme="dark"]'), 'base.css should not include a dark theme');
assert(!starter.includes('data-theme') && !starter.includes('data-theme-toggle'), 'starter should not include theme switching');
assert(starter.includes('name="theme-color" content="#f8f5ee"'), 'starter should advertise the Warm Archive browser color');
assert(css.includes('border: 1px solid var(--card-border)'), 'base.css should use the Warm Archive fine rule');
assert(review.includes('Solo Warm Archive') && review.includes('review-decision'), 'review-decision reference should describe the Warm Archive variant');

console.log('solo-artifacts skill source checks passed');
