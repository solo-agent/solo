'use client';

import { useCallback, useMemo, useRef } from 'react';
import {
  Background,
  Controls,
  Handle,
  MarkerType,
  Position,
  ReactFlow,
  type Edge,
  type Node,
  type NodeProps,
  type ReactFlowInstance,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import dagre from 'dagre';
import { RelationshipEdge } from '@/components/relationships/relationship-edge';
import { orderCollaboratingIds, reorderRankX } from '@/components/relationships/relationship-layout';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { t } from '@/lib/i18n';
import type { Template, TemplateMember, TemplateRelationship } from '@/lib/templates-api';
import type { RelationshipType } from '@/lib/types';

const NODE_TYPES = { templateRole: TemplateRoleNode };
const EDGE_TYPES = { relationship: RelationshipEdge };

interface TemplateRoleNodeData {
  templateId: string;
  member: TemplateMember;
}

export function TemplateGraph({
  template,
  selectedRef,
  selectedRelationshipIndex,
  onSelect,
  onSelectRelationship,
}: {
  template: Template;
  selectedRef?: string | null;
  selectedRelationshipIndex?: number | null;
  onSelect?: (ref: string) => void;
  onSelectRelationship?: (relationship: TemplateRelationship, index: number) => void;
}) {
  const flowRef = useRef<ReactFlowInstance | null>(null);
  const { nodes, edges } = useMemo(
    () => layoutTemplate(template, selectedRef, selectedRelationshipIndex),
    [selectedRef, selectedRelationshipIndex, template],
  );
  const interactiveEdges = useMemo<Edge[]>(() => edges.map((edge) => ({
    ...edge,
    data: {
      ...edge.data,
      ariaLabel: edge.ariaLabel,
      onSelect: (event: React.MouseEvent<HTMLButtonElement>) => {
        const data = edge.data as { relationship?: TemplateRelationship; relationshipIndex?: number } | undefined;
        if (data?.relationship && data.relationshipIndex !== undefined) {
          event.stopPropagation();
          onSelectRelationship?.(data.relationship, data.relationshipIndex);
        }
      },
    },
  })), [edges, onSelectRelationship]);
  const fitGraph = useCallback(() => {
    requestAnimationFrame(() => {
      flowRef.current?.fitView({
        padding: template.member_count > 6 ? 0.08 : 0.18,
        maxZoom: 1,
      });
    });
  }, [template.member_count]);

  return (
    <ReactFlow
      key={template.id}
      className="template-flow"
      nodes={nodes}
      edges={interactiveEdges}
      nodeTypes={NODE_TYPES}
      edgeTypes={EDGE_TYPES}
      fitView
      fitViewOptions={{ padding: template.member_count > 6 ? 0.08 : 0.18, maxZoom: 1 }}
      minZoom={0.35}
      maxZoom={1.6}
      nodesDraggable={false}
      nodesConnectable={false}
      elementsSelectable
      onInit={(instance) => {
        flowRef.current = instance;
        fitGraph();
      }}
      onNodeClick={(_event, node) => onSelect?.(node.id)}
      onEdgeClick={(_event, edge) => {
        const data = edge.data as { relationship?: TemplateRelationship; relationshipIndex?: number } | undefined;
        if (data?.relationship && data.relationshipIndex !== undefined) {
          onSelectRelationship?.(data.relationship, data.relationshipIndex);
        }
      }}
      proOptions={{ hideAttribution: true }}
    >
      <Background color="var(--skin-rule)" gap={24} size={1} />
      <Controls
        showInteractive={false}
        className="flow-controls"
        style={{ border: '2px solid var(--skin-rule)', boxShadow: '3px 3px 0 var(--color-brutal-shadow)' }}
      />
    </ReactFlow>
  );
}

function TemplateRoleNode({ data }: NodeProps) {
  const { templateId, member } = data as unknown as TemplateRoleNodeData;
  return (
    <>
      <Handle id="top" type="target" position={Position.Top} className="!h-2 !w-2 !border !border-white !bg-[var(--skin-ink)]" />
      <Handle id="bottom" type="source" position={Position.Bottom} className="!h-2 !w-2 !border !border-white !bg-[var(--skin-ink)]" />
      <Handle id="left" type="target" position={Position.Left} className="!h-2 !w-2 !border !border-white !bg-[var(--skin-ink)]" />
      <Handle id="right" type="source" position={Position.Right} className="!h-2 !w-2 !border !border-white !bg-[var(--skin-ink)]" />
      <div className="flex items-center gap-2.5 text-left">
        <PixelAvatar
          agentId={`${templateId}:${member.ref}`}
          avatarUrl={member.avatar_url}
          size="sm"
          ariaLabel={member.name}
        />
        <div className="min-w-0">
          {member.role !== member.name && (
            <div className="font-mono text-[9px] font-bold uppercase tracking-wider text-black/50">{member.role}</div>
          )}
          <div className="truncate font-heading text-sm font-black">{member.name}</div>
          <div className="mt-0.5 truncate font-body text-[10px] text-black/60">{member.description}</div>
        </div>
      </div>
    </>
  );
}

function layoutTemplate(
  template: Template,
  selectedRef?: string | null,
  selectedRelationshipIndex?: number | null,
): { nodes: Node[]; edges: Edge[] } {
  const isLargeTeam = (template.members?.length ?? 0) > 6;
  const nodeWidth = isLargeTeam ? 168 : 204;
  const nodeHeight = isLargeTeam ? 68 : 76;
  const graph = new dagre.graphlib.Graph();
  graph.setDefaultEdgeLabel(() => ({}));
  graph.setGraph({
    rankdir: 'TB',
    ranksep: isLargeTeam ? 72 : 88,
    nodesep: isLargeTeam ? 24 : 60,
    marginx: 20,
    marginy: 20,
  });

  const members = template.members ?? [];
  const membersByRef = new Map(members.map((member) => [member.ref, member]));
  const orderedMembers = orderCollaboratingIds(
    members.map((member) => member.ref),
    (template.relationships ?? [])
      .filter((relationship) => relationship.type === 'collaborates_with')
      .map((relationship) => [relationship.from_ref, relationship.to_ref]),
  ).map((ref) => membersByRef.get(ref)!);

  for (const member of orderedMembers) {
    graph.setNode(member.ref, { width: nodeWidth, height: nodeHeight });
  }
  for (const relationship of template.relationships ?? []) {
    if (relationship.type === 'assigns_to') {
      graph.setEdge(relationship.from_ref, relationship.to_ref);
    }
  }
  dagre.layout(graph);
  const rankX = reorderRankX(
    orderedMembers.map((member) => member.ref),
    new Map(orderedMembers.map((member) => [member.ref, graph.node(member.ref)])),
  );

  const nodes: Node[] = members.map((member) => {
    const position = graph.node(member.ref);
    return {
      id: member.ref,
      type: 'templateRole',
      selected: selectedRef === member.ref,
      className: selectedRef === member.ref ? 'template-role-node template-role-node-selected' : 'template-role-node',
      position: { x: (rankX.get(member.ref) ?? position.x) - nodeWidth / 2, y: position.y - nodeHeight / 2 },
      data: { templateId: template.id, member },
      style: {
        width: nodeWidth,
        height: nodeHeight,
        border: '2px solid var(--skin-rule)',
        borderRadius: 0,
        background: selectedRef === member.ref ? 'var(--skin-accent-light)' : 'var(--skin-surface)',
        color: 'var(--skin-ink)',
        boxShadow: '3px 3px 0 var(--color-brutal-shadow)',
        padding: 9,
      },
    };
  });

  const positions = new Map(nodes.map((node) => [node.id, node.position]));
  const names = new Map((template.members ?? []).map((member) => [member.ref, member.name]));
  const edges: Edge[] = (template.relationships ?? []).map((relationship, index) => {
    const isCollaboration = relationship.type === 'collaborates_with';
    const fromX = positions.get(relationship.from_ref)?.x ?? 0;
    const toX = positions.get(relationship.to_ref)?.x ?? 0;
    const source = isCollaboration && fromX > toX ? relationship.to_ref : relationship.from_ref;
    const target = isCollaboration && fromX > toX ? relationship.from_ref : relationship.to_ref;
    return {
      id: `${relationship.from_ref}-${relationship.to_ref}-${index}`,
      source,
      target,
      type: 'relationship',
      selected: selectedRelationshipIndex === index,
      focusable: true,
      ariaLabel: relationship.type === 'assigns_to'
        ? `${names.get(relationship.from_ref)} ${t('assignsTo')} ${names.get(relationship.to_ref)}`
        : `${names.get(relationship.from_ref)} ${t('collaboratesWith')} ${names.get(relationship.to_ref)}`,
      ...(isCollaboration
        ? { sourceHandle: 'right', targetHandle: 'left' }
        : { sourceHandle: 'bottom', targetHandle: 'top' }),
      markerEnd: isCollaboration
        ? undefined
        : { type: MarkerType.ArrowClosed, color: 'var(--skin-ink)' },
      data: {
        relType: relationship.type as RelationshipType,
        relationship,
        relationshipIndex: index,
        stroke: isCollaboration ? 'var(--skin-success)' : 'var(--skin-accent)',
      },
    };
  });

  return { nodes, edges };
}
