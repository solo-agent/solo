'use client';

import { useState } from 'react';
import { Wrench, ChevronDown, ChevronRight, AlertTriangle } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { AgentChunk } from '@/lib/hooks/use-agent-chunks';

interface AgentChunkItemProps {
  chunk: AgentChunk;
}

function ToolUseDisplay({ chunk }: { chunk: AgentChunk }) {
  const [expanded, setExpanded] = useState(false);
  const tool = chunk.tool;
  if (!tool) return null;

  return (
    <div className="chunk-tool-use border-l-2 border-brutal-pink pl-2 py-0.5">
      <button
        type="button"
        onClick={() => setExpanded(!expanded)}
        className="flex items-center gap-1 w-full text-left font-mono text-[11px]"
      >
        {expanded ? (
          <ChevronDown className="h-3 w-3 flex-shrink-0" />
        ) : (
          <ChevronRight className="h-3 w-3 flex-shrink-0" />
        )}
        <Wrench className="h-3 w-3 flex-shrink-0 text-brutal-pink" />
        <span className="font-bold">{tool.name}</span>
      </button>
      {expanded && tool.input && (
        <pre className="mt-1 bg-black/5 p-1.5 text-[10px] font-mono break-all whitespace-pre-wrap max-h-32 overflow-y-auto border border-black/10">
          {tool.input}
        </pre>
      )}
    </div>
  );
}

function ToolResultDisplay({ chunk }: { chunk: AgentChunk }) {
  const tool = chunk.tool;
  if (!tool) return null;

  return (
    <div className={cn(
      'chunk-tool-result border-l-2 pl-2 py-0.5',
      tool.output && tool.output.length > 0 ? 'border-brutal-lime' : 'border-muted',
    )}>
      <div className="font-mono text-[11px] text-muted-foreground">
        {tool.name} result
      </div>
      {tool.output && (
        <pre className="mt-0.5 text-[10px] font-mono break-all whitespace-pre-wrap max-h-24 overflow-y-auto text-foreground">
          {tool.output.length > 500 ? tool.output.slice(0, 500) + '…' : tool.output}
        </pre>
      )}
    </div>
  );
}

function ThinkingDisplay({ chunk }: { chunk: AgentChunk }) {
  const [expanded, setExpanded] = useState(false);

  const preview = chunk.content.length > 100
    ? chunk.content.slice(0, 100) + '…'
    : chunk.content;

  return (
    <div className="chunk-thinking">
      <button
        type="button"
        onClick={() => setExpanded(!expanded)}
        className="flex items-center gap-1 w-full text-left group"
      >
        {expanded ? (
          <ChevronDown className="h-3 w-3 flex-shrink-0 text-muted-foreground" />
        ) : (
          <ChevronRight className="h-3 w-3 flex-shrink-0 text-muted-foreground" />
        )}
        <span className="font-mono text-[11px] text-muted-foreground italic group-hover:text-foreground transition-colors">
          {expanded ? chunk.content : preview}
        </span>
      </button>
    </div>
  );
}

export function AgentChunkItem({ chunk }: AgentChunkItemProps) {
  switch (chunk.chunkType) {
    case 'thinking':
      return <ThinkingDisplay chunk={chunk} />;
    case 'tool_use':
      return <ToolUseDisplay chunk={chunk} />;
    case 'tool_result':
      return <ToolResultDisplay chunk={chunk} />;
    case 'error':
      return (
        <div className="chunk-error border-l-2 border-red-500 pl-2 py-0.5 flex items-start gap-1">
          <AlertTriangle className="h-3 w-3 flex-shrink-0 text-red-500 mt-0.5" />
          <span className="font-mono text-[11px] text-red-600">{chunk.content}</span>
        </div>
      );
    case 'text':
      return (
        <div className="chunk-text font-mono text-[11px] text-foreground/70 pl-2 py-0.5 border-l-2 border-transparent">
          {chunk.content}
        </div>
      );
    default:
      return null;
  }
}
