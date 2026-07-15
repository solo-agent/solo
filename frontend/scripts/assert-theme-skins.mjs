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

if (defaultThemeId !== 'classic') throw new Error('Default theme should be classic');
if (themeOptions.length !== 9) throw new Error('Expected exactly nine themes');
if (new Set(themeOptions.map(({ id }) => id)).size !== 9) throw new Error('Theme IDs must be unique');
if (resolveThemeId('unknown') !== 'classic') throw new Error('Unknown theme must fall back');
if (getStoredTheme() !== 'classic') throw new Error('Missing storage must fall back');
if (setTheme('seasalt') !== 'seasalt') throw new Error('Valid theme should apply');
if (theme.document.documentElement.dataset.skin !== 'seasalt') throw new Error('Theme must update the root');
if (theme.storage.get('solo.skin') !== 'seasalt') throw new Error('Theme must persist');
if (!theme.events.includes('solo:theme-change')) throw new Error('Theme switch should notify the UI');

const stored = loadTheme([['solo.skin', 'blueprint']]);
if (stored.getStoredTheme() !== 'blueprint') throw new Error('Stored theme should be restored');

const invalid = loadTheme([['solo.skin', 'nope']]);
if (invalid.getStoredTheme() !== 'classic') throw new Error('Invalid storage should fall back');

const blockedRead = loadTheme([], { readError: true });
if (blockedRead.getStoredTheme() !== 'classic') throw new Error('Blocked reads should fall back');

const blockedWrite = loadTheme([], { writeError: true });
if (blockedWrite.setTheme('tomato') !== 'tomato') throw new Error('Blocked writes should still apply');
if (blockedWrite.document.documentElement.dataset.skin !== 'tomato') {
  throw new Error('Blocked writes should still update the root');
}

const css = read('app/globals.brutal.css');
if (css.includes('@theme inline')) {
  throw new Error('Theme colors must remain runtime-overridable instead of being inlined');
}
for (const { id } of themeOptions) {
  if (!css.includes(`data-skin="${id}"`)) throw new Error(`Missing root CSS for ${id}`);
  if (!css.includes(`data-skin-preview="${id}"`)) throw new Error(`Missing preview CSS for ${id}`);
}

const expectedRefresh = {
  blueprint: ['Light Modern', '明亮现代', '#74b4ee', '#c88bdd'],
  ultraviolet: ['GitHub Light', 'GitHub 浅色', '#54aeff', '#d2a8ff'],
  seasalt: ['Quiet Light', '静谧浅色', '#c4b7d7', '#91b3e0'],
  tomato: ['Solarized Light', 'Solarized 浅色', '#73b8e6', '#e2c66f'],
  matcha: ['Ayu Light', 'Ayu 浅色', '#74baf0', '#c6a1e4'],
  bubblegum: ['Catppuccin Latte', 'Catppuccin 拿铁', '#89b4fa', '#cba6f7'],
  lavender: ['Rosé Pine Dawn', 'Rosé Pine 黎明', '#9ccfd8', '#c4a7e7'],
  sky: ['Gruvbox Light', 'Gruvbox 浅色', '#83a598', '#d3869b'],
};
const i18n = read('lib/i18n.ts');
for (const [id, [englishName, chineseName, primary, accent]] of Object.entries(expectedRefresh)) {
  const blockStart = css.indexOf(`:root[data-skin="${id}"]`);
  const block = css.slice(blockStart, css.indexOf('\n}', blockStart)).toLowerCase();
  if (!block.includes(primary) || !block.includes(accent)) {
    throw new Error(`${id} is missing its approved editor palette`);
  }
  const { labelKey } = themeOptions.find((option) => option.id === id);
  for (const name of [englishName, chineseName]) {
    if (!i18n.includes(`${labelKey}: '${name}'`)) throw new Error(`${id} is missing the name ${name}`);
  }
}

const layout = read('app/layout.tsx');
for (const needle of ['data-skin="classic"', 'suppressHydrationWarning', 'solo.skin']) {
  if (!layout.includes(needle)) throw new Error(`Layout is missing ${needle}`);
}

const bootstrap = layout.match(/const themeScript = `([^`]+)`;/)?.[1];
if (!bootstrap) throw new Error('Layout theme bootstrap is missing');

function runBootstrap(stored) {
  const document = { documentElement: { dataset: { skin: 'classic' } } };
  vm.runInNewContext(bootstrap, {
    document,
    localStorage: { getItem: () => stored },
  });
  return document.documentElement.dataset.skin;
}

for (const { id } of themeOptions) {
  if (runBootstrap(id) !== id) throw new Error(`Bootstrap should restore ${id}`);
}
if (runBootstrap('unknown-skin') !== 'classic') {
  throw new Error('Bootstrap should normalize invalid storage to classic');
}

const settings = read('app/settings/page.tsx');
for (const needle of ['themeOptions.map', 'aria-pressed', 'data-skin-preview']) {
  if (!settings.includes(needle)) throw new Error(`Settings is missing ${needle}`);
}

console.log('theme skins source check passed');
