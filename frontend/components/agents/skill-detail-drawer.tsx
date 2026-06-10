'use client';

import { X, FileText, FolderTree, Loader2, AlertCircle, RefreshCw } from 'lucide-react';
import { useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { useSkill } from '@/lib/hooks/use-skills';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

interface SkillDetailDrawerProps {
  skillId: string | null;
  onClose: () => void;
}

export function SkillDetailDrawer({ skillId, onClose }: SkillDetailDrawerProps) {
  const { skill, isLoading, error } = useSkill(skillId);
  const [activeTab, setActiveTab] = useState<'md' | 'files'>('md');

  if (skillId === null) {
    return null;
  }

  return (
    <div
      className="fixed inset-y-0 right-0 z-40 flex w-full max-w-2xl flex-col border-l-2 border-black bg-white shadow-brutal-xl"
      role="dialog"
      aria-modal="true"
      aria-label="Skill 详情"
    >
      {/* Header */}
      <div className="flex items-center justify-between border-b-2 border-black px-4 py-3">
        <div className="flex items-center gap-2 min-w-0 flex-1">
          <FileText className="h-4 w-4 flex-shrink-0" />
          {skill ? (
            <>
              <h2 className="font-heading text-base font-bold text-foreground truncate">
                {skill.name}
              </h2>
              <span className="badge-brutal text-[10px] bg-brutal-primary text-white px-1.5 flex-shrink-0">
                {skill.source_kind}
              </span>
            </>
          ) : (
            <h2 className="font-heading text-base font-bold text-muted-foreground">
              Skill 详情
            </h2>
          )}
        </div>
        <button
          type="button"
          onClick={onClose}
          className="flex h-8 w-8 flex-shrink-0 items-center justify-center border-2 border-black bg-white shadow-brutal-sm hover:shadow-brutal transition-all"
          aria-label="关闭"
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      {/* Tab bar */}
      <div className="flex border-b-2 border-black">
        <button
          type="button"
          onClick={() => setActiveTab('md')}
          className={cn(
            'flex items-center gap-1.5 px-4 py-2 font-heading text-xs font-bold uppercase tracking-wider border-r-2 border-black',
            activeTab === 'md' ? 'bg-brutal-primary text-white' : 'bg-white text-foreground',
          )}
        >
          <FileText className="h-3.5 w-3.5" />
          SKILL.md
        </button>
        <button
          type="button"
          onClick={() => setActiveTab('files')}
          className={cn(
            'flex items-center gap-1.5 px-4 py-2 font-heading text-xs font-bold uppercase tracking-wider',
            activeTab === 'files' ? 'bg-brutal-primary text-white' : 'bg-white text-foreground',
          )}
        >
          <FolderTree className="h-3.5 w-3.5" />
          Files ({skill?.files.length ?? 0})
        </button>
      </div>

      {/* Body */}
      <div className="flex-1 overflow-y-auto">
        {isLoading && (
          <div className="flex items-center justify-center py-12 gap-2">
            <Loader2 className="h-5 w-5 animate-spin" />
            <span className="font-mono text-xs text-muted-foreground">加载中...</span>
          </div>
        )}
        {error && (
          <div className="flex flex-col items-center justify-center py-12 px-4">
            <div className="mb-3 flex h-12 w-12 items-center justify-center border-2 border-black bg-brutal-danger-light shadow-brutal-sm">
              <AlertCircle className="h-6 w-6 text-brutal-danger" />
            </div>
            <p className="font-body text-sm text-brutal-danger text-center">{error}</p>
          </div>
        )}
        {skill && !isLoading && !error && activeTab === 'md' && (
          <article className="prose prose-sm max-w-none p-6 font-body text-sm">
            <ReactMarkdown remarkPlugins={[remarkGfm]}>
              {skill.body}
            </ReactMarkdown>
          </article>
        )}
        {skill && !isLoading && !error && activeTab === 'files' && (
          <div className="p-4 space-y-2">
            {skill.files.length === 0 ? (
              <div className="card-brutal bg-brutal-cream p-4 text-center">
                <p className="font-mono text-xs italic text-muted-foreground">
                  暂未同步 supporting files
                </p>
              </div>
            ) : (
              skill.files.map((f) => (
                <div key={f.id} className="border-2 border-black bg-white p-3 shadow-brutal-sm">
                  <div className="font-mono text-xs font-bold text-foreground">{f.path}</div>
                  <pre className="mt-2 overflow-x-auto font-mono text-[11px] text-muted-foreground whitespace-pre">
                    {f.content.slice(0, 500)}
                    {f.content.length > 500 ? '…' : ''}
                  </pre>
                </div>
              ))
            )}
          </div>
        )}
      </div>

      {/* Footer */}
      {skill && (
        <div className="border-t-2 border-black bg-brutal-cream px-4 py-2">
          <p className="font-mono text-[10px] text-muted-foreground truncate" title={skill.source_path}>
            {skill.source_path}
          </p>
        </div>
      )}
    </div>
  );
}
