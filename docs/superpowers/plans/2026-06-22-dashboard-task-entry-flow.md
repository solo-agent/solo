# Dashboard Task Entry Flow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Dashboard the primary place to start tracked agent work, while Tasks stays focused on tracking.

**Architecture:** Reuse the existing `MessageInput` task mode and existing `ThreadPanel`. Do not add a new task creation route, model, or dialog. After task-mode send, synthesize/open the existing thread panel around the created message id and let existing task refetch logic fill in task metadata.

**Tech Stack:** Next.js App Router, React client components, TypeScript, existing Solo hooks/components.

## Global Constraints

- Dashboard is the work entry point.
- Message is the default mode.
- Task is an explicit mode inside the Dashboard input.
- Tasks is a tracking board.
- No new backend task model.
- No new `/tasks/new` experience.
- No full task creation form.
- No Jira-like fields.
- No visual restyling beyond copy and state needed for the flow.

---

### Task 1: Clarify Dashboard Input Modes

**Files:**
- Modify: `frontend/components/dashboard/message-input.tsx`

**Interfaces:**
- Consumes: existing `MessageInputProps.onSend(content, mentionedAgentIds, asTask, taskTitle, attachmentIds)`
- Produces: same props and behavior; only user-facing copy changes.

- [ ] **Step 1: Update task-mode toggle copy**

In `frontend/components/dashboard/message-input.tsx`, replace the toggle label block:

```tsx
{asTask ? 'Cancel Task' : 'Create as Task'}
```

with:

```tsx
{asTask ? 'Message Mode' : 'Track as Task'}
```

- [ ] **Step 2: Update helper text**

Replace the task helper text:

```tsx
'Enter to create task · Title can be empty (first 100 chars of message) · Toggle to cancel task mode'
```

with:

```tsx
'Enter to create tracked work · stays linked to this conversation'
```

- [ ] **Step 3: Verify**

Run:

```bash
npm run build
```

from `frontend/`.

Expected: build succeeds.

- [ ] **Step 4: Commit**

```bash
git add frontend/components/dashboard/message-input.tsx
git commit -m "Clarify dashboard task input mode"
```

---

### Task 2: Open Thread Panel After Dashboard Task Creation

**Files:**
- Modify: `frontend/components/dashboard/channel-view.tsx`
- Modify: `frontend/components/dashboard/dm-view.tsx`

**Interfaces:**
- Consumes: `MessageInput.onSend` result `{ id: string; task_number?: number } | null`
- Produces: existing `threadMessage` / `threadTask` state is opened after task-mode send.

- [ ] **Step 1: Update channel task send path**

In `frontend/components/dashboard/channel-view.tsx`, inside the `MessageInput` `onSend` handler, replace the `if (asTask)` branch:

```tsx
if (asTask) {
  const result = await sendMessage(content, _mentionedAgentIds, true, attachmentIds);
  if (result && result.task_number !== undefined) {
    showToast(t('taskCreatedToast', { n: result.task_number }), 'success');
  }
}
```

with:

```tsx
if (asTask) {
  const result = await sendMessage(content, _mentionedAgentIds, true, attachmentIds);
  if (result && result.task_number !== undefined) {
    showToast(t('taskCreatedToast', { n: result.task_number }), 'success');
    const parentMessage: Message = {
      id: result.id,
      channel_id: channel.id,
      user_id: 'user-1',
      display_name: t('selfRef'),
      content,
      created_at: new Date().toISOString(),
      status: 'sent',
      sender_type: 'user',
      task_number: result.task_number,
    };
    setThreadMessage(parentMessage);
    setThreadTask(null);
    onThreadChange?.(result.id);
    refetchTasks();
  }
}
```

- [ ] **Step 2: Update DM task send path**

In `frontend/components/dashboard/dm-view.tsx`, replace the task send handler:

```tsx
const result = await sendMessage(content, mentionedAgentIds, asTask, attachmentIds);
if (asTask && result?.task_number) onTaskCreated?.();
return result;
```

with:

```tsx
const result = await sendMessage(content, mentionedAgentIds, asTask, attachmentIds);
if (asTask && result?.task_number) {
  onTaskCreated?.();
  const parentMessage: Message = {
    id: result.id,
    channel_id: dm.id,
    user_id: 'user-1',
    display_name: t('selfRef'),
    content,
    created_at: new Date().toISOString(),
    status: 'sent',
    sender_type: 'user',
    task_number: result.task_number,
  };
  setThreadMessage(parentMessage);
  setThreadTask(null);
  onThreadChange?.(result.id);
  refetchTasks?.();
}
return result;
```

- [ ] **Step 3: Verify**

Run:

```bash
npm run build
```

from `frontend/`.

Expected: build succeeds.

- [ ] **Step 4: Commit**

```bash
git add frontend/components/dashboard/channel-view.tsx frontend/components/dashboard/dm-view.tsx
git commit -m "Open task thread after dashboard task creation"
```

---

### Task 3: Make Tasks Page Read As Tracking

**Files:**
- Modify: `frontend/app/tasks/page.tsx`

**Interfaces:**
- Consumes: existing `EmptyState`
- Produces: Tasks empty states point users back to Dashboard instead of implying Tasks is the creation entry.

- [ ] **Step 1: Add dashboard navigation helper**

In `TasksPageContent`, add this callback near `handleClearFilters`:

```tsx
const handleGoToDashboard = useCallback(() => {
  router.push('/dashboard');
}, [router]);
```

- [ ] **Step 2: Update no-tasks empty state**

Replace:

```tsx
<EmptyState
  variant="dashed"
  icon={<Plus className="h-6 w-6 text-muted-foreground" />}
  title={t('noTasks')}
/>
```

with:

```tsx
<EmptyState
  variant="dashed"
  icon={<Plus className="h-6 w-6 text-muted-foreground" />}
  title={t('noTasks')}
  actionLabel={t('navChannels')}
  onAction={handleGoToDashboard}
/>
```

- [ ] **Step 3: Verify**

Run:

```bash
npm run build
```

from `frontend/`.

Expected: build succeeds.

- [ ] **Step 4: Commit**

```bash
git add frontend/app/tasks/page.tsx
git commit -m "Guide empty task board back to dashboard"
```

---

## Final Verification

- [ ] Run `npm run build` from `frontend/`.
- [ ] In Dashboard channel: switch input to task mode, create task, confirm right thread panel opens.
- [ ] In Dashboard DM: switch input to task mode, create task, confirm right thread panel opens when task creation returns a task number.
- [ ] In Tasks with no tasks: confirm empty state routes to Dashboard.
