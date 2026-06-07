'use client';

import { useEffect, useState, type ComponentPropsWithoutRef } from 'react';
import { createHighlighter } from 'shiki';
import { Loader2 } from 'lucide-react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeRaw from 'rehype-raw';
import { sanitizeHtml } from '@/lib/sanitize';

// Module-level singleton to avoid re-creating the highlighter
let highlighterPromise: ReturnType<typeof createHighlighter> | null = null;

function getHighlighter() {
  if (!highlighterPromise) {
    highlighterPromise = createHighlighter({
      themes: ['everforest-light'],
      langs: ['typescript', 'javascript', 'python', 'go', 'rust', 'json', 'markdown',
              'css', 'html', 'yaml', 'toml', 'sql', 'bash', 'tsx', 'jsx',
              'xml', 'graphql', 'mdx', 'dockerfile', 'shellscript'],
    });
  }
  return highlighterPromise;
}

function detectLanguage(filename: string): string {
  const ext = filename.split('.').pop()?.toLowerCase();
  const map: Record<string, string> = {
    ts: 'typescript', tsx: 'tsx', js: 'javascript', jsx: 'jsx',
    py: 'python', go: 'go', rs: 'rust', json: 'json',
    md: 'markdown', css: 'css', html: 'html', yaml: 'yaml',
    yml: 'yaml', toml: 'toml', sql: 'sql', sh: 'bash',
    bash: 'bash', graphql: 'graphql', mdx: 'mdx',
    dockerfile: 'dockerfile', xml: 'xml',
  };
  return map[ext || ''] || 'text';
}

function CodeBlock({ children, className }: ComponentPropsWithoutRef<'code'>) {
  const [html, setHtml] = useState<string | null>(null);
  const code = String(children).replace(/\n$/, '');
  const lang = className?.replace('language-', '') || 'text';

  useEffect(() => {
    let cancelled = false;
    getHighlighter().then(async (highlighter) => {
      if (cancelled) return;
      try {
        const result = highlighter.codeToHtml(code, { lang, theme: 'everforest-light' });
        if (!cancelled) setHtml(result);
      } catch {
        // unsupported language — fall through to plain pre/code
      }
    });
    return () => { cancelled = true; };
  }, [code, lang]);

  if (html) {
    return <div dangerouslySetInnerHTML={{ __html: sanitizeHtml(html) }} className="[&>pre]:my-0 [&>pre]:rounded-none [&>pre]:bg-brutal-cream" />;
  }

  return (
    <pre className="bg-black text-brutal-success border-2 border-black shadow-brutal-sm p-3 overflow-x-auto">
      <code className={className}>{children}</code>
    </pre>
  );
}

// ---- Markdown component map (matches chat brutalist style) ----

const mdComponents = {
  p({ children }: { children: React.ReactNode }) {
    return <p className="my-1 whitespace-pre-wrap break-words">{children}</p>;
  },
  ul({ children }: { children: React.ReactNode }) {
    return <ul className="my-1 list-disc pl-4 space-y-0.5">{children}</ul>;
  },
  ol({ children }: { children: React.ReactNode }) {
    return <ol className="my-1 list-decimal pl-4 space-y-0.5">{children}</ol>;
  },
  li({ children }: { children: React.ReactNode }) {
    return <li className="leading-relaxed">{children}</li>;
  },
  strong({ children }: { children: React.ReactNode }) {
    return <strong className="font-heading font-black">{children}</strong>;
  },
  a({ href, children }: { href?: string; children: React.ReactNode }) {
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
  blockquote({ children }: { children: React.ReactNode }) {
    return (
      <blockquote className="my-1.5 border-l-2 border-brutal-primary pl-3 italic text-muted-foreground">
        {children}
      </blockquote>
    );
  },
  code({ className, children, ...props }: ComponentPropsWithoutRef<'code'>) {
    const isInline = !className;
    if (isInline) {
      return (
        <code className="rounded-none border border-black bg-black/5 px-1 py-0.5 font-mono text-xs text-foreground">
          {children}
        </code>
      );
    }
    return <CodeBlock className={className}>{children}</CodeBlock>;
  },
  pre({ children }: { children: React.ReactNode }) {
    return <>{children}</>;
  },
  hr() {
    return <hr className="divider-brutal my-3" />;
  },
  table({ children }: { children: React.ReactNode }) {
    return (
      <div className="my-2 overflow-x-auto border-2 border-black shadow-brutal-sm">
        <table className="w-full text-sm font-body">{children}</table>
      </div>
    );
  },
  th({ children }: { children: React.ReactNode }) {
    return (
      <th className="border-b-2 border-black bg-brutal-primary px-3 py-2 text-left font-heading font-bold text-black">
        {children}
      </th>
    );
  },
  td({ children }: { children: React.ReactNode }) {
    return <td className="border-t border-black px-3 py-1.5">{children}</td>;
  },
  h1({ children }: { children: React.ReactNode }) {
    return <h1 className="mt-4 mb-2 font-heading text-xl font-bold text-foreground">{children}</h1>;
  },
  h2({ children }: { children: React.ReactNode }) {
    return <h2 className="mt-3 mb-1.5 font-heading text-lg font-bold text-foreground">{children}</h2>;
  },
  h3({ children }: { children: React.ReactNode }) {
    return <h3 className="mt-2 mb-1 font-heading text-base font-bold text-foreground">{children}</h3>;
  },
  img({ src, alt }: { src?: string; alt?: string }) {
    return (
      <img
        src={src}
        alt={alt}
        className="my-2 max-w-full border-2 border-black shadow-brutal-sm"
      />
    );
  },
};

export function MarkdownPreview({ content }: { content: string }) {
  return (
    <div className="p-4 font-body text-sm leading-relaxed space-y-1">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[rehypeRaw]}
        components={mdComponents}
      >
        {content}
      </ReactMarkdown>
    </div>
  );
}

export function CodePreview({ content, language }: { content: string; language: string }) {
  const [html, setHtml] = useState<string | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    let cancelled = false;
    getHighlighter().then(async (highlighter) => {
      if (cancelled) return;
      try {
        const lang = language || detectLanguage('');
        const result = highlighter.codeToHtml(content, {
          lang: lang,
          theme: 'everforest-light',
        });
        if (!cancelled) setHtml(result);
      } catch {
        if (!cancelled) setError(true);
      }
    });
    return () => { cancelled = true; };
  }, [content, language]);

  if (error) {
    return (
      <pre className="p-4 font-mono text-xs leading-relaxed whitespace-pre-wrap overflow-auto h-full bg-brutal-cream border-l-2 border-black">
        <code>{content}</code>
      </pre>
    );
  }

  if (html === null) {
    return (
      <div className="flex items-center justify-center h-full">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div
      className="overflow-auto h-full [&>pre]:p-4 [&>pre]:font-mono [&>pre]:text-xs [&>pre]:leading-relaxed [&>pre]:bg-brutal-cream [&>pre]:min-h-full [&>pre]:rounded-none"
      dangerouslySetInnerHTML={{ __html: sanitizeHtml(html) }}
    />
  );
}

export function FilePreview({
  path, content, isLoading,
}: {
  path: string;
  content: string | null;
  isLoading: boolean;
}) {
  const isMarkdown = path.endsWith('.md');

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-full">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (content === null) {
    return (
      <div className="flex items-center justify-center h-full">
        <p className="font-mono text-xs text-muted-foreground">加载文件内容失败</p>
      </div>
    );
  }

  if (isMarkdown) {
    return <MarkdownPreview content={content} />;
  }

  return <CodePreview content={content} language={detectLanguage(path)} />;
}
