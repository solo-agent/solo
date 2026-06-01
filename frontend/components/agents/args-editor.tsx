// ============================================================================
// ArgsEditor — brutalist tag-style input for custom_args
// Type text and press Enter to add a tag, click X to remove
// Each tag shown as a brutalist pill/badge
// ============================================================================

'use client';

import { useState, useCallback, useEffect, useRef, type KeyboardEvent } from 'react';
import { X } from 'lucide-react';
import { cn } from '@/lib/utils';

interface ArgsEditorProps {
  value?: string[];
  onChange?: (args: string[]) => void;
  disabled?: boolean;
}

export function ArgsEditor({ value, onChange, disabled }: ArgsEditorProps) {
  const [tags, setTags] = useState<string[]>(value ?? []);
  const [inputValue, setInputValue] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);

  // Sync external value changes (e.g., edit form pre-fill), but only when
  // the length differs — user edits are the source of truth after initial load.
  useEffect(() => {
    const incoming = value ?? [];
    if (incoming.length !== tags.length) {
      setTags(incoming);
    }
  }, [value]); // eslint-disable-line react-hooks/exhaustive-deps

  const emit = useCallback(
    (next: string[]) => {
      onChange?.(next);
    },
    [onChange],
  );

  const addTag = useCallback(
    (raw: string) => {
      const trimmed = raw.trim();
      if (!trimmed) return;
      // Prevent duplicates
      setTags((prev) => {
        if (prev.includes(trimmed)) return prev;
        const next = [...prev, trimmed];
        emit(next);
        return next;
      });
    },
    [emit],
  );

  const removeTag = useCallback(
    (idx: number) => {
      setTags((prev) => {
        const next = prev.filter((_, i) => i !== idx);
        emit(next);
        return next;
      });
    },
    [emit],
  );

  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLInputElement>) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        addTag(inputValue);
        setInputValue('');
      }
      // Backspace on empty input removes last tag
      if (e.key === 'Backspace' && inputValue === '' && tags.length > 0) {
        removeTag(tags.length - 1);
      }
    },
    [inputValue, addTag, tags.length, removeTag],
  );

  return (
    <div className="space-y-2">
      {/* Tag display area */}
      <div
        className={cn(
          'flex flex-wrap items-center gap-2 rounded-none p-2',
          'border-2 border-black shadow-brutal-sm min-h-[44px]',
          disabled && 'opacity-50',
        )}
        style={{ background: '#fffaef' }}
        onClick={() => inputRef.current?.focus()}
      >
        {tags.map((tag, idx) => (
          <span
            key={`${tag}-${idx}`}
            className={cn(
              'inline-flex items-center gap-1 px-2 py-0.5',
              'border-2 border-black bg-white shadow-brutal-sm',
              'font-mono text-xs font-bold text-foreground',
            )}
          >
            {tag}
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                removeTag(idx);
              }}
              disabled={disabled}
              aria-label={`移除参数 ${tag}`}
              className="ml-0.5 inline-flex items-center justify-center hover:opacity-70"
            >
              <X className="h-3 w-3" />
            </button>
          </span>
        ))}
        <input
          ref={inputRef}
          type="text"
          value={inputValue}
          onChange={(e) => setInputValue(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={tags.length === 0 ? '输入参数后按 Enter 添加...' : '继续添加...'}
          disabled={disabled}
          aria-label="自定义参数输入"
          className={cn(
            'flex-1 min-w-[140px] bg-transparent border-none outline-none',
            'font-mono text-xs text-foreground',
            'placeholder:text-muted-foreground placeholder:font-body',
            'py-1',
          )}
        />
      </div>
      <p className="font-mono text-[11px] text-muted-foreground">
        输入每个参数后按 Enter 添加。例如: <code className="text-foreground">--thinking-budget</code>{' '}
        <code className="text-foreground">16000</code>
      </p>
    </div>
  );
}
