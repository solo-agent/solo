// ============================================================================
// Relationships Management Page — full CRUD table with filters
// - Relationship list table with color-coded type badges
// - Filter bar: rel_type dropdown + agent name search
// - Create via CreateRelationshipModal, delete with confirm dialog
// - Loading skeleton, empty state, error state with retry
// ============================================================================

'use client';

import { useState, useMemo, useCallback } from 'react';
import {
  Plus,
  Trash2,
  Search,
  X,
  Link2,
  Loader2,
} from 'lucide-react';
import { NavBar } from '@/components/ui/navbar';
import { Button } from '@/components/ui/button';
import { Select, type SelectOption } from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { EmptyState } from '@/components/ui/empty-state';
import { BrutalAlert } from '@/components/ui/brutal-alert';
import { CreateRelationshipModal } from '@/components/relationships/create-relationship-modal';
import {
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog';
import { useRelationships } from '@/lib/hooks/use-relationships';
import { useAgents } from '@/lib/hooks/use-agents';
import { useChannels } from '@/lib/hooks/use-channels';
import type { RelationshipType, AgentRelationship } from '@/lib/types';
import { t } from '@/lib/i18n';
import { cn } from '@/lib/utils';
import { relativeTime } from '@/lib/utils/time';

// ---- Relationship type visual config ----

const TYPE_STYLES: Record<RelationshipType, { color: string; bg: string; label: string }> = {
  reports_to: { color: '#4A90D9', bg: 'bg-[#4A90D9]', label: 'Reports To' },
  delegates_to: { color: '#7B6CF6', bg: 'bg-[#7B6CF6]', label: 'Delegates To' },
  collaborates_with: { color: '#10B981', bg: 'bg-[#10B981]', label: 'Collaborates' },
  escalates_to: { color: '#EF4444', bg: 'bg-[#EF4444]', label: 'Escalates To' },
};

const TYPE_FILTER_OPTIONS: SelectOption[] = [
  { value: '', label: 'All Types' },
  { value: 'assigns_to', label: 'Assigns To' },
  { value: 'collaborates_with', label: 'Collaborates' },
];

// ---- Badge component ----

function TypeBadge({ type }: { type: RelationshipType }) {
  const style = TYPE_STYLES[type];
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 px-2 py-0.5',
        'font-heading text-[10px] font-bold uppercase tracking-wider',
        'border-2 border-black text-white',
        style.bg,
      )}
    >
      {style.label}
    </span>
  );
}

// ---- Skeleton row ----

function TableSkeletonRow() {
  return (
    <div className="grid grid-cols-12 gap-4 items-center px-4 py-3 border-2 border-black bg-white">
      <div className="col-span-3 flex items-center gap-2">
        <Skeleton className="h-4 w-20 rounded-none" />
      </div>
      <div className="col-span-3">
        <Skeleton className="h-4 w-20 rounded-none" />
      </div>
      <div className="col-span-2">
        <Skeleton className="h-5 w-16 rounded-none" />
      </div>
      <div className="col-span-1.5">
        <Skeleton className="h-4 w-10 rounded-none" />
      </div>
      <div className="col-span-1">
        <Skeleton className="h-4 w-10 rounded-none" />
      </div>
      <div className="col-span-1.5 flex justify-end">
        <Skeleton className="h-7 w-16 rounded-none" />
      </div>
    </div>
  );
}

// ---- Page ----

export default function RelationshipsManagePage() {
  const { agents } = useAgents();
  const { channels } = useChannels();

  // ---- Filters ----
  const [filterType, setFilterType] = useState<string>('');
  const [searchQuery, setSearchQuery] = useState('');

  // Build hook filters — only pass rel_type when a non-empty value is selected
  const hookFilters = useMemo(() => {
    const f: { rel_type?: RelationshipType; agent_id?: string } = {};
    if (filterType) f.rel_type = filterType as RelationshipType;
    // agent_id filter is handled client-side via search
    return f;
  }, [filterType]);

  const {
    relationships,
    isLoading,
    error,
    deleteRelationship,
    refetch,
  } = useRelationships(hookFilters);

  // ---- Client-side search filter ----
  const filteredRelationships = useMemo(() => {
    if (!searchQuery.trim()) return relationships;
    const q = searchQuery.toLowerCase().trim();
    return relationships.filter((r) => {
      const fromName = (r.from_agent_name || r.from_agent_id).toLowerCase();
      const toName = (r.to_agent_name || r.to_agent_id).toLowerCase();
      return fromName.includes(q) || toName.includes(q);
    });
  }, [relationships, searchQuery]);

  // ---- Create modal ----
  const [showCreateModal, setShowCreateModal] = useState(false);

  // ---- Delete confirmation ----
  const [deleteTarget, setDeleteTarget] = useState<AgentRelationship | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);

  const handleDelete = useCallback(async () => {
    if (!deleteTarget) return;
    setIsDeleting(true);
    try {
      await deleteRelationship(deleteTarget.id);
    } catch {
      // error handled by hook
    } finally {
      setIsDeleting(false);
      setDeleteTarget(null);
    }
  }, [deleteTarget, deleteRelationship]);

  // ---- Derived ----
  const isEmpty = !isLoading && !error && relationships.length === 0;
  const isFilteredEmpty =
    !isLoading && !error && relationships.length > 0 && filteredRelationships.length === 0;

  // ---- Render ----
  return (
    <div className="flex h-screen overflow-hidden bg-brutal-cream">
      <NavBar />

      <div className="flex flex-1 flex-col overflow-hidden">
        {/* ---- Top bar ---- */}
        <div className="flex flex-shrink-0 items-center h-14 px-4 border-b-2 border-black bg-brutal-cream gap-3">
          <h1 className="font-heading text-lg font-bold uppercase tracking-wider mr-auto">
            Relationships
          </h1>
          <Button
            variant="primary"
            size="sm"
            onClick={() => setShowCreateModal(true)}
          >
            <Plus className="h-3.5 w-3.5 mr-1" />
            Create
          </Button>
        </div>

        {/* ---- Filter bar ---- */}
        <div className="flex flex-shrink-0 items-center gap-3 px-4 py-3 border-b-2 border-black bg-white">
          <div className="flex items-center gap-1.5">
            <span className="font-heading text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
              Type
            </span>
            <Select
              options={TYPE_FILTER_OPTIONS}
              value={filterType}
              onChange={(v) => {
                setFilterType(v);
                // Reset search when changing type filter
              }}
              placeholder="All Types"
              size="sm"
              className="w-36"
            />
          </div>

          <div className="flex-1 relative max-w-xs">
            <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground pointer-events-none" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search agent name..."
              className={cn(
                'w-full h-8 pl-7 pr-7',
                'border-2 border-black bg-white',
                'font-heading text-xs font-bold',
                'placeholder:text-muted-foreground/50',
                'focus-visible:outline-none focus-visible:bg-brutal-primary-light',
                'rounded-none',
              )}
            />
            {searchQuery && (
              <button
                type="button"
                onClick={() => setSearchQuery('')}
                className="absolute right-2 top-1/2 -translate-y-1/2"
              >
                <X className="h-3 w-3 text-muted-foreground hover:text-black" />
              </button>
            )}
          </div>

          {filteredRelationships.length > 0 && (
            <span className="font-mono text-[10px] text-muted-foreground">
              {filteredRelationships.length} of {relationships.length}
            </span>
          )}
        </div>

        {/* ---- Content area ---- */}
        <div className="flex-1 overflow-y-auto px-4 py-4">
          {/* Error state */}
          {error && (
            <div className="mb-4 space-y-3">
              <BrutalAlert variant="warning" className="p-4">
                {error}
              </BrutalAlert>
              <Button variant="outline" size="sm" onClick={refetch}>
                {t('retry')}
              </Button>
            </div>
          )}

          {/* Loading state */}
          {isLoading && (
            <div className="space-y-1.5">
              {/* Table header (preserved during loading for layout stability) */}
              <div className="grid grid-cols-12 gap-4 items-center px-4 py-2">
                <div className="col-span-3">
                  <span className="font-heading text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                    From Agent
                  </span>
                </div>
                <div className="col-span-3">
                  <span className="font-heading text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                    To Agent
                  </span>
                </div>
                <div className="col-span-2">
                  <span className="font-heading text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                    Type
                  </span>
                </div>
                <div className="col-span-1.5">
                  <span className="font-heading text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                    Channel
                  </span>
                </div>
                <div className="col-span-1">
                  <span className="font-heading text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                    Weight
                  </span>
                </div>
                <div className="col-span-1">
                  <span className="font-heading text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                    Created
                  </span>
                </div>
                <div className="col-span-0.5" />
              </div>
              {[1, 2, 3, 4, 5, 6].map((i) => (
                <TableSkeletonRow key={i} />
              ))}
            </div>
          )}

          {/* Empty state (no relationships at all) */}
          {isEmpty && (
            <div className="mt-16">
              <EmptyState
                variant="plain"
                rotation={-0.5}
                icon={
                  <Link2 className="h-6 w-6" />
                }
                title="No relationships yet"
                description="Create relationships between agents to define reporting lines, delegations, collaborations, and escalation paths."
                actionLabel="Create Relationship"
                onAction={() => setShowCreateModal(true)}
              />
            </div>
          )}

          {/* Filtered empty state (relationships exist, but filters hide them) */}
          {isFilteredEmpty && (
            <div className="mt-16">
              <EmptyState
                variant="dashed"
                rotation={0}
                icon={<Search className="h-6 w-6" />}
                title="No matching relationships"
                description="Try adjusting your type filter or search query."
              />
            </div>
          )}

          {/* Table */}
          {!isLoading && !error && filteredRelationships.length > 0 && (
            <div className="space-y-1.5">
              {/* Table header */}
              <div className="grid grid-cols-12 gap-4 items-center px-4 py-2 border-2 border-black bg-brutal-primary-light">
                <div className="col-span-3">
                  <span className="font-heading text-[11px] font-bold uppercase tracking-wider">
                    From Agent
                  </span>
                </div>
                <div className="col-span-3">
                  <span className="font-heading text-[11px] font-bold uppercase tracking-wider">
                    To Agent
                  </span>
                </div>
                <div className="col-span-2">
                  <span className="font-heading text-[11px] font-bold uppercase tracking-wider">
                    Type
                  </span>
                </div>
                <div className="col-span-1.5">
                  <span className="font-heading text-[11px] font-bold uppercase tracking-wider">
                    Channel
                  </span>
                </div>
                <div className="col-span-1">
                  <span className="font-heading text-[11px] font-bold uppercase tracking-wider">
                    Weight
                  </span>
                </div>
                <div className="col-span-1">
                  <span className="font-heading text-[11px] font-bold uppercase tracking-wider">
                    Created
                  </span>
                </div>
                <div className="col-span-0.5" />
              </div>

              {/* Table rows */}
              {filteredRelationships.map((rel) => {
                const fromName = rel.from_agent_name || rel.from_agent_id.slice(0, 8);
                const toName = rel.to_agent_name || rel.to_agent_id.slice(0, 8);
                const fromActive = rel.from_agent_active;
                const toActive = rel.to_agent_active;

                return (
                  <div
                    key={rel.id}
                    className="grid grid-cols-12 gap-4 items-center px-4 py-2.5 border-2 border-black bg-white hover:bg-brutal-primary-light/50 transition-colors duration-100"
                  >
                    {/* From Agent */}
                    <div className="col-span-3 flex items-center gap-2 min-w-0">
                      <span
                        className={cn(
                          'h-2 w-2 flex-shrink-0 border border-black',
                          fromActive !== false ? 'bg-brutal-success' : 'bg-brutal-muted',
                        )}
                        title={fromActive !== false ? 'Online' : 'Offline'}
                      />
                      <span className="font-heading text-xs font-bold truncate" title={fromName}>
                        {fromName}
                      </span>
                    </div>

                    {/* To Agent */}
                    <div className="col-span-3 flex items-center gap-2 min-w-0">
                      <span
                        className={cn(
                          'h-2 w-2 flex-shrink-0 border border-black',
                          toActive !== false ? 'bg-brutal-success' : 'bg-brutal-muted',
                        )}
                        title={toActive !== false ? 'Online' : 'Offline'}
                      />
                      <span className="font-heading text-xs font-bold truncate" title={toName}>
                        {toName}
                      </span>
                    </div>

                    {/* Type */}
                    <div className="col-span-2">
                      <TypeBadge type={rel.rel_type} />
                    </div>

                    {/* Channel */}
                    <div className="col-span-1.5">
                      {rel.channel_name ? (
                        <span className="font-mono text-[10px] text-black">
                          #{rel.channel_name}
                        </span>
                      ) : (
                        <span className="font-mono text-[10px] text-muted-foreground">
                          global
                        </span>
                      )}
                    </div>

                    {/* Weight */}
                    <div className="col-span-1">
                      <span className="font-mono text-xs font-bold tabular-nums">
                        {rel.weight ?? 1}
                      </span>
                    </div>

                    {/* Created */}
                    <div className="col-span-1">
                      <span
                        className="font-mono text-[10px] text-muted-foreground"
                        title={rel.created_at}
                      >
                        {rel.created_at ? relativeTime(rel.created_at) : '--'}
                      </span>
                    </div>

                    {/* Actions */}
                    <div className="col-span-0.5 flex justify-end">
                      <button
                        type="button"
                        onClick={() => setDeleteTarget(rel)}
                        className={cn(
                          'flex h-7 w-7 items-center justify-center',
                          'border-2 border-black bg-white',
                          'hover:bg-brutal-danger hover:text-white',
                          'active:translate-x-0.5 active:translate-y-0.5 active:shadow-none',
                          'transition-all duration-100',
                          'rounded-none',
                        )}
                        title="Delete relationship"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </div>

      {/* ---- Create modal ---- */}
      <CreateRelationshipModal
        open={showCreateModal}
        onOpenChange={setShowCreateModal}
        onCreated={() => {
          refetch();
        }}
        agents={agents}
        channels={channels}
      />

      {/* ---- Delete confirm dialog ---- */}
      <Dialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
        width="sm"
      >
        <DialogHeader>
          <DialogTitle className="font-heading text-base font-black uppercase tracking-wider">
            Delete Relationship
          </DialogTitle>
        </DialogHeader>

        {deleteTarget && (
          <div className="space-y-3">
            <p className="font-body text-sm text-muted-foreground">
              Are you sure you want to delete this relationship?
            </p>

            <div className="flex items-center gap-3 p-3 border-2 border-black bg-brutal-muted-light">
              <div className="flex flex-col gap-1">
                <div className="flex items-center gap-2">
                  <span className="font-mono text-[10px] text-muted-foreground">From:</span>
                  <span className="font-heading text-xs font-bold">
                    {deleteTarget.from_agent_name || deleteTarget.from_agent_id.slice(0, 8)}
                  </span>
                </div>
                <div className="flex items-center gap-2">
                  <span className="font-mono text-[10px] text-muted-foreground">To:</span>
                  <span className="font-heading text-xs font-bold">
                    {deleteTarget.to_agent_name || deleteTarget.to_agent_id.slice(0, 8)}
                  </span>
                </div>
              </div>
              <TypeBadge type={deleteTarget.rel_type} />
            </div>
          </div>
        )}

        <DialogFooter>
          <button
            type="button"
            onClick={() => setDeleteTarget(null)}
            disabled={isDeleting}
            className="btn-brutal-sm px-4 py-1.5"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={handleDelete}
            disabled={isDeleting}
            className="btn-brutal-sm bg-brutal-danger text-white px-4 py-1.5 disabled:opacity-50 disabled:pointer-events-none"
          >
            {isDeleting ? (
              <span className="flex items-center gap-1.5">
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
                Deleting...
              </span>
            ) : (
              'Delete'
            )}
          </button>
        </DialogFooter>
      </Dialog>
    </div>
  );
}
