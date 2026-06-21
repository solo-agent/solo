// Lightweight i18n for Solo frontend. English is the default language.
// All user-facing strings are defined here. To add a language, add a new
// object with the same shape and set `currentLocale` at runtime.
//
// Usage: import { t } from '@/lib/i18n'; t('save')

const en = {
  // ── Common ──
  loading: 'Loading...',
  save: 'Save',
  saving: 'Saving...',
  cancel: 'Cancel',
  delete: 'Delete',
  deleting: 'Deleting...',
  retry: 'Retry',
  close: 'Close',
  edit: 'Edit',
  remove: 'Remove',
  submit: 'Submit',
  submitting: 'Submitting...',
  confirm: 'Confirm',
  search: 'Search',
  clear: 'Clear',
  all: 'All',
  none: 'None',
  unknown: 'Unknown',
  never: 'Never',
  justNow: 'just now',
  noResults: 'No results found',
  back: 'Back',
  create: 'Create',
  refresh: 'Refresh',
  online: 'Online',
  offline: 'Offline',
  enabled: 'Enabled',
  disabled: 'Disabled',
  user: 'User',
  agent: 'Agent',

  // ── Relative Time ──
  minutesAgo: '{n} min ago',
  hoursAgo: '{n} hr ago',
  daysAgo: '{n} day ago',
  minutes: '{n} min',
  hours: '{n} hr',
  days: '{n} day',

  // ── Layout / Nav / App Shell ──
  appTitle: 'Solo — Channel-based Multi-Agent Collaboration Platform',
  appDescription: 'A collaboration space for humans and AI, organized like a team chat',
  navChannels: 'Channels',
  navTasks: 'Task Board',
  navTeams: 'Teams',
  navComputers: 'Computer Management',
  navWorkspace: 'Workspace',
  navSettings: 'Settings',
  navSoloWorkspace: 'Solo Workspace',
  navCollapseComputers: 'Expand or collapse computer list',
  navCollapseChannels: 'Expand or collapse channels',
  navCollapseDMs: 'Expand or collapse direct messages',

  // ── Channel Management ──
  deleteChannelTitle: 'Delete Channel',
  deleteChannelDesc: 'Are you sure you want to delete #{name}? All messages in the channel will be permanently removed.',
  createChannelTitle: 'Create Channel',
  createChannelDesc: 'Channels are collaboration spaces around specific topics. Use a suitable name like general, random, or project-alpha.',
  channelNameLabel: 'Name',
  channelNamePlaceholder: 'e.g. project-alpha',
  channelNameRequired: 'Channel name is required',
  channelNameMaxLen: 'Channel name cannot exceed 80 characters',
  channelNamePattern: 'Channel name can only contain lowercase letters, numbers, underscores, and hyphens',
  channelDescLabel: 'Description (optional)',
  channelDescPlaceholder: 'What is this channel about?',
  channelDescMaxLen: 'Description cannot exceed 200 characters',
  creating: 'Creating...',
  noChannelsYet: 'No channels yet',
  closeChannel: 'Close {name}',
  createChannel: 'Create Channel',

  // ── Mention ──
  mentionSelect: 'Select member to mention',
  noMatchingMembers: 'No matching members',

  // ── Thread ──
  thread: 'Thread',
  threadReplies: '{n} replies',
  closeThreadPanel: 'Close thread panel',
  noRepliesYet: 'No replies yet. Start the discussion.',
  threadReplyPlaceholder: 'Reply to thread... (type @ to mention)',
  threadReplyInput: 'Thread reply input',
  sendReply: 'Send reply',
  notYetClaimed: 'Not yet claimed',
  claim: 'Claim',
  release: 'Release',
  priorityLabel: 'Priority',
  claimerLabel: 'Claimer',

  // ── Upload ──
  dragDropHint: 'Drop files here to upload',
  maxFileSize: 'Max 50MB',
  uploadPreview: 'Upload file preview',
  uploadedDone: 'Upload complete',
  uploadFailed: 'Upload failed',
  removeUpload: 'Remove {filename}',
  fileSizeExceeded: '{name} exceeds the 50MB limit',

  // ── Input ──
  createAsTask: 'Create as task',
  cancelAsTask: 'Cancel task',
  taskDescriptionInput: 'Task description input',
  messageInput: 'Message input',
  sendMessage: 'Send message',
  messagePlaceholder: 'Type a message... (Enter to send, Shift+Enter for new line)',
  taskMessagePlaceholder: 'Task description (optional)... (Enter to create, Shift+Enter for new line)',

  // ── Teams Workspace ──
  agentWorkspaceNoFiles: 'Agent workspace has no files yet',
  agentWorkspaceHint: 'Files will appear here after running agent tasks',
  selectFileHint: 'Select a file to preview its content',

  // ── Teams Items ──
  viewAgentDetail: 'View {name} details',
  viewUserDetail: 'View {name} details',

  // ── Teams Sidebar ──
  expandOrCollapseAgents: 'Expand or collapse Agents',
  expandOrCollapseHumans: 'Expand or collapse Humans',
  noAgentsHint: 'No agents yet',
  noHumansHint: 'No humans yet',

  // ── Teams Human Profile ──
  teamsUserNotFound: 'This user does not exist or is currently unavailable',
  registeredOn: 'Registered on {date}',

  // ── Message Loading ──
  messageLoadError: 'Could not load messages',
  earlierMessageLoadError: 'Could not load earlier messages',

  // ── Global Errors ──
  somethingWentWrong: 'Something went wrong',
  unexpectedError: 'An unexpected error occurred. Please try again.',
  pageNotFound: 'Page not found',
  pageNotFoundDesc: 'The page you are looking for does not exist or has been removed.',
  backToDashboard: 'Back to Dashboard',
  backToLogin: 'Back to Login',

  // ── Auth ──
  welcomeBack: 'Welcome back',
  loginToSolo: 'Log in to your Solo account',
  email: 'Email',
  password: 'Password',
  enterPassword: 'Enter password',
  loggingIn: 'Logging in...',
  login: 'Log in',
  noAccount: "Don't have an account?",
  register: 'Register',
  createAccount: 'Create account',
  registerToSolo: 'Register to start collaborating on Solo',
  confirmPassword: 'Confirm password',
  enterPasswordAgain: 'Enter password again',
  registering: 'Registering...',
  hasAccount: 'Already have an account?',
  checkingAuth: 'Checking login status...',
  emailRequired: 'Email is required',
  emailInvalid: 'Please enter a valid email',
  passwordRequired: 'Password is required',
  passwordMinLength: 'Password must be at least 8 characters',
  confirmPasswordRequired: 'Please confirm your password',
  passwordsMismatch: 'Passwords do not match',
  displayName: 'Display name',
  displayNamePlaceholder: 'Your name',
  displayNameRequired: 'Display name is required',
  displayNameMaxLength: 'Display name cannot exceed 50 characters',

  // ── Dashboard ──
  dashboardLoadError: 'Dashboard failed to load',
  dashboardErrorDesc: 'Dashboard encountered an error. Please retry or return home.',
  noChannelsOrDMs: 'No channels or DMs yet',
  createChannelPrompt: 'Create a channel to start collaborating with your team.',
  selectChannelPrompt: 'Select a channel or DM from the sidebar to start chatting.',
  newChannel: 'New Channel',
  messages: 'Messages',
  tasks: 'Tasks',
  channelMembers: 'Channel Members',
  addAgentToChannel: 'Add agent to channel',
  searchAgent: 'Search agents...',
  noMatchingAgents: 'No matching agents',
  noAgentsAvailable: 'No agents available to add',
  allAgentsInChannel: 'All agents are already in this channel',
  adding: 'Adding...',
  add: 'Add',
  noMembersYet: 'No members yet',

  // ── Messages ──
  messageList: 'Message list',
  noMessages: 'No messages yet. Send the first message to start the discussion.',
  sendFailed: 'Send failed',
  sending: 'Sending...',
  loadEarlierMessages: 'Load earlier messages',
  beginningOfChannel: 'This is the beginning of the channel',
  loadError: 'Load failed',
  editMessage: 'Edit message',
  saveMessage: 'Save',
  savingMessage: 'Saving...',
  editingMessage: 'Editing...',
  deleteMessage: 'Delete message',
  deleteMessageTitle: 'Delete message?',
  deleteMessageConfirm: "Delete {name}'s message? This action cannot be undone.",
  replyToMessage: 'Reply to {name}',
  convertToTask: 'Convert to task',
  scrollToLatest: 'Back to latest',
  keyboardShortcutHint: 'Hover then press E to edit · Delete to remove',
  closeShortcutHint: 'Close keyboard shortcut hint',

  // ── Streaming ──
  streaming: 'Streaming...',

  // ── Threads ──
  unreadThreadReply: 'Unread thread reply, click to view',
  unreadReply: 'Unread reply',

  // ── DM ──
  noDMsYet: 'No DMs yet',
  startDM: 'Start DM',
  dmWith: 'DM with {name}',
  systemMessageDM: 'This is your DM with {name}',
  agentDeletedDM: 'This agent has been removed. Messages cannot be sent.',
  sendMessageTo: 'Send message to {name}...',
  closeDM: 'Close DM',
  dmSearch: 'Search users or agents...',
  userAgentList: 'User and agent list',
  noMatchingUsers: 'No matching users or agents',
  noUsersAvailable: 'No users or agents available to message',
  alreadyHaveDM: '· Already have a DM',
  untitledConversation: 'Untitled Conversation',
  createDMError: 'Could not start conversation. Please try again.',

  // ── Tasks ──
  taskCreatedToast: 'Task #{n} created',
  taskConverted: 'Converted to task #{n}',
  taskConvertError: 'Could not convert to task. Please try again.',
  taskStatusUpdated: 'Task status updated: {status}',
  taskNotFound: 'Task not found',
  noTasks: 'No tasks yet',
  noTasksInChannel: 'No tasks in this channel',
  noTasksInDM: 'No tasks in this DM',
  noTasksMatchingFilter: 'No tasks match the current filter',
  createTask: 'Create task',
  createTaskTitle: 'Create task',
  createTaskDesc: 'Create a new task and assign it to team members or agents.',
  backToTaskList: 'Back to task list',
  taskTitle: 'Task title',
  taskTitlePlaceholder: 'Enter task title...',
  taskTitleRequired: 'Task title is required',
  taskTitleMaxLen: 'Task title cannot exceed 500 characters',
  taskDesc: 'Description',
  taskDescPlaceholder: 'Describe the task in detail...',
  taskPriority: 'Priority',
  taskAssignee: 'Assignee',
  taskDueDate: 'Due date',
  taskChannel: 'Channel',
  taskChannelPlaceholder: 'Select channel...',
  taskChannelRequired: 'Please select a channel',
  taskRedirecting: 'Redirecting to discussion...',
  filterByClaimer: 'Filter by assignee',
  filterByCreator: 'Filter by creator',
  clearFilter: 'Clear filter',
  allAssignees: 'All assignees',
  allCreators: 'All creators',
  unassigned: 'Unassigned',
  subTask: 'Subtask',
  parentTask: 'Parent task',
  subTaskLabel: 'Subtasks:',
  claimed: 'Claimed',
  unclaimed: 'Unclaimed',
  priorityUrgent: 'Urgent',
  priorityHigh: 'High',
  priorityNormal: 'Normal',
  priorityLow: 'Low',

  // ── Task Status ──
  statusTodo: 'Todo',
  statusInProgress: 'In Progress',
  statusDone: 'Done',
  statusCancelled: 'Cancelled',
  statusProcessing: 'Processing',
  statusPendingReview: 'Pending Review',

  // ── Inbox ──
  inboxTabAll: 'All',
  inboxTabMentions: '@Mentions',
  inboxTabReplies: 'Thread Replies',
  inboxTabDMs: 'DMs',
  markAllRead: 'Mark all read',
  clearInbox: 'Clear',
  filterSender: 'Filter by sender...',
  noNewMessages: 'No new messages',
  inboxEmptyDesc: 'Messages appear here when someone replies in your threads, sends you a DM, or @mentions you.',
  loadMore: 'Load more',
  inboxThreadReply: 'Thread reply',
  inboxDM: 'DM',
  inboxMention: '@Mention',
  inboxDMWith: 'DM with {name}',
  inboxReplyIn: 'Replied in #{channel}',
  inboxMentionIn: 'Mentioned you in #{channel}',
  inboxAriaLabel: 'Inbox, {n} unread',
  inboxUnread: '{n} unread',

  // ── Connection / Network ──
  networkRestored: 'Network restored',
  networkDisconnected: 'Connection lost. Some features may be unavailable.',
  connectionRestored: 'Connection restored',
  reconnecting: 'Reconnecting...',
  connectionLost: 'Connection lost',

  // ── Search ──
  globalSearch: 'Global search',
  searchPlaceholder: 'Search all messages...',
  searchKeyword: 'Search keyword',
  searchLoading: 'Searching...',
  searchEmpty: 'Enter a keyword to search all messages.',
  searchNoResults: 'No matching messages found.',
  searchResults: 'Search results',
  searchError: 'Search failed',
  searchResultsCount: '{n} results',
  today: 'Today',
  yesterday: 'Yesterday',
  jumpToMessage: 'Jump to {name}\'s message',

  // ── Channel Search ──
  channelSearch: 'Search in #{channel}',
  channelSearchPlaceholder: 'Search in #{channel}...',
  channelSearchClose: 'Close search',

  // ── Member Status ──
  thinking: 'thinking...',
  typing: 'typing...',

  // ── Computers ──
  computersLoading: 'Loading...',
  computersNoComputers: 'No computers connected',
  computersNoComputersDesc: 'Computers will appear here after you start the Daemon and register.',
  computersViewGuide: 'View setup guide',
  computersSelectOne: 'Select a computer from the sidebar',
  computersAddComputer: 'Add Computer',
  computersAddTitle: 'Add Computer',
  computersAddDesc: 'Start the Daemon on the target machine and register it with the Solo server.',
  computersSteps: 'Steps',
  computersStepClone: 'Clone the project on the target machine',
  computersStepEnv: 'Set DAEMON_PORT and SERVER_URL in .env',
  computersStepStart: 'Run the Daemon',
  computersStepAuto: 'The Daemon will auto-register with the server',
  computersStepDone: 'The computer will appear in the list once registered',
  computersGotIt: 'Got it',
  computersRemoveTitle: 'Remove Computer',
  computersRemoveConfirm: 'Remove {name}? This action cannot be undone. The computer will be deregistered.',
  computersRemoving: 'Removing...',
  computersConfirmRemove: 'Confirm Remove',
  computersNameUpdated: 'Name updated',
  computersNameUpdateError: 'Failed to update name',
  computersRemoved: 'Computer removed',
  computersRemoveError: 'Failed to remove computer',
  computersLastHeartbeat: 'Last heartbeat',
  computersRegistered: 'Registered',
  computersNoAgentBound: 'No agent bound',
  computersSystemInfo: 'System Info',
  computersOS: 'OS',
  computersHostname: 'Hostname',
  computersIP: 'IP Address',
  computersBasicInfo: 'Basic Info',
  computersName: 'Name',
  computersSaveName: 'Save name',
  computersCancelEdit: 'Cancel edit',
  computersEditName: 'Edit name',
  computersStatus: 'Status',
  computersCurrent: 'Current',
  computersIdle: 'Idle',
  computersRunning: 'Running',
  computersConnectedAgents: 'Connected Agents',
  computersNoConnectedAgents: 'No connected agents yet',
  computersAgentCount: '{n} agents connected to this computer',
  computersActiveTasks: 'Active Tasks',
  computersExpandCard: 'Expand card to view',

  // ── Settings ──
  settingsTitle: 'Settings',
  settingsDesc: 'Manage your personal info',
  settingsEmailUnmodifiable: 'Email cannot be changed',
  settingsDisplayName: 'Display name',
  settingsDisplayNamePlaceholder: 'Enter your display name',
  settingsDisplayNameHint: 'Display name is set during registration and cannot be changed',
  settingsSave: 'Save',
  settingsSaving: 'Saving...',
  settingsLoadError: 'Could not load user info',
  settingsLogout: 'Log Out',
  settingsLoggingOut: 'Logging out...',

  // ── Teams ──
  teamsLoading: 'Loading...',
  teamsNoAgents: 'No agents yet',
  teamsNoAgentsDesc: 'Create an agent first, then return to the Teams page.',
  teamsCreateAgent: 'Create Agent',
  teamsAgentCreated: 'Agent created',
  teamsAgentCreateError: 'Failed to create agent. Please try again.',
  teamsMessage: 'Message',
  teamsJumping: 'Jumping...',

  // ── Agent Island / Activity ──
  agentIdle: 'Idle',
  agentThinkingShort: 'Thinking',
  agentExecuting: 'Executing',
  agentGenerating: 'Generating',
  agentWaitingApproval: 'Awaiting Approval',
  agentErrored: 'Error',
  agentRealTimeStatus: 'Agent real-time status',
  agentClickForDetail: 'Agent {name} {status}, click for detail',
  agentClearAll: 'Clear all',
  agentCollapse: 'Collapse',
  agentViewTrace: 'View {name} full trace',
  agentNoActive: 'No agents currently active',
  agentPanelEmptyDesc: 'Agent thinking and output will appear here during execution.',
  agentWaiting: 'Waiting...',
  agentDone: 'Done',

  // ── Agent Run Status ──
  runCompleted: 'Completed',
  runFailed: 'Failed',
  runInterrupted: 'Interrupted',
  runTimeout: 'Timeout',
  runCancelled: 'Cancelled',

  // ── Agent Form ──
  agentFormName: 'Name *',
  agentFormNamePlaceholder: 'e.g. Code Reviewer',
  agentFormDesc: 'Description',
  agentFormDescPlaceholder: 'Brief description of the agent\'s role',
  agentFormNameRequired: 'Name is required',
  agentFormNameMaxLen: 'Name cannot exceed 50 characters',
  agentFormDescMaxLen: 'Description cannot exceed 200 characters',
  agentFormRuntimeRequired: 'Please select a Runtime',
  agentFormSelectRuntime: 'Select Runtime...',
  agentFormNotInstalled: '{name} — not installed',
  agentFormRoleTemplate: 'Role Template (optional)',
  agentFormSystemPrompt: 'System Prompt',
  agentFormSystemPromptPlaceholder: 'Define the agent\'s behavior and role...',
  agentFormSystemPromptHelp: 'Defines how the agent behaves and responds. The agent will follow these instructions when replying to messages.',
  agentFormTemplateWarning: 'You have unsaved changes in the current System Prompt. Switching the template will replace your content. Continue?',
  agentFormEnv: 'Environment Variables (optional)',
  agentFormEnvHelp: 'Inject environment variables into the agent runtime, such as API keys and configuration parameters.',
  agentFormCustomArgs: 'Custom Arguments (optional)',
  agentFormCustomArgsHelp: 'Extra arguments passed to the CLI. Add each argument as a separate tag.',
  agentFormSubmit: 'Create Agent',
  agentFormSubmitting: 'Submitting...',
  agentFormModel: 'Model Name',
  agentFormModelPlaceholder: 'e.g. gpt-4o',
  agentFormLabelModel: 'Model',

  // ── Agent Profile Tab ──
  agentProfileTitle: 'Agent Profile',
  agentProfileError: 'Could not load agent info',
  agentProfileAgentNotFound: 'Agent not found',
  agentProfileUpdateSuccess: 'Update successful',
  agentProfileNotSet: 'Not set',
  agentProfileEdit: 'Edit {label}',
  agentProfileEnabled: 'Enabled',
  agentProfileDisabled: 'Disabled',
  agentProfileDisable: 'Disable Agent',
  agentProfileEnable: 'Enable Agent',
  agentProfileNoRuntime: 'No runtime configured',
  agentProfileCreatedAt: 'Created',
  agentProfileCreatedBy: 'Created by',
  agentProfileMeta: 'Meta',
  agentProfileInfo: 'Info',
  agentDeleteButton: 'Delete Agent',
  agentDeleteTitle: 'Delete Agent',
  agentDeleteDesc: 'Delete "{name}"? DM history will be retained with a DELETED indicator. This action cannot be undone.',
  agentDeleteSuccess: 'Agent deleted',
  agentDeleteError: 'Failed to delete agent',

  // ── Agent Skills Tab ──
  agentSkillCatalog: 'Skill Catalog',
  agentSkillGlobal: 'Global',
  agentSkillWorkspace: 'Workspace',
  agentSkillNoGlobal: 'No global skills',
  agentSkillNoWorkspace: 'No workspace skills',
  agentSkillEmpty: 'No skills discovered yet',
  agentSkillEmptyHint: 'Ensure your skill directories contain valid SKILL.md files.',
  agentSkillLoadError: 'Could not load skills',

  // ── Agent Runtime Tab ──
  agentRuntimeError: 'Could not load runtime config',
  agentRuntimeType: 'Runtime Type',
  agentRuntimeModelConfig: 'Model Config',
  agentRuntimeEnvVars: 'Environment Variables',
  agentRuntimeConfig: 'Runtime Config',
  agentRuntimeNotConfigured: 'Not configured',
  agentRuntimeDefault: 'Default',
  agentRuntimeNoEnvVars: 'No environment variables configured',
  agentRuntimeSave: 'Save',
  agentRuntimeSaving: 'Saving...',
  agentRuntimeEdit: 'Edit',

  // ── Agent History Tab ──
  agentHistoryTitle: 'Execution History',
  agentHistoryTitleCount: 'Execution History ({n})',
  agentHistoryNoRecords: 'No execution records yet',
  agentHistoryNoRecordsDesc: '@Mention the agent in a channel or add it to a channel to trigger execution.',
  agentHistoryStatusSuccess: 'Success',
  agentHistoryStatusFailed: 'Failed',
  agentHistoryStatusRunning: 'Running',

  // ── Agent Env Editor ──
  agentEnvKey: 'Env var name #{n}',
  agentEnvValue: 'Env var value #{n}',
  agentEnvRemove: 'Remove env var #{n}',

  // ── Agent Args Editor ──
  agentArgsPlaceholder: 'Type argument and press Enter...',
  agentArgsMorePlaceholder: 'Keep adding...',
  agentArgsAriaLabel: 'Custom argument input',
  agentArgsHelp: 'Type each argument and press Enter to add. Example: --thinking-budget value',
  agentArgsRemove: 'Remove argument {tag}',

  // ── Agent CLI Detection ──
  cliChecking: 'Checking CLI installation...',
  cliCheckFailed: 'Could not detect CLI status',

  // ── Agent Detail Panel ──
  agentDetailPanel: 'Agent detail panel',
  agentDetailPanelClose: 'Close agent detail panel',
  agentDetailTitle: 'Agent Detail',

  // ── Relationship Editor ──
  assignsTo: 'Assigns To',
  collaboratesWith: 'Collaborates With',
  relationshipEditor: 'Relationship Editor',
  relationshipEditorLoading: 'Loading editor...',
  relationshipEditorEmpty: 'Drag agents onto the canvas to start building the relationship graph.',
  relationshipEditorCreateRelationship: 'Create Relationship',
  relationshipEditorSelectType: 'Select relationship type',
  relationshipEditorDelete: 'Delete relationship',
  relationshipEditorDeleteConfirm: 'Remove this relationship?',
  relationshipEditorDeleteEdge: 'Remove',
  relationshipEditorEdgeDetail: 'Relationship detail',
  relationshipEditorFrom: 'From',
  relationshipEditorTo: 'To',
  relationshipEditorType: 'Type',
  relationshipEditorChannel: 'Channel *',
  relationshipEditorChannelPlaceholder: 'Select channel...',
  relationshipEditorAutoLayout: 'Auto Layout',
  relationshipEditorUndo: 'Undo',
  relationshipEditorRedo: 'Redo',
  relationshipEditorNodeUnconnected: 'Unconnected',
  relationshipEditorDragHint: 'Drag from node handle to create relationship',

  // ── Workspace ──
  workspaceExpandAgents: 'Expand or collapse agents',
  workspaceNoAgents: 'No agents',
  workspaceSelectAgent: 'Select an agent from the sidebar to browse its workspace files.',
  workspaceSelectAgentPlaceholder: 'Select an agent...',
  workspaceRefreshTree: 'Refresh file tree',
  workspaceNoFiles: 'No files in workspace yet',
  workspaceNoFilesDesc: 'Files will appear here after running agent tasks.',
  workspaceFiles: 'Files',
  workspaceOpenNewTab: 'Open in new tab',
  workspaceSelectFile: 'Select a file to preview its content.',
  workspaceLoadError: 'Could not load file content',
  workspaceLoadFilesError: 'Could not load workspace files',

  // ── Attachments ──
  attachmentPreview: 'Preview image: {filename}',
  attachmentClosePreview: 'Close preview',
  attachmentImageError: 'Image failed to load',
  attachmentViewFull: 'View full image: {filename}',
  attachmentDownload: 'Download {filename}',
  attachmentCount: '{n} attachments',

  // ── Status Indicator ──
  statusOnline: 'Online',
  statusThinking: 'Thinking',
  statusStreaming: 'Streaming',
  statusOffline: 'Offline',

  // ── Toast ──
  toastDismiss: 'Dismiss notification',

  // ── Dialog ──
  dialogClose: 'Close',

  // ── Spinner ──
  spinnerLoading: 'Loading',

  // ── Error Boundaries ──
  errorBoundaryRetry: 'Retry',

  // ── API Errors ──
  apiNetworkError: 'Network error. Please check your connection and try again.',
  apiAuthExpired: 'Session expired. Please log in again.',
  apiBadRequest: 'Invalid request parameters.',
  apiUnauthorized: 'Not logged in or session expired.',
  apiForbidden: 'You do not have permission to perform this action.',
  apiNotFound: 'The requested resource does not exist.',
  apiConflict: 'Resource conflict.',
  apiTooManyRequests: 'Too many requests. Please try again later.',
  apiInternalError: 'Internal server error. Please try again later.',
  apiBadGateway: 'Gateway error.',
  apiServiceUnavailable: 'Service temporarily unavailable. Please try again later.',
  apiDefaultError: 'Request failed. Please try again later ({n}).',

  // ── Auth Context ──
  authInitError: 'Authentication initialization failed',
  authLoginError: 'Login failed. Please try again later.',
  authRegisterError: 'Registration failed. Please try again later.',
  authNameUpdated: 'Display name updated',
  authUpdateError: 'Update failed. Please try again later.',

  // ── Channel / Message Errors ──
  channelSendError: 'Cannot send message: no channel specified.',
  messageSendError: 'Message send failed.',
  channelLoadError: 'Could not load channel list.',
  selfRef: 'Me',
  enterExisting: 'Enter',
  dmLoadError: 'Could not load DM list.',
  dmMessageLoadError: 'Could not load messages.',
  dmEarlierMessageError: 'Could not load earlier messages.',
  memberLoadError: 'Could not load member list.',
  taskLoadError: 'Could not load task list.',
  taskDetailLoadError: 'Could not load task info.',
  userLoadError: 'Could not load user info.',
  agentListLoadError: 'Could not load agent list.',
  computerListLoadError: 'Could not load computer list.',
  agentLoadError: 'Could not load agent list.',
  workspaceFileLoadError: 'Could not load workspace files.',
  cliDetectionError: 'CLI detection failed.',
  backendMetaLoadError: 'Could not load backend metadata.',

  // ── Member List ──
  members: 'Members',
  noMembers: 'No members',
  membersUsers: 'Users ({n})',
  removeAgent: 'Remove agent',

  // ── Channel Tasks ──
  channelTasks: '#{channel} tasks',
  dmTasks: 'Tasks with {name}',

  // ── Gentoken ──
  gentokenDefaultName: 'Test User',
} as const;

type TranslationKey = keyof typeof en;
type Replacements = Record<string, string | number>;

const translations: Record<string, typeof en> = { en };

let locale = 'en';

export function setLocale(l: string) {
  if (translations[l]) locale = l;
}

export function t(key: TranslationKey, replacements?: Replacements): string {
  let text: string = translations[locale]?.[key] ?? en[key] ?? key;
  if (replacements) {
    for (const [k, v] of Object.entries(replacements)) {
      text = text.replace(`{${k}}`, String(v));
    }
  }
  return text;
}

export type { TranslationKey };
