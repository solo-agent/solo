// ============================================================================
// RelationshipDetailPanel — slide-out panel for edge/node click (T5.2.5)
// - Shows relationship details when an edge is clicked
// - Shows agent info when a node is clicked
// - Edit rel_type in-place
// - Delete with confirmation
// - Color-coded rel_type badge
// ============================================================================

'use client';

import { useState, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { X, Trash2, Edit3, Check, AlertTriangle, MessageSquare } from 'lucide-react';
import { Select } from '@/components/ui/select';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { Button, iconActionClass } from '@/components/ui/button';
import {
  detailEditActionClass,
  detailFieldLabelClass,
  detailSectionClass,
  detailSectionTitleClass,
} from '@/components/ui/detail-section';
import { panelHeaderClass, panelTitleClass } from '@/components/ui/panel-header';
import { TeamsAgentProfile } from '@/components/teams/teams-agent-profile';
import { TeamsAgentWorkspace } from '@/components/teams/teams-agent-workspace';
import { useDM } from '@/lib/hooks/use-dm';
import { apiClient, ApiError } from '@/lib/api-client';
import { t } from '@/lib/i18n';
import type { RelationshipType, Agent, AgentRelationship } from '@/lib/types';

// ---- Edge style config ----

const EDGE_COLORS: Record<RelationshipType, { stroke: string; bg: string }> = {
  assigns_to:        { stroke: '#4A90D9', bg: '#E8F0FD' },
  collaborates_with: { stroke: '#10B981', bg: '#E6F7F0' },
};

const REL_TYPE_OPTIONS = [
  { value: 'assigns_to', label: 'Assigns To' },
  { value: 'collaborates_with', label: 'Collaborates With' },
];

// ---- Component ----

interface RelationshipDetailPanelProps {
  /** The relationship to display (null when showing a node) */
  relationship: AgentRelationship | null;
  /** The agent to display (null when showing a relationship) */
  agent: (Agent & { isActive?: boolean }) | null;
  /** Called to close the panel */
  onClose: () => void;
  /** Called after successful update */
  onUpdate: () => void;
  /** Called after successful delete */
  onDelete: (id: string) => void;
  /** Called after an agent is deleted from the embedded profile */
  onAgentDeleted?: (id: string) => void;
}

export function RelationshipDetailPanel({
  relationship,
  agent,
  onClose,
  onUpdate,
  onDelete,
  onAgentDeleted,
}: RelationshipDetailPanelProps) {
  const router = useRouter();
  const { createOrGetDM } = useDM();
  const [isEditing, setIsEditing] = useState(false);
  const [agentTab, setAgentTab] = useState<'profile' | 'workspace'>('profile');
  const [panelWidth, setPanelWidth] = useState(400);
  const [hasUserResizedPanel, setHasUserResizedPanel] = useState(false);
  const [editType, setEditType] = useState<RelationshipType>(
    relationship?.rel_type ?? 'assigns_to',
  );
  const [isEditingInstruction, setIsEditingInstruction] = useState(false);
  const [editInstruction, setEditInstruction] = useState('');
  const [isSaving, setIsSaving] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [isOpeningDM, setIsOpeningDM] = useState(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // ---- Edit handler ----

  const handleSave = useCallback(async () => {
    if (!relationship) return;
    setIsSaving(true);
    setError(null);
    try {
      await apiClient.patch(`/api/v1/agent-relationships/${relationship.id}`, {
        rel_type: editType,
      });
      setIsEditing(false);
      onUpdate();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to update relationship');
    } finally {
      setIsSaving(false);
    }
  }, [relationship, editType, onUpdate]);

  const handleSaveInstruction = useCallback(async () => {
    if (!relationship) return;
    setIsSaving(true);
    setError(null);
    try {
      await apiClient.patch(`/api/v1/agent-relationships/${relationship.id}`, {
        instruction: editInstruction,
      });
      setIsEditingInstruction(false);
      onUpdate();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to update instruction');
    } finally {
      setIsSaving(false);
    }
  }, [relationship, editInstruction, onUpdate]);

  // ---- Delete handler ----

  const handleDelete = useCallback(async () => {
    if (!relationship) return;
    setIsDeleting(true);
    setError(null);
    try {
      await apiClient.delete(`/api/v1/agent-relationships/${relationship.id}`);
      onDelete(relationship.id);
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to delete relationship');
      setIsDeleting(false);
    }
  }, [relationship, onDelete, onClose]);

  const handleMessageAgent = useCallback(async () => {
    if (!agent) return;
    setIsOpeningDM(true);
    setError(null);
    try {
      const dm = await createOrGetDM({ agent_id: agent.id });
      router.push(`/dashboard?dm=${dm.id}`);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to open message');
    } finally {
      setIsOpeningDM(false);
    }
  }, [agent, createOrGetDM, router]);

  // ---- Render agent detail ----

  if (agent) {
    const isActive = agent.isActive ?? (agent.is_active ?? false);
    return (
      <div
        className="fixed right-0 top-14 h-[calc(100%-3.5rem)] border-l-4 border-black bg-white shadow-brutal-2xl z-40 flex flex-col animate-slide-in-from-right"
        style={{ width: panelWidth }}
      >
        <div
          className="absolute left-0 top-0 bottom-0 w-1.5 cursor-col-resize hover:bg-brutal-primary/50 transition-colors z-10"
          onMouseDown={(e) => {
            e.preventDefault();
            const startX = e.clientX;
            const startWidth = panelWidth;
            const onMove = (ev: MouseEvent) => {
              setHasUserResizedPanel(true);
              setPanelWidth(Math.max(280, Math.min(800, startWidth + startX - ev.clientX)));
            };
            const onUp = () => {
              document.removeEventListener('mousemove', onMove);
              document.removeEventListener('mouseup', onUp);
            };
            document.addEventListener('mousemove', onMove);
            document.addEventListener('mouseup', onUp);
          }}
        />
        {/* Header */}
        <div className={panelHeaderClass()}>
          <h3 className={panelTitleClass()}>
            {t('agentDetailTitle')}
          </h3>
          <button
            type="button"
            onClick={onClose}
            className={iconActionClass()}
            aria-label={t('close')}
          >
            <X className="h-3.5 w-3.5" />
          </button>
        </div>

        <div className="flex items-center gap-3 border-b-2 border-black bg-white px-4 py-3">
          <PixelAvatar agentId={agent.id} avatarUrl={agent.avatar_url} size="md" />
          <div className="min-w-0 flex-1">
            <div className="truncate font-heading text-base font-bold text-black">
              {agent.name}
            </div>
            <div className="font-mono text-[10px] font-bold uppercase tracking-wider">
              {isActive ? (
                <span className="text-brutal-success">ONLINE</span>
              ) : (
                <span className="text-brutal-muted">OFFLINE</span>
              )}
            </div>
          </div>
          <Button
            type="button"
            onClick={handleMessageAgent}
            disabled={isOpeningDM}
            variant="outline"
            size="sm"
            className="flex-shrink-0 gap-1.5 px-2.5 text-[10px] font-black uppercase tracking-wider"
            aria-label={`${t('teamsMessage')} ${agent.name}`}
          >
            <MessageSquare className="h-3.5 w-3.5" />
            <span>{t('teamsMessage')}</span>
          </Button>
        </div>

        <div className="grid grid-cols-2 border-b-2 border-black">
          <button
            type="button"
            onClick={() => setAgentTab('profile')}
            className={[
              'border-r-2 border-black px-3 py-2 font-heading text-xs font-bold uppercase tracking-wider',
              agentTab === 'profile' ? 'bg-brutal-primary text-black' : 'bg-white hover:bg-brutal-primary-light',
            ].join(' ')}
          >
            Profile
          </button>
          <button
            type="button"
            onClick={() => {
              setAgentTab('workspace');
              if (!hasUserResizedPanel) setPanelWidth((width) => Math.max(width, 720));
            }}
            className={[
              'px-3 py-2 font-heading text-xs font-bold uppercase tracking-wider',
              agentTab === 'workspace' ? 'bg-brutal-primary text-black' : 'bg-white hover:bg-brutal-primary-light',
            ].join(' ')}
          >
            Workspace
          </button>
        </div>

        <div className="flex-1 overflow-hidden">
          {agentTab === 'profile' ? (
            <TeamsAgentProfile
              agentId={agent.id}
              redirectAfterDelete={false}
              showProfileHeader={false}
              onAgentDeleted={(deletedId) => {
                onAgentDeleted?.(deletedId);
                onClose();
              }}
            />
          ) : (
            <TeamsAgentWorkspace agentId={agent.id} />
          )}
        </div>
      </div>
    );
  }

  // ---- Render relationship detail ----

  if (!relationship) return null;

  const colors = EDGE_COLORS[relationship.rel_type] || EDGE_COLORS.collaborates_with;

  return (
    <div
      className="fixed right-0 top-14 h-[calc(100%-3.5rem)] border-l-4 border-black bg-white shadow-brutal-2xl z-40 flex flex-col animate-slide-in-from-right"
      style={{ width: panelWidth }}
    >
      <div
        className="absolute left-0 top-0 bottom-0 w-1.5 cursor-col-resize hover:bg-brutal-primary/50 transition-colors z-10"
        onMouseDown={(e) => {
          e.preventDefault();
          const startX = e.clientX;
          const startWidth = panelWidth;
          const onMove = (ev: MouseEvent) => {
            setPanelWidth(Math.max(280, Math.min(800, startWidth + startX - ev.clientX)));
          };
          const onUp = () => {
            document.removeEventListener('mousemove', onMove);
            document.removeEventListener('mouseup', onUp);
          };
          document.addEventListener('mousemove', onMove);
          document.addEventListener('mouseup', onUp);
        }}
      />
      {/* Header */}
      <div className={panelHeaderClass()}>
        <h3 className={panelTitleClass()}>
          {t('relationshipEditorEdgeDetail')}
        </h3>
        <button
          type="button"
          onClick={onClose}
          className={iconActionClass()}
          aria-label={t('close')}
        >
          <X className="h-3.5 w-3.5" />
        </button>
      </div>

      <div className="border-b-2 border-black bg-white px-4 py-3">
        <div
          className="mb-3 inline-flex items-center gap-1.5 px-3 py-1.5 border-2 border-black bg-brutal-primary font-heading text-xs font-black uppercase tracking-wider shadow-brutal-sm"
        >
          <svg width="16" height="8">
            <line x1="0" y1="4" x2="16" y2="4"
              stroke={colors.stroke}
              strokeWidth={2}
              strokeDasharray={relationship.rel_type === 'collaborates_with' ? '6,3' : 'none'}
            />
          </svg>
          {relationship.rel_type.replace(/_/g, ' ')}
        </div>

        <div className="grid grid-cols-[1fr_auto_1fr] items-center gap-3">
          <div className="min-w-0">
            <div className="font-heading text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
              {t('relationshipEditorFrom')}
            </div>
            <div className="truncate font-heading text-base font-bold text-black">
              {relationship.from_agent_name || relationship.from_agent_id.slice(0, 8)}
            </div>
            {relationship.from_agent_active !== undefined && (
              <span className={[
                'font-mono text-[9px] font-bold uppercase tracking-wider',
                relationship.from_agent_active ? 'text-brutal-success' : 'text-brutal-muted',
              ].join(' ')}>
                {relationship.from_agent_active ? 'ONLINE' : 'OFFLINE'}
              </span>
            )}
          </div>
          <svg width="30" height="24" viewBox="0 0 30 24" fill="none" stroke={colors.stroke} strokeWidth="2.5">
            <path d="M4 12h20M17 5l7 7-7 7" />
          </svg>
          <div className="min-w-0 text-right">
            <div className="font-heading text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
              {t('relationshipEditorTo')}
            </div>
            <div className="truncate font-heading text-base font-bold text-black">
              {relationship.to_agent_name || relationship.to_agent_id.slice(0, 8)}
            </div>
            {relationship.to_agent_active !== undefined && (
              <span className={[
                'font-mono text-[9px] font-bold uppercase tracking-wider',
                relationship.to_agent_active ? 'text-brutal-success' : 'text-brutal-muted',
              ].join(' ')}>
                {relationship.to_agent_active ? 'ONLINE' : 'OFFLINE'}
              </span>
            )}
          </div>
        </div>
      </div>

      {/* Body */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {/* Instruction */}
        <div className={detailSectionClass()}>
          <div className="flex items-center justify-between mb-2">
            <div className={detailSectionTitleClass()}>
              ★ {relationship.rel_type === 'assigns_to' ? 'Delegation Criteria' : 'Collaboration Criteria'}
            </div>
            {!isEditingInstruction ? (
              <button
                type="button"
                onClick={() => { setIsEditingInstruction(true); setEditInstruction(relationship.instruction || ''); }}
                className={detailEditActionClass()}
              >
                <Edit3 className="h-3 w-3" />
                {t('edit')}
              </button>
            ) : null}
          </div>

          {isEditingInstruction ? (
            <div className="space-y-2">
              <textarea
                value={editInstruction}
                onChange={(e) => setEditInstruction(e.target.value)}
                placeholder={relationship.rel_type === 'assigns_to'
                  ? 'Delegate coding tasks with: clear requirement description...\n\nReport back with: implementation status, files changed...'
                  : 'Coordinate on: API contract sync, shared component design...\n\nKeep in sync: interface definitions, breaking changes...'
                }
                className="w-full min-h-[80px] px-3 py-2 border-2 border-black font-mono text-xs resize-y bg-white"
                rows={4}
              />
              <div className="flex items-center gap-2">
                <Button
                  type="button"
                  onClick={handleSaveInstruction}
                  disabled={isSaving}
                  variant="success"
                  size="sm"
                  className="gap-1 text-[10px] uppercase tracking-wider"
                >
                  {isSaving ? (
                    <span>{t('saving')}</span>
                  ) : (
                    <>
                      <Check className="h-3 w-3" />
                      {t('save')}
                    </>
                  )}
                </Button>
                <Button
                  type="button"
                  onClick={() => setIsEditingInstruction(false)}
                  variant="outline"
                  size="sm"
                  className="gap-1 text-[10px] uppercase tracking-wider"
                >
                  {t('cancel')}
                </Button>
              </div>
            </div>
          ) : (
            <div className="font-body text-sm text-black whitespace-pre-wrap leading-relaxed">
              {relationship.instruction || (
                <span className="text-muted-foreground italic">
                  {relationship.rel_type === 'assigns_to'
                    ? 'No delegation criteria set.'
                    : 'No collaboration criteria set.'}
                </span>
              )}
            </div>
          )}
        </div>

        {/* Type edit (in-place) */}
        <div className={detailSectionClass()}>
          <div className="flex items-center justify-between mb-2">
            <div className={detailSectionTitleClass()}>
              ★ {t('relationshipEditorType')}
            </div>
            {!isEditing ? (
              <button
                type="button"
                onClick={() => { setIsEditing(true); setEditType(relationship.rel_type); }}
                className={detailEditActionClass()}
              >
                <Edit3 className="h-3 w-3" />
                {t('edit')}
              </button>
            ) : null}
          </div>

          {isEditing ? (
            <div className="space-y-2">
              <Select
                options={REL_TYPE_OPTIONS}
                value={editType}
                onChange={(v) => setEditType(v as RelationshipType)}
                size="md"
                className="w-full"
              />
              <div className="flex items-center gap-2">
                <Button
                  type="button"
                  onClick={handleSave}
                  disabled={isSaving}
                  variant="success"
                  size="sm"
                  className="gap-1 text-[10px] uppercase tracking-wider"
                >
                  {isSaving ? (
                    <span>{t('saving')}</span>
                  ) : (
                    <>
                      <Check className="h-3 w-3" />
                      {t('save')}
                    </>
                  )}
                </Button>
                <Button
                  type="button"
                  onClick={() => setIsEditing(false)}
                  variant="outline"
                  size="sm"
                  className="gap-1 text-[10px] uppercase tracking-wider"
                >
                  {t('cancel')}
                </Button>
              </div>
            </div>
          ) : (
            <div className="font-mono text-xs text-black">
              {relationship.rel_type.replace(/_/g, ' ')}
            </div>
          )}
        </div>

        {/* Channel info */}
        {relationship.channel_id && (
          <div className={detailSectionClass()}>
            <div className={detailFieldLabelClass('mb-2')}>
              Channel
            </div>
            <div className="font-mono text-xs text-black">
              {relationship.channel_name
                ? `#${relationship.channel_name}`
                : relationship.channel_id.slice(0, 8)}
            </div>
          </div>
        )}

        {/* Weight */}
        {relationship.weight !== undefined && relationship.weight !== null && (
          <div className={detailSectionClass()}>
            <div className={detailSectionTitleClass('mb-2')}>
              ★ Weight
            </div>
            <div className="font-mono text-xs text-black">{relationship.weight}</div>
          </div>
        )}

        {/* Created at */}
        {relationship.created_at && (
          <div className={detailSectionClass()}>
            <div className={detailSectionTitleClass('mb-2')}>
              ★ Created
            </div>
            <div className="font-mono text-xs text-black">
              {new Date(relationship.created_at).toLocaleString()}
            </div>
          </div>
        )}

        {/* Error */}
        {error && (
          <p className="font-mono text-xs text-brutal-danger">{error}</p>
        )}
      </div>

      {/* Delete action */}
      <div className="border-t-2 border-black p-4 bg-brutal-cream">
        {!showDeleteConfirm ? (
          <Button
            type="button"
            onClick={() => setShowDeleteConfirm(true)}
            variant="danger"
            className="w-full justify-center"
          >
            <Trash2 className="mr-2 h-4 w-4" />
            {t('relationshipEditorDelete')}
          </Button>
        ) : (
          <div className="space-y-2">
            <div className="flex items-start gap-2 px-3 py-2 border-2 border-brutal-danger bg-brutal-danger-light">
              <AlertTriangle className="h-4 w-4 flex-shrink-0 mt-0.5 text-brutal-danger" />
              <p className="font-heading text-[10px] font-bold uppercase tracking-wider text-brutal-danger">
                {t('relationshipEditorDeleteConfirm')}
              </p>
            </div>
            <div className="flex items-center gap-2">
              <Button
                type="button"
                onClick={handleDelete}
                disabled={isDeleting}
                variant="danger"
                size="sm"
                className="flex-1 text-[10px] uppercase tracking-wider"
              >
                {isDeleting ? t('deleting') : t('confirm')}
              </Button>
              <Button
                type="button"
                onClick={() => setShowDeleteConfirm(false)}
                variant="outline"
                size="sm"
                className="flex-1 text-[10px] uppercase tracking-wider"
              >
                {t('cancel')}
              </Button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
