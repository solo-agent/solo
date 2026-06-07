// ============================================================================
// StreamingMessage — renders an in-progress streaming Agent message
// - Pink left border (3px, #fe7da8) — distinct from completed message
// - Pink blinking cursor at end of content
// - Live Markdown rendering (incremental)
// - Unclosed code block detected and shows "..." placeholder
// ============================================================================

'use client';

import { useRef, useEffect } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { cn } from '@/lib/utils';
import type { Message } from '@/lib/types';
import { PixelAvatar } from '@/components/ui/pixel-avatar';

interface StreamingMessageProps {
  message: Message;
}

/** Detect if a code block is unclosed (odd number of ``` markers) */
function hasUnclosedCodeBlock(content: string): boolean {
  const matches = content.match(/```/g);
  return matches !== null && matches.length % 2 !== 0;
}

/** Fenced code block renderer with "..." fallback for unclosed blocks */
function CodeBlock({
  className,
  children,
  isUnclosed,
}: {
  className?: string;
  children?: React.ReactNode;
  isUnclosed?: boolean;
}) {
  const language = className?.replace('language-', '') ?? '';
  return (
    <div className="my-2 border-2 border-black shadow-brutal-sm overflow-x-auto">
      {language && (
        <div className="border-b-2 border-black bg-brutal-pink px-3 py-1 font-mono text-[10px] font-bold uppercase tracking-wider text-black">
          {language}
        </div>
      )}
      <pre className="bg-black p-3 text-xs leading-relaxed">
        <code className={`${className ?? ''} font-mono text-brutal-lime`}>
          {children}
          {isUnclosed && (
            <span className="text-muted-foreground">...</span>
          )}
        </code>
      </pre>
    </div>
  );
}

/** Pink blinking cursor indicator */
function BlinkingCursor() {
  return (
    <span className="cursor-blink-pink align-middle" />
  );
}

/** Animated typing indicator — three bouncing dots */
function TypingDots() {
  const dots = [0, 1, 2];
  return (
    <span className="inline-flex items-center gap-1" aria-label="Agent is typing">
      {dots.map((i) => (
        <span
          key={i}
          className={cn(
            'inline-block h-1.5 w-1.5 bg-brutal-pink',
            'animate-bounce',
          )}
          style={{
            animationDelay: `${i * 0.15}s`,
            animationDuration: '0.8s',
          }}
        />
      ))}
    </span>
  );
}

export function StreamingMessage({ message }: StreamingMessageProps) {
  const time = new Date(message.created_at).toLocaleString('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
  });

  const unclosedCodeBlock = hasUnclosedCodeBlock(message.content);
  const containerRef = useRef<HTMLDivElement>(null);

  // Auto-scroll parent container when streaming content changes
  useEffect(() => {
    if (!containerRef.current) return;
    const parent = containerRef.current.closest('[data-streaming-container]') as HTMLElement | null;
    if (!parent) return;

    const threshold = 80;
    const atBottom = parent.scrollHeight - parent.scrollTop - parent.clientHeight < threshold;
    if (atBottom) {
      parent.scrollTop = parent.scrollHeight;
    }
  }, [message.content]);

  return (
    <div
      ref={containerRef}
      data-message-id={message.id}
      className="group relative flex gap-3 px-6 py-2.5 agent-message border-l-brutal-pink"
      role="listitem"
      aria-label="流式输出中"
      data-streaming="true"
    >
      <PixelAvatar agentId={message.user_id} size="md" className="mt-0.5 flex-shrink-0" />

      <div className="min-w-0 flex-1">
        <div className="mb-1.5 flex items-baseline gap-2">
          <span className="font-heading text-sm font-bold text-brutal-pink">
            {message.display_name}
          </span>
          {message.sender_active === false && (
            <span className="badge-brutal bg-brutal-stone text-black">
              DELETED
            </span>
          )}
          <span className="font-mono text-[11px] text-muted-foreground">
            {time}
          </span>
          <span className="badge-brutal bg-brutal-pink/20 text-brutal-pink">
            流式输出中
          </span>
        </div>
        <div className="font-body text-sm leading-relaxed space-y-1">
          {message.content ? (
            <ReactMarkdown
              remarkPlugins={[remarkGfm]}
              components={{
                p({ children }) {
                  return (
                    <p className="my-1 whitespace-pre-wrap break-words">
                      {children}
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
                  return <li className="leading-relaxed">{children}</li>;
                },
                strong({ children }) {
                  return (
                    <strong className="font-heading font-bold">
                      {children}
                    </strong>
                  );
                },
                blockquote({ children }) {
                  return (
                    <blockquote className="my-1.5 border-l-2 border-brutal-pink/50 pl-3 italic text-muted-foreground">
                      {children}
                    </blockquote>
                  );
                },
                code({ className, children, ...props }) {
                  const isInline = !className;
                  if (isInline) {
                    return (
                      <code className="rounded-none border border-black bg-black/5 px-1 py-0.5 font-mono text-xs text-foreground">
                        {children}
                      </code>
                    );
                  }
                  return (
                    <CodeBlock className={className} isUnclosed={unclosedCodeBlock}>
                      {children}
                    </CodeBlock>
                  );
                },
                pre({ children }) {
                  return <>{children}</>;
                },
                hr() {
                  return <hr className="divider-brutal my-3" />;
                },
              }}
            >
              {message.content}
            </ReactMarkdown>
          ) : (
            <div className="py-1">
              <TypingDots />
            </div>
          )}
        </div>

        {/* Pink blinking cursor at end of content (only when content exists) */}
        {message.content && <BlinkingCursor />}
      </div>
    </div>
  );
}
