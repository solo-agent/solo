// ============================================================================
// AgentMessage — brutalist Agent message with Markdown rendering
// - Pink left border (3px, #fe7da8)
// - Cream background
// - Bot icon + badge-brutal "Agent" label
// - Markdown code blocks: black bg, Space Mono, green text
// ============================================================================

'use client';

import { Children } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeRaw from 'rehype-raw';
import { MessageSquare } from 'lucide-react';
import type { AgentDetailTarget, Message } from '@/lib/types';
import { cn } from '@/lib/utils';
import { highlightSpecials } from '@/lib/utils/highlight';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { t } from '@/lib/i18n';

interface AgentMessageProps {
  message: Message;
  onReply?: (message: Message) => void;
  /** Lowercased display_names that may receive highlight. Empty = no @mentions highlighted. */
  validNames?: string[];
  isHighlighted?: boolean;
  onOpenArtifactReference?: (ref: string) => void;
  onAgentClick?: (agent: AgentDetailTarget) => void;
}

/** Fenced code block renderer — black bg, Space Mono, green text */
function CodeBlock({
  className,
  children,
}: {
  className?: string;
  children?: React.ReactNode;
}) {
  const language = className?.replace('language-', '') ?? '';
  return (
    <div className="my-2 border-2 border-black shadow-brutal-sm overflow-x-auto">
      {language && (
        <div className="border-b-2 border-black bg-brutal-primary px-3 py-1 font-mono text-[10px] font-bold uppercase tracking-wider text-black">
          {language}
        </div>
      )}
      <pre className="bg-black p-3 text-xs leading-relaxed">
        <code className={`${className ?? ''} font-mono text-brutal-success`}>
          {children}
        </code>
      </pre>
    </div>
  );
}

/** Wrap @mentions and #task-numbers in HTML spans, protecting code fences */

function getArtifactReference(value?: string): string | null {
  if (!value) return null;
  const text = value.trim();
  if (/\/api\/v1\/artifacts\/[0-9a-f-]+/i.test(text)) return text;
  if (/\/\.solo\/artifacts\/[0-9a-f-]+\/[^/\s]+\.html/i.test(text)) return text;
  return null;
}

const artifactReferencePattern = /(https?:\/\/[^\s)]+\/api\/v1\/artifacts\/[0-9a-f-]+(?:\/meta)?(?:\?[^\s)]*)?|\/[^\s)]+\/\.solo\/artifacts\/[0-9a-f-]+\/[^\s)]+\.html)/gi;

export function AgentMessage({ message, onReply, validNames = [], isHighlighted, onOpenArtifactReference, onAgentClick }: AgentMessageProps) {
  const time = new Date(message.created_at).toLocaleString('en-US', {
    hour: '2-digit',
    minute: '2-digit',
  });

  const hasUnreadThread = message.has_unread_thread === true && (message.reply_count ?? 0) > 0;
  const openArtifact = (reference: string) => {
    onOpenArtifactReference?.(reference);
  };
  const renderChildren = (children?: React.ReactNode) => (
    Children.map(children, (child) => {
      if (typeof child !== 'string') return child;
      const parts = child.split(artifactReferencePattern);
      return parts.map((part, index) => {
        const artifactRef = getArtifactReference(part);
        if (!artifactRef) return part;
        return (
          <button
            key={`${part}-${index}`}
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              openArtifact(artifactRef);
            }}
            className="inline border-0 bg-transparent p-0 font-bold text-brutal-info underline decoration-2 underline-offset-2 hover:text-brutal-primary"
          >
            {part}
          </button>
        );
      });
    })
  );

  return (
    <div
      data-message-id={message.id}
      className={cn(
        'group relative flex gap-3 px-6 py-2.5 agent-message border-l-brutal-primary border-b border-brutal-muted',
        isHighlighted && 'bg-brutal-info-light ring-2 ring-black',
      )}
      role="listitem"
    >
      <PixelAvatar
        agentId={message.user_id}
        size="md"
        className="mt-0.5 flex-shrink-0"
        onClick={onAgentClick ? () => onAgentClick?.({
          id: message.user_id,
          name: message.display_name,
          is_active: message.sender_active,
        }) : undefined}
        ariaLabel={t('viewAgentDetail', { name: message.display_name })}
      />

      <div className="min-w-0 flex-1">
        <div className="mb-1.5 flex items-baseline gap-2">
          <span className="font-heading text-sm font-bold text-foreground">
            {message.display_name}
          </span>
          {message.sender_active === false ? (
            <span className="badge-brutal bg-brutal-muted text-black">
              {t('deleted')}
            </span>
          ) : (
            <span className="badge-brutal bg-brutal-primary text-black">
              {t('agent')}
            </span>
          )}
          <span className="font-mono text-[11px] text-muted-foreground">
            {time}
          </span>
        </div>
        <div className="font-body text-sm leading-relaxed space-y-1">
          <ReactMarkdown
            remarkPlugins={[remarkGfm]}
            rehypePlugins={[rehypeRaw]}
            components={{
              p({ children }) {
                return (
                  <p className="my-1 whitespace-pre-wrap break-words">
                    {renderChildren(children)}
                  </p>
                );
              },
              ul({ children }) {
                return (
                  <ul className="my-1 list-disc pl-4 space-y-0.5">
                    {children}
                  </ul>
                );
              },
              ol({ children }) {
                return (
                  <ol className="my-1 list-decimal pl-4 space-y-0.5">
                    {children}
                  </ol>
                );
              },
              li({ children }) {
                return <li className="leading-relaxed">{renderChildren(children)}</li>;
              },
              strong({ children }) {
                return (
                  <strong className="font-heading font-black">
                    {children}
                  </strong>
                );
              },
              a({ href, children }) {
                const artifactRef = getArtifactReference(href);
                if (artifactRef) {
                  return (
                    <button
                      type="button"
                      onClick={(e) => {
                        e.stopPropagation();
                        openArtifact(artifactRef);
                      }}
                      className="inline border-0 bg-transparent p-0 font-bold text-brutal-info underline decoration-2 underline-offset-2 hover:text-brutal-primary"
                    >
                      {children}
                    </button>
                  );
                }
                return (
                  <a
                    href={href}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-brutal-info font-bold underline decoration-2 underline-offset-2 hover:text-brutal-primary transition-colors"
                  >
                    {children}
                  </a>
                );
              },
              blockquote({ children }) {
                return (
                  <blockquote className="my-1.5 border-l-2 border-brutal-primary/50 pl-3 italic text-muted-foreground">
                    {children}
                  </blockquote>
                );
              },
              code({ className, children, ...props }) {
                const isInline = !className;
                if (isInline) {
                  const artifactRef = getArtifactReference(String(children ?? ''));
                  if (artifactRef) {
                    return (
                      <code className="inline-code-brutal">
                        <button
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation();
                            openArtifact(artifactRef);
                          }}
                          className="text-brutal-info underline decoration-2 underline-offset-2 hover:text-brutal-primary"
                        >
                          {children}
                        </button>
                      </code>
                    );
                  }
                  return (
                    <code className="inline-code-brutal">
                      {children}
                    </code>
                  );
                }
                return (
                  <CodeBlock className={className}>{children}</CodeBlock>
                );
              },
              pre({ children }) {
                return <>{children}</>;
              },
              hr() {
                return <hr className="divider-brutal my-3" />;
              },
              table({ children }) {
                return (
                  <div className="my-2 overflow-x-auto border-2 border-black shadow-brutal-sm">
                    <table className="w-full text-sm font-body">
                      {children}
                    </table>
                  </div>
                );
              },
              th({ children }) {
                return (
                  <th className="border-b-2 border-black bg-brutal-primary px-3 py-2 text-left font-heading font-bold text-black">
                    {children}
                  </th>
                );
              },
              td({ children }) {
                return (
                  <td className="border-t border-black px-3 py-1.5">
                    {children}
                  </td>
                );
              },
            }}
          >
            {highlightSpecials(message.content, validNames)}
          </ReactMarkdown>
        </div>

        {/* Thread reply count — brutalist badge */}
        {(message.reply_count ?? 0) > 0 && onReply && (
          <button
            type="button"
            onClick={(e) => { e.stopPropagation(); onReply(message); }}
            className={cn(
              'mt-2 badge-brutal cursor-pointer transition-all',
              hasUnreadThread
                ? 'bg-brutal-primary text-black border-brutal-primary'
                : 'bg-white text-black hover:bg-brutal-primary hover:-translate-y-px hover:shadow-brutal',
            )}
          >
            <MessageSquare className="mr-1 h-3 w-3" />
            <span>{t('threadReplies', { n: message.reply_count ?? 0 })}</span>
          </button>
        )}
      </div>

      {/* Hover reply button */}
      {onReply && (
        <div className="absolute right-3 top-2 flex items-center gap-1
                        opacity-0 group-hover:opacity-100
                        translate-x-2 group-hover:translate-x-0
                        transition-all duration-200">
          <button
            type="button"
            onClick={(e) => { e.stopPropagation(); onReply(message); }}
            className="btn-brutal btn-brutal-sm flex h-7 w-7 items-center justify-center p-0"
            aria-label={t('replyToMessage', { name: message.display_name })}
            title="Reply"
          >
            <MessageSquare className="h-3.5 w-3.5" />
          </button>
        </div>
      )}
    </div>
  );
}
