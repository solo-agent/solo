'use client';

import { Children, type ReactNode } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeRaw from 'rehype-raw';
import { highlightSpecials } from '@/lib/utils/highlight';

interface MessageMarkdownProps {
  content: string;
  validNames?: string[];
  onOpenArtifactReference?: (ref: string) => void;
}

function CodeBlock({ className, children }: { className?: string; children?: ReactNode }) {
  const language = className?.replace('language-', '') ?? '';
  return (
    <div className="my-2 overflow-x-auto border-2 border-black shadow-brutal-sm">
      {language && (
        <div className="border-b-2 border-black bg-brutal-primary px-3 py-1 font-mono text-[10px] font-bold uppercase tracking-wider text-black">
          {language}
        </div>
      )}
      <pre className="bg-black p-3 text-xs leading-relaxed">
        <code className={`${className ?? ''} font-mono text-brutal-success`}>{children}</code>
      </pre>
    </div>
  );
}

function getArtifactReference(value?: string): string | null {
  if (!value) return null;
  const text = value.trim();
  if (/\/api\/v1\/artifacts\/[0-9a-f-]+/i.test(text)) return text;
  if (/\/\.solo\/artifacts\/[0-9a-f-]+\/[^/\s]+\.html/i.test(text)) return text;
  return null;
}

const artifactReferencePattern = /(https?:\/\/[^\s)]+\/api\/v1\/artifacts\/[0-9a-f-]+(?:\/meta)?(?:\?[^\s)]*)?|\/[^\s)]+\/\.solo\/artifacts\/[0-9a-f-]+\/[^\s)]+\.html)/gi;

export function MessageMarkdown({ content, validNames = [], onOpenArtifactReference }: MessageMarkdownProps) {
  const renderChildren = (children?: ReactNode) => (
    Children.map(children, (child) => {
      if (typeof child !== 'string') return child;
      return child.split(artifactReferencePattern).map((part, index) => {
        const artifactRef = getArtifactReference(part);
        if (!artifactRef) return part;
        return (
          <button
            key={`${part}-${index}`}
            type="button"
            onClick={(event) => {
              event.stopPropagation();
              onOpenArtifactReference?.(artifactRef);
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
    <div className="space-y-1 font-body text-sm leading-relaxed">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[rehypeRaw]}
        components={{
          h1({ children }) {
            return <h1 className="mb-2 mt-3 font-heading text-xl font-black">{children}</h1>;
          },
          h2({ children }) {
            return <h2 className="mb-1.5 mt-3 font-heading text-lg font-black">{children}</h2>;
          },
          h3({ children }) {
            return <h3 className="mb-1 mt-2 font-heading text-base font-black">{children}</h3>;
          },
          p({ children }) {
            return <p className="my-1 whitespace-pre-wrap break-words">{renderChildren(children)}</p>;
          },
          ul({ children }) {
            return <ul className="my-1 list-disc space-y-0.5 pl-4">{children}</ul>;
          },
          ol({ children }) {
            return <ol className="my-1 list-decimal space-y-0.5 pl-4">{children}</ol>;
          },
          li({ children }) {
            return <li className="leading-relaxed">{renderChildren(children)}</li>;
          },
          strong({ children }) {
            return <strong className="font-heading font-black">{children}</strong>;
          },
          a({ href, children }) {
            const artifactRef = getArtifactReference(href);
            if (artifactRef) {
              return (
                <button
                  type="button"
                  onClick={(event) => {
                    event.stopPropagation();
                    onOpenArtifactReference?.(artifactRef);
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
                className="font-bold text-brutal-info underline decoration-2 underline-offset-2 transition-colors hover:text-brutal-primary"
              >
                {children}
              </a>
            );
          },
          blockquote({ children }) {
            return <blockquote className="my-1.5 border-l-2 border-brutal-primary/50 pl-3 italic text-muted-foreground">{children}</blockquote>;
          },
          code({ className, children }) {
            if (!className) {
              const artifactRef = getArtifactReference(String(children ?? ''));
              return (
                <code className="inline-code-brutal">
                  {artifactRef ? (
                    <button
                      type="button"
                      onClick={(event) => {
                        event.stopPropagation();
                        onOpenArtifactReference?.(artifactRef);
                      }}
                      className="text-brutal-info underline decoration-2 underline-offset-2 hover:text-brutal-primary"
                    >
                      {children}
                    </button>
                  ) : children}
                </code>
              );
            }
            return <CodeBlock className={className}>{children}</CodeBlock>;
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
                <table className="w-full font-body text-sm">{children}</table>
              </div>
            );
          },
          th({ children }) {
            return <th className="border-b-2 border-black bg-brutal-primary px-3 py-2 text-left font-heading font-bold text-black">{children}</th>;
          },
          td({ children }) {
            return <td className="border-t border-black px-3 py-1.5">{children}</td>;
          },
        }}
      >
        {highlightSpecials(content, validNames)}
      </ReactMarkdown>
    </div>
  );
}
