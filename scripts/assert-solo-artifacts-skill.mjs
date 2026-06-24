import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

const root = new URL('../', import.meta.url).pathname;
const skillDir = join(root, '.codex/skills/solo-artifacts');
const read = (path) => readFileSync(join(skillDir, path), 'utf8');
const assert = (condition, message) => {
  if (!condition) throw new Error(message);
};

assert(existsSync(skillDir), 'solo-artifacts skill directory should exist');
assert(!existsSync(join(skillDir, '.git')), 'solo-artifacts skill should not contain nested git metadata');

const skill = read('SKILL.md');
const starter = read('assets/starter.html');
const css = read('assets/base.css');
const review = read('references/review-decision.md');

assert(skill.includes('name: solo-artifacts'), 'SKILL.md should use solo-artifacts as the skill name');
assert(!skill.includes('name: work-canvas'), 'SKILL.md should not keep the old work-canvas name');
assert(skill.includes('Solo') && skill.includes('artifact'), 'SKILL.md should describe Solo artifact usage');
assert(starter.includes('solo-artifacts STARTER'), 'starter should identify itself as solo-artifacts');
assert(css.includes('--solo-blue') && css.includes('shadow-brutal'), 'base.css should use Solo brutal tokens');
assert(css.includes('border: 2px solid var(--ink)'), 'base.css should use hard brutal borders');
assert(review.includes('Solo-brutal') && review.includes('review-decision'), 'review-decision reference should describe the Solo-brutal variant');

console.log('solo-artifacts skill source checks passed');
