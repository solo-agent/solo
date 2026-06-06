'use client';

import { Loader2 } from 'lucide-react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

export function MarkdownPreview({ content }: { content: string }) {
  return (
    <div className="p-4 prose prose-sm max-w-none prose-headings:font-heading prose-headings:font-bold prose-code:font-mono prose-pre:bg-black prose-pre:text-brutal-lime prose-pre:border-2 prose-pre:border-black prose-pre:shadow-brutal-sm">
      <ReactMarkdown remarkPlugins={[remarkGfm]}>
        {content}
      </ReactMarkdown>
    </div>
  );
}

export function CodePreview({ content, language: _language }: { content: string; language: string }) {
  return (
    <pre className="p-4 font-mono text-xs leading-relaxed whitespace-pre-wrap overflow-auto h-full bg-brutal-cream border-l-2 border-black">
      <code>{content}</code>
    </pre>
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

  return <CodePreview content={content} language="" />;
}
