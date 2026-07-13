import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import ts from 'typescript';

const read = (path) => readFileSync(new URL(`../${path}`, import.meta.url), 'utf8');
const source = read('lib/i18n.ts');
const compiled = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
}).outputText;

function loadI18n(initialStorage = []) {
  const storage = new Map(initialStorage);
  const events = [];
  const sandbox = {
    exports: {},
    localStorage: {
      getItem: (key) => storage.get(key) ?? null,
      setItem: (key, value) => storage.set(key, value),
    },
    window: {
      localStorage: {
        getItem: (key) => storage.get(key) ?? null,
        setItem: (key, value) => storage.set(key, value),
      },
      dispatchEvent: (event) => events.push(event.type),
    },
    document: { documentElement: { lang: '' } },
    Event: class Event {
      constructor(type) {
        this.type = type;
      }
    },
  };

  vm.runInNewContext(compiled, sandbox);
  return { ...sandbox.exports, events, storage };
}

const { getLocale, setLocale, t, events } = loadI18n();

if (getLocale() !== 'en') throw new Error('Default locale should be English');

setLocale('zh-CN');
if (getLocale() !== 'zh-CN') throw new Error('Locale should switch to zh-CN');
if (t('settingsTitle') !== '设置') throw new Error('settingsTitle should be translated');
if (t('threadReplies', { n: 3 }) !== '3 条回复') throw new Error('Chinese replacements should work');
if (!events.includes('solo:locale-change')) throw new Error('Locale switch should notify the UI');

setLocale('nope');
if (getLocale() !== 'zh-CN') throw new Error('Invalid locale should be ignored');

const storedZh = loadI18n([['solo.locale', 'zh-CN']]);
if (storedZh.getLocale() !== 'en') {
  throw new Error('Stored locale must not change the initial render locale');
}
if (!storedZh.initLocaleFromStorage()) throw new Error('Stored locale should apply after hydration');
if (storedZh.getLocale() !== 'zh-CN') throw new Error('Stored locale should switch after hydration');

const expectedChinese = {
  taskFilterAll: '全部任务',
  taskActionAccept: '通过',
  taskActionReject: '驳回',
  taskActionReopen: '重新打开',
  taskArtifactGenerate: '生成产物',
  computersAll: '全部电脑',
  relationshipPageTitle: '关系',
  relationshipCriteriaDelegation: '委派标准',
  relationshipAddAgent: '智能体',
  observabilityGroupWorking: '工作中',
  messageInputHint: '回车发送 · Shift+回车换行 · @ 提及成员 · 拖放文件或 Ctrl+V 粘贴图片',
  agentView: '智能体视图',
  deleted: '已删除',
  sidebarInbox: '收件箱',
  directMessages: '私信',
};

for (const [key, value] of Object.entries(expectedChinese)) {
  if (t(key) !== value) throw new Error(`${key} should translate to ${value}`);
}

const sourceChecks = [
  ['components/dashboard/channel-view.tsx', ["label: 'All tasks'", 'routeTitle="Chat"']],
  ['components/tasks/task-column.tsx', ["label: 'TODO'", "todo: 'TODO'", "done: 'DONE'"]],
  ['components/tasks/task-action-buttons.tsx', ["'Accept'", "'Reject'", "'Reopen'", 'Close Task', 'Closing...']],
  ['lib/utils/task-artifact.ts', ['Generate Artifact', "'Generating'", "'Artifact'"]],
  ['components/computers/computers-left-column.tsx', ['>Computers<', '>All Computers<']],
  ['components/dashboard/thread-panel.tsx', ['No replies yet. Start the discussion.', '>Retry<', 'Priority:', 'Claimer:', 'Not yet claimed']],
  ['components/relationships/relationship-workspace.tsx', ['>Agent<']],
  ['components/relationships/relationship-detail-panel.tsx', ['Delegation Criteria', 'Collaboration Criteria', '★ Weight', '★ Created']],
  ['components/relationships/relationship-node.tsx', ['>ONLINE<', '>OFFLINE<']],
  ['components/relationships/relationship-edge.tsx', ['Assigns To', 'relType.replace']],
  ['components/relationships/create-relationship-modal.tsx', ['Delegation Criteria', 'Collaboration Criteria', 'Select agent...', 'Swap from and to agents']],
  ['components/dashboard/message-input.tsx', ['Type a message', 'Enter to send', 'Create Task', 'exceeds the 50MB limit']],
  ['components/dashboard/agent-message.tsx', ['>Agent<', '>DELETED<', 'REPLIES']],
  ['components/dashboard/dm-list.tsx', ['>Agent<', '>DELETED<']],
  ['components/dashboard/mention-dropdown.tsx', ['>Agent<']],
  ['components/dashboard/agent-view-panel.tsx', ['Agent View']],
  ['components/dashboard/sidebar.tsx', ["routeTitle = 'Chat'"]],
  ['components/teams/teams-agent-workspace.tsx', ['>Retry<', 'Agent workspace has no files yet', 'Files will appear here after running agent tasks']],
  ['components/inbox/inbox-badge.tsx', ['>Inbox<']],
  ['components/dashboard/channel-list.tsx', ['>Channels<', '>No channels yet<', '>Create Channel<']],
  ['components/dashboard/dm-list.tsx', ['>Direct Messages<']],
  ['components/dashboard/message-input.tsx', ['aria-label="Message mode"', '>Message<', '>Task<']],
  ['components/dashboard/message-list.tsx', ['REPLIES']],
];

for (const [path, needles] of sourceChecks) {
  const file = read(path);
  for (const needle of needles) {
    if (file.includes(needle)) throw new Error(`${path} still contains hard-coded English: ${needle}`);
  }
}

console.log('i18n language source check passed');
