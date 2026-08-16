import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { isPersonalArea, isPersonalRouteActive } from '../lib/personal-navigation.ts';

assert.equal(isPersonalArea('/home'), true);
assert.equal(isPersonalArea('/computers'), true);
assert.equal(isPersonalArea('/settings/profile'), true);
assert.equal(isPersonalArea('/dashboard'), false);
assert.equal(isPersonalRouteActive('/computers/device-1', '/computers'), true);
assert.equal(isPersonalRouteActive('/settings', '/computers'), false);

const workspaceRail = readFileSync(new URL('../components/workspaces/workspace-rail.tsx', import.meta.url), 'utf8');
assert.doesNotMatch(workspaceRail, /window\.prompt/);
assert.match(workspaceRail, /<Dialog open=\{createOpen\}/);

const appFrame = readFileSync(new URL('../components/layout/app-frame.tsx', import.meta.url), 'utf8');
assert.match(appFrame, /<WorkspaceRail \/>/);
assert.match(appFrame, /<WorkspaceManageDialog \/>/);
