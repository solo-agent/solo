// ============================================================================
// EnvEditor — brutalist key-value pair editor for custom_env
// Each row: KEY input + VALUE input + remove button
// Controls its own local state, synced via onChange
// ============================================================================

'use client';

import { useState, useCallback, useEffect } from 'react';
import { Plus, Trash2 } from 'lucide-react';
import { cn } from '@/lib/utils';

interface EnvEntry {
  id: string;
  key: string;
  value: string;
}

let envIdCounter = 0;
function nextEnvId(): string {
  return `env-${++envIdCounter}`;
}

interface EnvEditorProps {
  value?: Record<string, string>;
  onChange?: (env: Record<string, string>) => void;
  disabled?: boolean;
}

export function EnvEditor({ value, onChange, disabled }: EnvEditorProps) {
  const [entries, setEntries] = useState<EnvEntry[]>(() => {
    if (!value) return [];
    return Object.entries(value).map(([key, val]) => ({
      id: nextEnvId(),
      key,
      value: val,
    }));
  });

  // Sync external value changes
  useEffect(() => {
    if (!value) {
      setEntries([]);
      return;
    }
    const incoming = Object.entries(value);
    // Only reset if structure changed from outside (not from our own edits)
    if (incoming.length !== entries.length) {
      setEntries(
        incoming.map(([key, val]) => ({
          id: nextEnvId(),
          key,
          value: val,
        })),
      );
    }
  }, [value]); // eslint-disable-line react-hooks/exhaustive-deps

  const emit = useCallback(
    (next: EnvEntry[]) => {
      const result: Record<string, string> = {};
      for (const e of next) {
        const k = e.key.trim();
        if (k) result[k] = e.value;
      }
      onChange?.(result);
    },
    [onChange],
  );

  const addEntry = useCallback(() => {
    setEntries((prev) => {
      const next = [...prev, { id: nextEnvId(), key: '', value: '' }];
      emit(next);
      return next;
    });
  }, [emit]);

  const updateEntry = useCallback(
    (id: string, field: 'key' | 'value', newVal: string) => {
      setEntries((prev) => {
        const next = prev.map((e) =>
          e.id === id ? { ...e, [field]: newVal } : e,
        );
        emit(next);
        return next;
      });
    },
    [emit],
  );

  const removeEntry = useCallback(
    (id: string) => {
      setEntries((prev) => {
        const next = prev.filter((e) => e.id !== id);
        emit(next);
        return next;
      });
    },
    [emit],
  );

  return (
    <div className="space-y-2.5">
      {entries.map((entry, idx) => (
        <div key={entry.id} className="flex items-center gap-2">
          <input
            type="text"
            value={entry.key}
            onChange={(e) => updateEntry(entry.id, 'key', e.target.value)}
            placeholder="KEY"
            disabled={disabled}
            aria-label={`环境变量名 #${idx + 1}`}
            className={cn(
              'input-brutal h-9 flex-[1.2] font-mono text-xs uppercase tracking-wider',
              'placeholder:text-muted-foreground placeholder:font-body placeholder:normal-case placeholder:tracking-normal',
            )}
            style={{
              background: '#fffaef',
            }}
          />
          <input
            type="text"
            value={entry.value}
            onChange={(e) => updateEntry(entry.id, 'value', e.target.value)}
            placeholder="VALUE"
            disabled={disabled}
            aria-label={`环境变量值 #${idx + 1}`}
            className={cn(
              'input-brutal h-9 flex-[2] font-mono text-xs',
              'placeholder:text-muted-foreground placeholder:font-body',
            )}
            style={{
              background: '#fffaef',
            }}
          />
          <button
            type="button"
            onClick={() => removeEntry(entry.id)}
            disabled={disabled}
            aria-label={`移除环境变量 #${idx + 1}`}
            className={cn(
              'flex h-9 w-9 flex-shrink-0 items-center justify-center',
              'border-2 border-black bg-white shadow-brutal-sm',
              'transition-all hover:-translate-x-0.5 hover:-translate-y-0.5 hover:shadow-brutal',
              'active:translate-x-[3px] active:translate-y-[3px] active:shadow-none',
              'disabled:opacity-50 disabled:pointer-events-none',
            )}
            style={{ background: '#fffaef' }}
          >
            <Trash2 className="h-3.5 w-3.5 text-brutal-red" />
          </button>
        </div>
      ))}

      <button
        type="button"
        onClick={addEntry}
        disabled={disabled}
        className={cn(
          'inline-flex items-center gap-1.5 px-3 py-1.5',
          'border-2 border-black bg-white shadow-brutal-sm',
          'font-heading text-xs font-bold',
          'transition-all hover:-translate-x-0.5 hover:-translate-y-0.5 hover:shadow-brutal',
          'active:translate-x-[3px] active:translate-y-[3px] active:shadow-none',
          'disabled:opacity-50 disabled:pointer-events-none',
        )}
        style={{ background: '#fffaef' }}
      >
        <Plus className="h-3.5 w-3.5" />
        Add Variable
      </button>
    </div>
  );
}
