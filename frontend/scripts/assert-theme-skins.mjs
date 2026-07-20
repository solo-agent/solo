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
if (themeOptions.length !== 2) throw new Error('Expected exactly two themes');
if (new Set(themeOptions.map(({ id }) => id)).size !== 2) throw new Error('Theme IDs must be unique');
if (resolveThemeId('unknown') !== 'archive') throw new Error('Unknown theme must fall back');
if (getStoredTheme() !== 'archive') throw new Error('Missing storage must fall back');
if (setTheme('classic') !== 'classic') throw new Error('Valid theme should apply');
if (theme.document.documentElement.dataset.skin !== 'classic') throw new Error('Theme must update the root');
if (theme.storage.get('solo.skin') !== 'classic') throw new Error('Theme must persist');
if (!theme.events.includes('solo:theme-change')) throw new Error('Theme switch should notify the UI');

const stored = loadTheme([['solo.skin', 'classic']]);
if (stored.getStoredTheme() !== 'classic') throw new Error('Stored theme should be restored');

const invalid = loadTheme([['solo.skin', 'nope']]);
if (invalid.getStoredTheme() !== 'archive') throw new Error('Invalid storage should fall back');

const blockedRead = loadTheme([], { readError: true });
if (blockedRead.getStoredTheme() !== 'archive') throw new Error('Blocked reads should fall back');

const blockedWrite = loadTheme([], { writeError: true });
if (blockedWrite.setTheme('classic') !== 'classic') throw new Error('Blocked writes should still apply');
if (blockedWrite.document.documentElement.dataset.skin !== 'classic') {
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
  archive: ['Warm Retro', '暖色复古', 'oklch(0.955 0.01 80)', '#d0937f'],
  classic: ['Yellow Neo-Brutalism', '黄色新粗野主义', '#ffd23f', '#ff6b6b'],
};
const i18n = read('lib/i18n.ts');
for (const [id, [englishName, chineseName, primary, accent]] of Object.entries(expectedRefresh)) {
  const blockStart = css.indexOf(`:root[data-skin="${id}"]`);
  const block = css.slice(blockStart, css.indexOf('\n}', blockStart)).toLowerCase();
  if (!block.includes(`--skin-primary: ${primary};`) || !block.includes(`--skin-accent: ${accent};`)) {
    throw new Error(`${id} is missing its approved editor palette`);
  }
  const { labelKey } = themeOptions.find((option) => option.id === id);
  for (const name of [englishName, chineseName]) {
    if (!i18n.includes(`${labelKey}: '${name}'`)) throw new Error(`${id} is missing the name ${name}`);
  }
}

const layout = read('app/layout.tsx');
for (const needle of ['data-skin="archive"', 'suppressHydrationWarning', 'solo.skin']) {
  if (!layout.includes(needle)) throw new Error(`Layout is missing ${needle}`);
}

const bootstrap = layout.match(/const themeScript = `([^`]+)`;/)?.[1];
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

const removedThemeIds = ['blueprint', 'ultraviolet', 'seasalt', 'tomato', 'matcha', 'bubblegum', 'lavender', 'sky'];
for (const id of removedThemeIds) {
  if (source.includes(`id: '${id}'`) || css.includes(`data-skin="${id}"`)) {
    throw new Error(`Removed theme ${id} is still exposed`);
  }
}

const settings = read('app/settings/page.tsx');
for (const needle of ['themeOptions.map', 'aria-pressed', 'data-skin-preview']) {
  if (!settings.includes(needle)) throw new Error(`Settings is missing ${needle}`);
}

const relationshipNode = read('components/relationships/relationship-node.tsx');
const relationshipWorkspace = read('components/relationships/relationship-workspace.tsx');
const thinkingWorkspace = read('components/thinking/thinking-workspace.tsx');
const channelView = read('components/dashboard/channel-view.tsx');
const tabBar = read('components/ui/tab-bar.tsx');
const agentActivity = read('lib/agent-activity.ts');
const taskActionButtons = read('components/tasks/task-action-buttons.tsx');
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
