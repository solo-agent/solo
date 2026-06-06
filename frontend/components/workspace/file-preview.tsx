'use client';

import { useEffect, useState, type ComponentPropsWithoutRef } from 'react';
import { createHighlighter } from 'shiki';
import { Loader2 } from 'lucide-react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

// Module-level singleton to avoid re-creating the highlighter
let highlighterPromise: ReturnType<typeof createHighlighter> | null = null;

function getHighlighter() {
  if (!highlighterPromise) {
    highlighterPromise = createHighlighter({
      themes: ['dark-plus'],
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
        const result = highlighter.codeToHtml(code, { lang, theme: 'dark-plus' });
        if (!cancelled) setHtml(result);
      } catch {
        // unsupported language — fall through to plain pre/code
      }
    });
    return () => { cancelled = true; };
  }, [code, lang]);

  if (html) {
    return <div dangerouslySetInnerHTML={{ __html: html }} className="[&>pre]:my-0 [&>pre]:rounded-none" />;
  }

  return (
    <pre className="bg-black text-brutal-lime border-2 border-black shadow-brutal-sm p-3 overflow-x-auto">
      <code className={className}>{children}</code>
    </pre>
  );
}

export function MarkdownPreview({ content }: { content: string }) {
  return (
    <div className="p-4 prose prose-sm max-w-none prose-headings:font-heading prose-headings:font-bold prose-code:font-mono">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{ code: CodeBlock }}
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
          theme: 'dark-plus',
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
      className="overflow-auto h-full [&>pre]:p-4 [&>pre]:font-mono [&>pre]:text-xs [&>pre]:leading-relaxed [&>pre]:bg-[#1e1e1e] [&>pre]:min-h-full [&>pre]:rounded-none"
      dangerouslySetInnerHTML={{ __html: html }}
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
