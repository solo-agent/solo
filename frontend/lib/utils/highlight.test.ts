import { test } from 'node:test';
import assert from 'node:assert/strict';
import { highlightSpecials } from './highlight.ts';

const validNames = ['pm', 'arc', 'tpm', 'fe', 'rd', 'fredal', 'lucy'];

test('highlights plain @mentions', () => {
  const out = highlightSpecials('hi @pm', validNames);
  assert.equal(out, 'hi <span class="mention-highlight">@pm</span>');
});

test('protects mentions inside fenced code blocks', () => {
  const out = highlightSpecials('```\n@pm\n```', validNames);
  assert.equal(out, '```\n@pm\n```');
});

test('protects mentions inside single-backtick inline code', () => {
  const out = highlightSpecials('try `@pm` here', validNames);
  assert.equal(out, 'try `@pm` here');
});

test('highlights mentions outside inline code in the same line', () => {
  const out = highlightSpecials('try `@pm` but @arc next', validNames);
  assert.equal(
    out,
    'try `@pm` but <span class="mention-highlight">@arc</span> next',
  );
});

test('protects mentions inside double-backtick inline code', () => {
  const out = highlightSpecials('see ``@pm`` here', validNames);
  assert.equal(out, 'see ``@pm`` here');
});

test('reproduces Lucy bug: bullet list with backticked mentions stays untouched', () => {
  const input = [
    '**我主要委托出去的**',
    '- `@pm` —— 任务拆解、项目管理',
    '- `@arc` —— 架构设计',
    '',
    '**通常一起协作的**',
    '- `@tpm` —— 技术 PM',
    '- `@fe` —— 前端',
    '- `@rd` —— 研发 / 后端',
  ].join('\n');
  assert.equal(highlightSpecials(input, validNames), input);
});
