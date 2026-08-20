import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import ts from 'typescript';

const read = (path) => readFileSync(new URL(`../${path}`, import.meta.url), 'utf8');
const source = read('lib/theme.ts');
const compiled = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
}).outputText;

function loadTheme(initialStorage = [], { readError = false, writeError = false } = {}) {
  const storage = new Map(initialStorage);
  const events = [];
  const localStorage = {
    getItem: (key) => {
      if (readError) throw new Error('storage read blocked');
      return storage.get(key) ?? null;
    },
    setItem: (key, value) => {
      if (writeError) throw new Error('storage write blocked');
      storage.set(key, value);
    },
  };
  const document = { documentElement: { dataset: {} } };
  const sandbox = {
    exports: {},
    localStorage,
    document,
    Event: class Event {
      constructor(type) {
        this.type = type;
      }
    },
    window: {
      localStorage,
      dispatchEvent: (event) => events.push(event.type),
    },
  };

  vm.runInNewContext(compiled, sandbox);
  return { ...sandbox.exports, document, events, storage };
}

const theme = loadTheme();
const { defaultThemeId, getStoredTheme, resolveThemeId, setTheme, themeOptions } = theme;

if (defaultThemeId !== 'archive') throw new Error('Default theme should be archive');
if (themeOptions.length !== 1) throw new Error('Expected exactly one theme');
if (new Set(themeOptions.map(({ id }) => id)).size !== 1) throw new Error('Theme IDs must be unique');
if (resolveThemeId('unknown') !== 'archive') throw new Error('Unknown theme must fall back');
if (getStoredTheme() !== 'archive') throw new Error('Missing storage must fall back');
if (setTheme('archive') !== 'archive') throw new Error('Default theme should apply');
if (theme.document.documentElement.dataset.skin !== 'archive') throw new Error('Theme must update the root');
if (theme.storage.get('solo.skin') !== 'archive') throw new Error('Theme must persist');
if (!theme.events.includes('solo:theme-change')) throw new Error('Theme switch should notify the UI');

const stored = loadTheme([['solo.skin', 'classic']]);
if (stored.getStoredTheme() !== 'archive') throw new Error('Removed theme should fall back');

const invalid = loadTheme([['solo.skin', 'nope']]);
if (invalid.getStoredTheme() !== 'archive') throw new Error('Invalid storage should fall back');

const blockedRead = loadTheme([], { readError: true });
if (blockedRead.getStoredTheme() !== 'archive') throw new Error('Blocked reads should fall back');

const blockedWrite = loadTheme([], { writeError: true });
if (blockedWrite.setTheme('classic') !== 'archive') throw new Error('Removed theme should fall back');
if (blockedWrite.document.documentElement.dataset.skin !== 'archive') {
  throw new Error('Blocked writes should still update the root');
}

const motionSource = read('lib/motion.ts');
const motionCompiled = ts.transpileModule(motionSource, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
}).outputText;

function loadMotion(reduced) {
  const sandbox = {
    exports: {},
    window: { matchMedia: () => ({ matches: reduced }) },
  };
  vm.runInNewContext(motionCompiled, sandbox);
  return sandbox.exports;
}

if (loadMotion(false).motionDuration(420) !== 420) throw new Error('Motion duration should remain enabled by default');
if (loadMotion(true).motionDuration(420) !== 0) throw new Error('Reduced motion should disable scripted movement');
if (loadMotion(true).motionScrollBehavior() !== 'auto') throw new Error('Reduced motion should disable smooth scrolling');

const css = read('app/globals.brutal.css');
if (css.includes('@theme inline')) {
  throw new Error('Theme colors must remain runtime-overridable instead of being inlined');
}
for (const needle of [
  '--motion-duration-fast: 180ms',
  '--motion-duration-slow: 420ms',
  '--motion-ease-standard: cubic-bezier(0.22, 1, 0.36, 1)',
  'translateX(18px)',
]) {
  if (!css.includes(needle)) throw new Error(`Global motion system is missing ${needle}`);
}
const transitionDurationBlock = css.slice(css.indexOf('.duration-100,'), css.indexOf('}', css.indexOf('.duration-100,')));
if (transitionDurationBlock.includes('animation-duration:')) {
  throw new Error('Transition duration utilities must not speed up looping animations');
}
const archiveCss = css.slice(css.indexOf('Warm Archive'));
for (const forbidden of ['animation: none', 'animation-name:', 'transition-duration:']) {
  if (archiveCss.includes(forbidden)) throw new Error(`Archive theme must not override global motion with ${forbidden}`);
}
for (const { id } of themeOptions) {
  if (!css.includes(`data-skin="${id}"`)) throw new Error(`Missing root CSS for ${id}`);
  if (!css.includes(`data-skin-preview="${id}"`)) throw new Error(`Missing preview CSS for ${id}`);
}

const expectedRefresh = {
  archive: ['Warm Editorial', '暖色编辑', '#eee9e1', '#d0937f', 'var(--skin-accent)', '#e3dacc'],
};
const i18n = read('lib/i18n.ts');
for (const [id, [englishName, chineseName, primary, accent, accentInteractive, stone]] of Object.entries(expectedRefresh)) {
  const blockStart = css.indexOf(`:root[data-skin="${id}"]`);
  const block = css.slice(blockStart, css.indexOf('\n}', blockStart)).toLowerCase();
  if (
    !block.includes(`--skin-primary: ${primary};`)
    || !block.includes(`--skin-accent: ${accent};`)
    || !block.includes(`--skin-accent-interactive: ${accentInteractive};`)
    || !block.includes(`--skin-stone: ${stone};`)
  ) {
    throw new Error(`${id} is missing its approved editor palette`);
  }
  const { labelKey } = themeOptions.find((option) => option.id === id);
  for (const name of [englishName, chineseName]) {
    if (!i18n.includes(`${labelKey}: '${name}'`)) throw new Error(`${id} is missing the name ${name}`);
  }
}

for (const token of ['cactus: #bcd1ca', 'heather: #cbcadb', 'sky: #6a9bcc']) {
  if (!css.includes(`--skin-illustration-${token};`)) {
    throw new Error(`Archive illustration palette is missing ${token}`);
  }
}
if (!css.includes('background: var(--skin-accent-interactive);\n  color: var(--skin-accent-foreground);')) {
  throw new Error('Archive primary CTA must use the interactive clay ramp and its foreground');
}
if (!css.includes('--skin-accent-interactive: var(--skin-accent);')) {
  throw new Error('Archive CTA must reuse the original Terra Pink token');
}
if (!css.includes('--skin-accent-foreground: var(--skin-ink);')) {
  throw new Error('Archive CTA must use Solo ink on the original Terra Pink');
}
if (!css.includes('--color-accent: var(--skin-accent-interactive);')) {
  throw new Error('Functional accent must resolve to the accessible interactive clay');
}

const templatesPage = `${read('app/templates/page.tsx')}\n${read('components/templates/lucy-team-composer.tsx')}`;
for (const token of ['bg-warm-stone', 'bg-illustration-cactus', 'bg-illustration-heather', 'bg-illustration-sky']) {
  if (!templatesPage.includes(token)) throw new Error(`Templates should use ${token}`);
}

const messageInput = read('components/dashboard/message-input.tsx');
if (!messageInput.includes("'btn-brutal-primary'")) {
  throw new Error('Message send action must use the primary CTA');
}
if (messageInput.includes("'btn-brutal btn-brutal-success'")) {
  throw new Error('Message send action must not use the semantic success color');
}

if (!css.includes(':root[data-skin="archive"] .mention-highlight {\n  background: var(--skin-muted-light);')) {
  throw new Error('Archive mentions must use the warm gray chip background');
}
if (!css.includes('color: var(--skin-ink);')) {
  throw new Error('Archive mentions must remain readable');
}

const layout = read('app/layout.tsx');
for (const needle of ['data-skin="archive"', 'suppressHydrationWarning', 'solo.skin']) {
  if (!layout.includes(needle)) throw new Error(`Layout is missing ${needle}`);
}

const bootstrap = layout.match(/const bootstrapScript = `([^`]+)`;/)?.[1];
if (!bootstrap) throw new Error('Layout theme bootstrap is missing');

function runBootstrap(stored) {
  const document = { documentElement: { dataset: { skin: 'archive' } } };
  vm.runInNewContext(bootstrap, {
    document,
    localStorage: { getItem: () => stored },
  });
  return document.documentElement.dataset.skin;
}

for (const { id } of themeOptions) {
  if (runBootstrap(id) !== id) throw new Error(`Bootstrap should restore ${id}`);
}
if (runBootstrap('unknown-skin') !== 'archive' || runBootstrap(null) !== 'archive') {
  throw new Error('Bootstrap should normalize missing or invalid storage to archive');
}

const removedThemeIds = ['classic', 'blueprint', 'ultraviolet', 'seasalt', 'tomato', 'matcha', 'bubblegum', 'lavender', 'sky'];
for (const id of removedThemeIds) {
  if (source.includes(`id: '${id}'`) || css.includes(`data-skin="${id}"`)) {
    throw new Error(`Removed theme ${id} is still exposed`);
  }
}

const settings = read('app/settings/page.tsx');
for (const needle of ['themeOptions', 'data-skin-preview']) {
  if (settings.includes(needle)) throw new Error(`Settings should not expose a removed theme selector`);
}

const relationshipNode = read('components/relationships/relationship-node.tsx');
const relationshipWorkspace = read('components/relationships/relationship-workspace.tsx');
const thinkingWorkspace = read('components/thinking/thinking-workspace.tsx');
const channelView = read('components/dashboard/channel-view.tsx');
const tabBar = read('components/ui/tab-bar.tsx');
const agentActivity = read('lib/agent-activity.ts');
const taskActionButtons = read('components/tasks/task-action-buttons.tsx');
const insightDashboard = read('components/dashboard/insight-dashboard.tsx');
for (const needle of [
  ':root[data-skin="archive"] .relationship-agent-node',
  ':root[data-skin="archive"] .relationship-task-card',
  ':root[data-skin="archive"] .selectable-row:hover',
  ':root[data-skin="archive"] .bg-white',
  '.btn-brutal.bg-brutal-white',
  ':root[data-skin="archive"] .channel-hash-icon',
  ':root[data-skin="archive"] .thinking-node-orb',
  ':is(.relationship-flow, .thinking-flow) .react-flow__controls',
  '.task-action-button[data-tone="info"]',
]) {
  if (!css.includes(needle)) throw new Error(`Archive relationship skin is missing ${needle}`);
}
if (!relationshipNode.includes("in_review: 'var(--color-brutal-violet)'")) {
  throw new Error('Mounted tasks must reuse the task board review color');
}
if (!agentActivity.includes("completed: 'var(--color-brutal-success)'")) {
  throw new Error('Agent animation colors must reuse task board status colors');
}
if (!taskActionButtons.includes('data-tone={tone}')) {
  throw new Error('Task action tones must be exposed to archive skin styles');
}
if (/#(?:[0-9a-f]{3}|[0-9a-f]{6})\b/i.test(insightDashboard)) {
  throw new Error('Insight dashboard colors must come from the active skin');
}
for (const color of ['info', 'success', 'accent', 'violet', 'warning', 'muted']) {
  if (!insightDashboard.includes(`var(--color-brutal-${color})`)) {
    throw new Error(`Insight dashboard is missing the shared ${color} color`);
  }
}
for (const source of [relationshipWorkspace, thinkingWorkspace]) {
  if (!source.includes('proOptions={{ hideAttribution: true }}')) {
    throw new Error('React Flow attribution must stay hidden');
  }
}
if (!channelView.includes('key={workspaceView}') || !channelView.includes('animate-fade-in')) {
  throw new Error('Workspace tabs must keep their shared entrance motion');
}
if (!channelView.includes("thinking-${thinking.selectedNodeId ?? 'root'}")) {
  throw new Error('Conversation branches must keep their shared entrance motion');
}
if (!css.includes('0 0 0 6px color-mix(in srgb, var(--skin-accent) 72%, transparent)')) {
  throw new Error('Selected thinking nodes must keep a visible accent ring');
}
if (!tabBar.includes('tab-button') || !css.includes('background-color var(--motion-duration-base) var(--motion-ease-standard)')) {
  throw new Error('Tabs must use the shared active-state motion');
}

console.log('theme skins source check passed');
