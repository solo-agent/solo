'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { useRouter } from 'next/navigation';
import { GitBranch, LayoutGrid, Link2, Loader2, RefreshCw, Save, Trash2, X } from 'lucide-react';
import { useAuth } from '@/lib/auth-context';
import { useAgents } from '@/lib/hooks/use-agents';
import {
  createRelationship,
  deleteRelationship,
  listRelationships,
  updateRelationship,
} from '@/lib/relationships-api';
import type { Agent, AgentRelationship, RelationshipType } from '@/lib/types';
import { NavBar } from '@/components/ui/navbar';
import { Button } from '@/components/ui/button';
import { Dialog, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Spinner } from '@/components/ui/spinner';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { cn } from '@/lib/utils';

type Point = { x: number; y: number };
type PositionMap = Record<string, Point>;

const NODE_W = 188;
const NODE_H = 82;
const STORAGE_KEY = 'solo-relationship-positions';

function readPositions(): PositionMap {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    return raw ? JSON.parse(raw) : {};
  } catch {
    return {};
  }
}

function writePositions(positions: PositionMap) {
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(positions));
  } catch {
    // localStorage may be unavailable; dragging should still work for the session.
  }
}

function autoLayout(agents: Agent[], relationships: AgentRelationship[]): PositionMap {
  const agentIDs = new Set(agents.map((a) => a.id));
  const children = new Map<string, string[]>();
  const incoming = new Map<string, number>();

  for (const agent of agents) {
    children.set(agent.id, []);
    incoming.set(agent.id, 0);
  }
  for (const rel of relationships) {
    if (rel.rel_type !== 'assigns_to') continue;
    if (!agentIDs.has(rel.from_agent_id) || !agentIDs.has(rel.to_agent_id)) continue;
    children.get(rel.from_agent_id)!.push(rel.to_agent_id);
    incoming.set(rel.to_agent_id, (incoming.get(rel.to_agent_id) ?? 0) + 1);
  }

  const level = new Map<string, number>();
  const queue = agents
    .filter((agent) => (incoming.get(agent.id) ?? 0) === 0)
    .sort((a, b) => a.name.localeCompare(b.name))
    .map((agent) => agent.id);

  for (const id of queue) level.set(id, 0);
  for (let i = 0; i < queue.length; i++) {
    const id = queue[i];
    const nextLevel = (level.get(id) ?? 0) + 1;
    for (const child of children.get(id) ?? []) {
      if ((level.get(child) ?? -1) < nextLevel) {
        level.set(child, nextLevel);
        queue.push(child);
      }
    }
  }
  for (const agent of agents) {
    if (!level.has(agent.id)) level.set(agent.id, 0);
  }

  const byLevel = new Map<number, Agent[]>();
  for (const agent of agents) {
    const l = level.get(agent.id) ?? 0;
    byLevel.set(l, [...(byLevel.get(l) ?? []), agent]);
  }

  const positions: PositionMap = {};
  [...byLevel.entries()].sort(([a], [b]) => a - b).forEach(([l, items]) => {
    items.sort((a, b) => a.name.localeCompare(b.name)).forEach((agent, i) => {
      positions[agent.id] = { x: 72 + l * 270, y: 72 + i * 132 };
    });
  });
  return positions;
}

function centerOf(p: Point): Point {
  return { x: p.x + NODE_W / 2, y: p.y + NODE_H / 2 };
}

function relationLabel(type: RelationshipType) {
  return type === 'assigns_to' ? 'assigns to' : 'collaborates';
}

export default function RelationshipsPage() {
  const router = useRouter();
  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const { agents, isLoading: agentsLoading, error: agentsError, refetch: refetchAgents } = useAgents();
  const [relationships, setRelationships] = useState<AgentRelationship[]>([]);
  const [isLoadingRels, setIsLoadingRels] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [positions, setPositions] = useState<PositionMap>({});
  const [dragging, setDragging] = useState<{
    id: string;
    start: Point;
    origin: Point;
  } | null>(null);
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [creating, setCreating] = useState(false);
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [form, setForm] = useState({
    from_agent_id: '',
    to_agent_id: '',
    rel_type: 'assigns_to' as RelationshipType,
    instruction: '',
  });
  const [draftInstruction, setDraftInstruction] = useState('');

  useEffect(() => {
    if (!authLoading && !isAuthenticated) router.push('/auth/login');
  }, [authLoading, isAuthenticated, router]);

  const loadRelationships = useCallback(async () => {
    setIsLoadingRels(true);
    setError(null);
    try {
      setRelationships(await listRelationships());
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load relationships');
    } finally {
      setIsLoadingRels(false);
    }
  }, []);

  useEffect(() => {
    loadRelationships();
  }, [loadRelationships]);

  useEffect(() => {
    if (agents.length === 0) return;
    const saved = readPositions();
    const layout = autoLayout(agents, relationships);
    const merged: PositionMap = {};
    for (const agent of agents) {
      merged[agent.id] = saved[agent.id] ?? layout[agent.id] ?? { x: 72, y: 72 };
    }
    setPositions(merged);
  }, [agents, relationships]);

  const agentByID = useMemo(() => new Map(agents.map((agent) => [agent.id, agent])), [agents]);
  const selected = selectedID ? relationships.find((rel) => rel.id === selectedID) ?? null : null;

  useEffect(() => {
    setDraftInstruction(selected?.instruction ?? '');
  }, [selected?.id, selected?.instruction]);

  useEffect(() => {
    if (!dragging) return;
    const handleMove = (event: PointerEvent) => {
      const next = {
        x: Math.max(0, dragging.origin.x + event.clientX - dragging.start.x),
        y: Math.max(0, dragging.origin.y + event.clientY - dragging.start.y),
      };
      setPositions((prev) => ({ ...prev, [dragging.id]: next }));
    };
    const handleUp = () => {
      setPositions((prev) => {
        writePositions(prev);
        return prev;
      });
      setDragging(null);
    };
    window.addEventListener('pointermove', handleMove);
    window.addEventListener('pointerup', handleUp, { once: true });
    return () => {
      window.removeEventListener('pointermove', handleMove);
      window.removeEventListener('pointerup', handleUp);
    };
  }, [dragging]);

  const canvasSize = useMemo(() => {
    const values = Object.values(positions);
    return {
      width: Math.max(900, ...values.map((p) => p.x + NODE_W + 160)),
      height: Math.max(560, ...values.map((p) => p.y + NODE_H + 120)),
    };
  }, [positions]);

  const handleAutoLayout = useCallback(() => {
    const next = autoLayout(agents, relationships);
    setPositions(next);
    writePositions(next);
  }, [agents, relationships]);

  const handleRefresh = useCallback(() => {
    refetchAgents();
    loadRelationships();
  }, [loadRelationships, refetchAgents]);

  const handleCreate = useCallback(async () => {
    if (!form.from_agent_id || !form.to_agent_id || form.from_agent_id === form.to_agent_id) return;
    setCreating(true);
    setError(null);
    try {
      await createRelationship(form);
      setShowCreate(false);
      setForm((prev) => ({ ...prev, instruction: '' }));
      await loadRelationships();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create relationship');
    } finally {
      setCreating(false);
    }
  }, [form, loadRelationships]);

  const handleSaveSelected = useCallback(async () => {
    if (!selected) return;
    setSaving(true);
    setError(null);
    try {
      const updated = await updateRelationship(selected.id, { instruction: draftInstruction });
      setRelationships((prev) => prev.map((rel) => (rel.id === updated.id ? updated : rel)));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update relationship');
    } finally {
      setSaving(false);
    }
  }, [draftInstruction, selected]);

  const handleDeleteSelected = useCallback(async () => {
    if (!selected) return;
    setDeleting(true);
    setError(null);
    try {
      await deleteRelationship(selected.id);
      setRelationships((prev) => prev.filter((rel) => rel.id !== selected.id));
      setSelectedID(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete relationship');
    } finally {
      setDeleting(false);
    }
  }, [selected]);

  const loading = authLoading || agentsLoading || isLoadingRels;
  if (loading) {
    return (
      <div className="flex h-screen bg-background">
        <NavBar />
        <div className="flex flex-1 items-center justify-center">
          <Spinner label="Loading relationships..." />
        </div>
      </div>
    );
  }

  const visibleRelationships = relationships.filter(
    (rel) => agentByID.has(rel.from_agent_id) && agentByID.has(rel.to_agent_id),
  );

  return (
    <div className="flex h-screen bg-background">
      <NavBar />
      <main className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 items-center justify-between border-b-2 border-black bg-brutal-cream px-4">
          <div className="flex items-center gap-2">
            <div className="flex h-8 w-8 items-center justify-center border-2 border-black bg-brutal-primary shadow-brutal-sm">
              <GitBranch className="h-4 w-4" />
            </div>
            <div>
              <h1 className="font-heading text-lg font-black uppercase tracking-wider">Relationships</h1>
              <p className="font-mono text-[10px] text-muted-foreground">
                Drag nodes · Auto layout · Edit instructions
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Button size="sm" variant="outline" onClick={handleRefresh}>
              <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
              Refresh
            </Button>
            <Button size="sm" variant="outline" onClick={handleAutoLayout}>
              <LayoutGrid className="mr-1.5 h-3.5 w-3.5" />
              Auto Layout
            </Button>
            <Button size="sm" onClick={() => setShowCreate(true)}>
              <Link2 className="mr-1.5 h-3.5 w-3.5" />
              Create
            </Button>
          </div>
        </header>

        {(error || agentsError) && (
          <div className="border-b-2 border-black bg-brutal-danger/10 px-4 py-2 font-body text-sm text-brutal-danger">
            {error || agentsError}
          </div>
        )}

        <section className="relative min-h-0 flex-1 overflow-auto bg-[radial-gradient(circle,rgba(0,0,0,0.08)_1px,transparent_1px)] [background-size:20px_20px]">
          {agents.length === 0 ? (
            <div className="flex h-full items-center justify-center p-8 text-center">
              <p className="font-body text-sm text-muted-foreground">
                No agents yet. Create agents from Teams before editing relationships.
              </p>
            </div>
          ) : (
            <div className="relative" style={{ width: canvasSize.width, height: canvasSize.height }}>
              <svg
                className="pointer-events-none absolute inset-0"
                width={canvasSize.width}
                height={canvasSize.height}
              >
                <defs>
                  <marker id="relationship-arrow" markerWidth="10" markerHeight="10" refX="8" refY="3" orient="auto">
                    <path d="M0,0 L0,6 L9,3 z" fill="#111" />
                  </marker>
                </defs>
                {visibleRelationships.map((rel) => {
                  const from = positions[rel.from_agent_id];
                  const to = positions[rel.to_agent_id];
                  if (!from || !to) return null;
                  const a = centerOf(from);
                  const b = centerOf(to);
                  const dx = Math.max(80, Math.abs(b.x - a.x) / 2);
                  const path = `M ${a.x} ${a.y} C ${a.x + dx} ${a.y}, ${b.x - dx} ${b.y}, ${b.x} ${b.y}`;
                  const isAssign = rel.rel_type === 'assigns_to';
                  return (
                    <g key={rel.id}>
                      <path
                        d={path}
                        fill="none"
                        stroke={selectedID === rel.id ? '#FFD23F' : isAssign ? '#111' : '#0f766e'}
                        strokeWidth={selectedID === rel.id ? 5 : 3}
                        strokeDasharray={isAssign ? undefined : '8 5'}
                        markerEnd={isAssign ? 'url(#relationship-arrow)' : undefined}
                      />
                      <text
                        x={(a.x + b.x) / 2}
                        y={(a.y + b.y) / 2 - 8}
                        className="fill-black font-mono text-[10px]"
                        textAnchor="middle"
                      >
                        {relationLabel(rel.rel_type)}
                      </text>
                    </g>
                  );
                })}
              </svg>

              {visibleRelationships.map((rel) => {
                const from = positions[rel.from_agent_id];
                const to = positions[rel.to_agent_id];
                if (!from || !to) return null;
                const a = centerOf(from);
                const b = centerOf(to);
                return (
                  <button
                    key={`${rel.id}-hit`}
                    type="button"
                    aria-label={`Select ${relationLabel(rel.rel_type)} relationship`}
                    onClick={() => setSelectedID(rel.id)}
                    className="absolute h-8 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-black bg-white px-2 font-mono text-[10px] shadow-brutal-sm hover:bg-brutal-primary"
                    style={{ left: (a.x + b.x) / 2, top: (a.y + b.y) / 2 }}
                  >
                    {rel.rel_type === 'assigns_to' ? 'assign' : 'collab'}
                  </button>
                );
              })}

              {agents.map((agent) => {
                const p = positions[agent.id] ?? { x: 72, y: 72 };
                return (
                  <div
                    key={agent.id}
                    className={cn(
                      'absolute cursor-grab select-none border-2 border-black bg-white p-3 shadow-brutal active:cursor-grabbing',
                      dragging?.id === agent.id && 'z-20 shadow-brutal-xl',
                    )}
                    style={{ left: p.x, top: p.y, width: NODE_W, height: NODE_H }}
                    onPointerDown={(event) => {
                      event.currentTarget.setPointerCapture(event.pointerId);
                      setDragging({
                        id: agent.id,
                        start: { x: event.clientX, y: event.clientY },
                        origin: p,
                      });
                    }}
                  >
                    <div className="flex items-start gap-2">
                      <PixelAvatar agentId={agent.id} avatarUrl={agent.avatar_url} size="sm" />
                      <div className="min-w-0">
                        <div className="truncate font-heading text-sm font-black">{agent.name}</div>
                        <div className="mt-0.5 line-clamp-2 font-body text-[11px] text-muted-foreground">
                          {agent.description || 'No description'}
                        </div>
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </section>
      </main>

      {selected && (
        <aside className="w-[320px] shrink-0 border-l-2 border-black bg-white p-4 shadow-brutal-xl">
          <div className="mb-3 flex items-center justify-between">
            <h2 className="font-heading text-sm font-black uppercase tracking-wider">Relationship</h2>
            <button
              type="button"
              onClick={() => setSelectedID(null)}
              className="flex h-7 w-7 items-center justify-center border-2 border-black bg-white hover:bg-brutal-primary"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          </div>
          <div className="space-y-3 font-body text-sm">
            <div className="rounded-none border-2 border-black bg-brutal-cream p-3">
              <div className="font-heading text-xs font-bold uppercase text-muted-foreground">From</div>
              <div className="font-bold">{agentByID.get(selected.from_agent_id)?.name ?? selected.from_agent_id}</div>
            </div>
            <div className="rounded-none border-2 border-black bg-brutal-cream p-3">
              <div className="font-heading text-xs font-bold uppercase text-muted-foreground">To</div>
              <div className="font-bold">{agentByID.get(selected.to_agent_id)?.name ?? selected.to_agent_id}</div>
            </div>
            <div>
              <div className="mb-1 font-heading text-xs font-bold uppercase text-muted-foreground">Type</div>
              <div className="inline-flex border-2 border-black bg-brutal-primary px-2 py-1 font-mono text-xs">
                {selected.rel_type}
              </div>
            </div>
            <label className="block">
              <span className="mb-1 block font-heading text-xs font-bold uppercase text-muted-foreground">
                Instruction
              </span>
              <textarea
                value={draftInstruction}
                onChange={(event) => setDraftInstruction(event.target.value)}
                className="min-h-[150px] w-full resize-y rounded-none border-2 border-black bg-white p-2 font-body text-sm outline-none focus:bg-brutal-primary-light/20"
                placeholder="When should this relationship be used?"
              />
            </label>
            <div className="flex gap-2">
              <Button size="sm" onClick={handleSaveSelected} disabled={saving}>
                {saving ? <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" /> : <Save className="mr-1.5 h-3.5 w-3.5" />}
                Save
              </Button>
              <Button size="sm" variant="danger" onClick={handleDeleteSelected} disabled={deleting}>
                {deleting ? <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" /> : <Trash2 className="mr-1.5 h-3.5 w-3.5" />}
                Delete
              </Button>
            </div>
          </div>
        </aside>
      )}

      <Dialog open={showCreate} onOpenChange={setShowCreate} width="md">
        <DialogHeader>
          <DialogTitle className="font-heading text-base font-black uppercase tracking-wider">
            Create Relationship
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <label className="block">
            <span className="mb-1 block font-heading text-xs font-bold uppercase">From Agent</span>
            <select
              value={form.from_agent_id}
              onChange={(event) => setForm((prev) => ({ ...prev, from_agent_id: event.target.value }))}
              className="h-10 w-full rounded-none border-2 border-black bg-white px-2 font-body text-sm"
            >
              <option value="">Select agent...</option>
              {agents.map((agent) => (
                <option key={agent.id} value={agent.id}>{agent.name}</option>
              ))}
            </select>
          </label>
          <label className="block">
            <span className="mb-1 block font-heading text-xs font-bold uppercase">To Agent</span>
            <select
              value={form.to_agent_id}
              onChange={(event) => setForm((prev) => ({ ...prev, to_agent_id: event.target.value }))}
              className="h-10 w-full rounded-none border-2 border-black bg-white px-2 font-body text-sm"
            >
              <option value="">Select agent...</option>
              {agents.map((agent) => (
                <option key={agent.id} value={agent.id}>{agent.name}</option>
              ))}
            </select>
          </label>
          <label className="block">
            <span className="mb-1 block font-heading text-xs font-bold uppercase">Type</span>
            <select
              value={form.rel_type}
              onChange={(event) => setForm((prev) => ({ ...prev, rel_type: event.target.value as RelationshipType }))}
              className="h-10 w-full rounded-none border-2 border-black bg-white px-2 font-body text-sm"
            >
              <option value="assigns_to">assigns_to</option>
              <option value="collaborates_with">collaborates_with</option>
            </select>
          </label>
          <label className="block">
            <span className="mb-1 block font-heading text-xs font-bold uppercase">Instruction</span>
            <textarea
              value={form.instruction}
              onChange={(event) => setForm((prev) => ({ ...prev, instruction: event.target.value }))}
              className="min-h-[96px] w-full resize-y rounded-none border-2 border-black bg-white p-2 font-body text-sm"
              placeholder="Optional delegation/collaboration guidance"
            />
          </label>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => setShowCreate(false)}>Cancel</Button>
          <Button
            onClick={handleCreate}
            disabled={creating || !form.from_agent_id || !form.to_agent_id || form.from_agent_id === form.to_agent_id}
          >
            {creating && <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
            Create
          </Button>
        </DialogFooter>
      </Dialog>
    </div>
  );
}
