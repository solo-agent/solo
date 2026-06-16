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
import { X, Trash2, Edit3, Check, AlertTriangle } from 'lucide-react';
import { useRouter } from 'next/navigation';
import { Select } from '@/components/ui/select';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
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
}

export function RelationshipDetailPanel({
  relationship,
  agent,
  onClose,
  onUpdate,
  onDelete,
}: RelationshipDetailPanelProps) {
  const router = useRouter();
  const [isEditing, setIsEditing] = useState(false);
  const [editType, setEditType] = useState<RelationshipType>(
    relationship?.rel_type ?? 'assigns_to',
  );
  const [isEditingInstruction, setIsEditingInstruction] = useState(false);
  const [editInstruction, setEditInstruction] = useState('');
  const [isSaving, setIsSaving] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
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

  // ---- Render agent detail ----

  if (agent) {
    const isActive = agent.isActive ?? (agent.is_active ?? false);
    return (
      <div className="fixed right-0 top-14 h-[calc(100%-3.5rem)] w-80 border-l-4 border-black bg-white shadow-brutal-2xl z-40 flex flex-col animate-slide-in-from-right">
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-3 border-b-2 border-black bg-brutal-cream">
          <h3 className="font-heading text-sm font-black uppercase tracking-wider">
            {t('agentDetailTitle')}
          </h3>
          <button
            type="button"
            onClick={onClose}
            className="flex h-7 w-7 items-center justify-center border-2 border-black bg-white hover:bg-brutal-primary-light active:translate-x-0.5 active:translate-y-0.5 active:shadow-none transition-all"
            aria-label={t('close')}
          >
            <X className="h-3.5 w-3.5" />
          </button>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto p-4 space-y-4">
          {/* Agent name badge */}
          <div className="flex items-center gap-3 p-3 border-2 border-black bg-brutal-cream">
            <PixelAvatar agentId={agent.id} avatarUrl={agent.avatar_url} size="md" />
            <div className="min-w-0">
              <div className="font-heading text-sm font-bold text-black truncate">
                {agent.name}
              </div>
              <div className="font-mono text-[10px] font-bold uppercase tracking-wider mt-0.5">
                {isActive ? (
                  <span className="text-brutal-success">ONLINE</span>
                ) : (
                  <span className="text-brutal-muted">OFFLINE</span>
                )}
              </div>
            </div>
          </div>

          {/* Agent details */}
          {agent.description && (
            <div className="p-3 border-2 border-black bg-white">
              <div className="font-heading text-[10px] font-bold uppercase tracking-wider text-muted-foreground mb-1">
                Description
              </div>
              <p className="font-sans text-sm text-black">{agent.description}</p>
            </div>
          )}

          <div className="p-3 border-2 border-black bg-white">
            <div className="font-heading text-[10px] font-bold uppercase tracking-wider text-muted-foreground mb-1">
              Runtime_Type
            </div>
            <p className="font-mono text-xs text-black">{agent.model_provider || 'Not configured'}</p>
          </div>

          {/* Go to workspace */}
          <button
            type="button"
            onClick={() => router.push(`/teams?agent=${agent.id}&tab=profile`)}
            className="w-full btn-brutal-sm bg-brutal-primary px-4 py-2 font-heading text-xs font-bold uppercase tracking-wider"
          >
            View Profile
          </button>
        </div>
      </div>
    );
  }

  // ---- Render relationship detail ----

  if (!relationship) return null;

  const colors = EDGE_COLORS[relationship.rel_type] || EDGE_COLORS.collaborates_with;

  return (
    <div className="fixed right-0 top-14 h-[calc(100%-3.5rem)] w-80 border-l-4 border-black bg-white shadow-brutal-2xl z-40 flex flex-col animate-slide-in-from-right">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b-2 border-black bg-brutal-cream">
        <h3 className="font-heading text-sm font-black uppercase tracking-wider">
          {t('relationshipEditorEdgeDetail')}
        </h3>
        <button
          type="button"
          onClick={onClose}
          className="flex h-7 w-7 items-center justify-center border-2 border-black bg-white hover:bg-brutal-primary-light active:translate-x-0.5 active:translate-y-0.5 active:shadow-none transition-all"
          aria-label={t('close')}
        >
          <X className="h-3.5 w-3.5" />
        </button>
      </div>

      {/* Body */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {/* Type badge */}
        <div
          className="inline-flex items-center gap-1.5 px-3 py-1.5 border-2 border-black font-heading text-xs font-black uppercase tracking-wider"
          style={{ backgroundColor: colors.bg }}
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

        {/* From → To */}
        <div className="p-3 border-2 border-black bg-brutal-cream space-y-2">
          <div>
            <div className="font-heading text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
              {t('relationshipEditorFrom')}
            </div>
            <div className="font-heading text-sm font-bold text-black">
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
          <div className="flex justify-center py-1">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke={colors.stroke} strokeWidth="2.5">
              <path d="M5 12h14M12 5l7 7-7 7" />
            </svg>
          </div>
          <div>
            <div className="font-heading text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
              {t('relationshipEditorTo')}
            </div>
            <div className="font-heading text-sm font-bold text-black">
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

        {/* Instruction */}
        <div className="p-3 border-2 border-black bg-white">
          <div className="flex items-center justify-between mb-2">
            <div className="font-heading text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
              {relationship.rel_type === 'assigns_to' ? 'Delegation Criteria' : 'Collaboration Criteria'}
            </div>
            {!isEditingInstruction ? (
              <button
                type="button"
                onClick={() => { setIsEditingInstruction(true); setEditInstruction(relationship.instruction || ''); }}
                className="flex items-center gap-1 font-mono text-[10px] font-bold uppercase tracking-wider text-muted-foreground hover:text-black"
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
                <button
                  type="button"
                  onClick={handleSaveInstruction}
                  disabled={isSaving}
                  className="flex items-center gap-1 px-3 py-1.5 border-2 border-black bg-brutal-success text-black font-heading text-[10px] font-bold uppercase tracking-wider hover:bg-brutal-success-light disabled:opacity-50"
                >
                  {isSaving ? (
                    <span>{t('saving')}</span>
                  ) : (
                    <>
                      <Check className="h-3 w-3" />
                      {t('save')}
                    </>
                  )}
                </button>
                <button
                  type="button"
                  onClick={() => setIsEditingInstruction(false)}
                  className="flex items-center gap-1 px-3 py-1.5 border-2 border-black bg-white font-heading text-[10px] font-bold uppercase tracking-wider hover:bg-brutal-muted-light"
                >
                  {t('cancel')}
                </button>
              </div>
            </div>
          ) : (
            <div className="font-mono text-[11px] text-black whitespace-pre-wrap leading-relaxed">
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
        <div className="p-3 border-2 border-black bg-white">
          <div className="flex items-center justify-between mb-2">
            <div className="font-heading text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
              {t('relationshipEditorType')}
            </div>
            {!isEditing ? (
              <button
                type="button"
                onClick={() => { setIsEditing(true); setEditType(relationship.rel_type); }}
                className="flex items-center gap-1 font-mono text-[10px] font-bold uppercase tracking-wider text-muted-foreground hover:text-black"
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
                <button
                  type="button"
                  onClick={handleSave}
                  disabled={isSaving}
                  className="flex items-center gap-1 px-3 py-1.5 border-2 border-black bg-brutal-success text-black font-heading text-[10px] font-bold uppercase tracking-wider hover:bg-brutal-success-light disabled:opacity-50"
                >
                  {isSaving ? (
                    <span>{t('saving')}</span>
                  ) : (
                    <>
                      <Check className="h-3 w-3" />
                      {t('save')}
                    </>
                  )}
                </button>
                <button
                  type="button"
                  onClick={() => setIsEditing(false)}
                  className="flex items-center gap-1 px-3 py-1.5 border-2 border-black bg-white font-heading text-[10px] font-bold uppercase tracking-wider hover:bg-brutal-muted-light"
                >
                  {t('cancel')}
                </button>
              </div>
            </div>
          ) : (
            <div className="font-mono text-xs font-bold text-black">
              {relationship.rel_type.replace(/_/g, ' ')}
            </div>
          )}
        </div>

        {/* Channel info */}
        {relationship.channel_id && (
          <div className="p-3 border-2 border-black bg-brutal-cream">
            <div className="font-heading text-[10px] font-bold uppercase tracking-wider text-muted-foreground mb-1">
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
          <div className="p-3 border-2 border-black bg-brutal-cream">
            <div className="font-heading text-[10px] font-bold uppercase tracking-wider text-muted-foreground mb-1">
              Weight
            </div>
            <div className="font-mono text-xs text-black">{relationship.weight}</div>
          </div>
        )}

        {/* Created at */}
        {relationship.created_at && (
          <div className="p-3 border-2 border-black bg-brutal-cream">
            <div className="font-heading text-[10px] font-bold uppercase tracking-wider text-muted-foreground mb-1">
              Created
            </div>
            <div className="font-mono text-[10px] text-black">
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
          <button
            type="button"
            onClick={() => setShowDeleteConfirm(true)}
            className="w-full flex items-center justify-center gap-2 px-4 py-2.5 border-2 border-brutal-danger bg-white text-brutal-danger font-heading text-xs font-bold uppercase tracking-wider hover:bg-brutal-danger-light active:translate-x-0.5 active:translate-y-0.5 active:shadow-none transition-all shadow-brutal-sm"
          >
            <Trash2 className="h-3.5 w-3.5" />
            {t('relationshipEditorDelete')}
          </button>
        ) : (
          <div className="space-y-2">
            <div className="flex items-start gap-2 px-3 py-2 border-2 border-brutal-danger bg-brutal-danger-light">
              <AlertTriangle className="h-4 w-4 flex-shrink-0 mt-0.5 text-brutal-danger" />
              <p className="font-heading text-[10px] font-bold uppercase tracking-wider text-brutal-danger">
                {t('relationshipEditorDeleteConfirm')}
              </p>
            </div>
            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={handleDelete}
                disabled={isDeleting}
                className="flex-1 px-3 py-1.5 border-2 border-brutal-danger bg-brutal-danger text-white font-heading text-[10px] font-bold uppercase tracking-wider hover:bg-red-600 disabled:opacity-50"
              >
                {isDeleting ? t('deleting') : t('confirm')}
              </button>
              <button
                type="button"
                onClick={() => setShowDeleteConfirm(false)}
                className="flex-1 px-3 py-1.5 border-2 border-black bg-white font-heading text-[10px] font-bold uppercase tracking-wider hover:bg-brutal-muted-light"
              >
                {t('cancel')}
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
