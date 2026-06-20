// ============================================================================
// TypeSelector — popup for selecting relationship type when creating a new edge
// - 4 type options with visual preview
// - Channel selector for channel-scoped types
// - Cancel / Confirm actions
// ============================================================================

import { useState } from 'react';
import { X } from 'lucide-react';
import type { RelationshipType } from '@/lib/types';
import { t, type TranslationKey } from '@/lib/i18n';

interface TypeOption {
  type: RelationshipType;
  label: TranslationKey;
  stroke: string;
  dash: string;
}

const TYPE_OPTIONS: TypeOption[] = [
  { type: 'assigns_to',        label: 'assignsTo' as TranslationKey,        stroke: '#4A90D9', dash: '' },
  { type: 'collaborates_with', label: 'collaboratesWith' as TranslationKey, stroke: '#10B981', dash: '8,4' },
];

interface TypeSelectorProps {
  fromName: string;
  toName: string;
  onSelect: (type: RelationshipType) => void;
  onCancel: () => void;
}

export function TypeSelector({ fromName, toName, onSelect, onCancel }: TypeSelectorProps) {
  const [selected, setSelected] = useState<RelationshipType>('assigns_to');

  return (
    <div className="border-4 border-black bg-white shadow-brutal-xl p-4 min-w-[280px]">
      {/* Header */}
      <div className="flex items-center justify-between mb-3">
        <h3 className="font-heading text-sm font-bold uppercase tracking-wider">
          {t('relationshipEditorSelectType')}
        </h3>
        <button
          type="button"
          onClick={onCancel}
          className="flex items-center justify-center w-6 h-6 border-2 border-black hover:bg-brutal-accent-light"
          aria-label={t('close')}
        >
          <X className="h-3 w-3" />
        </button>
      </div>

      {/* From → To */}
      <div className="mb-3 px-2 py-1.5 border-2 border-black bg-brutal-cream">
        <span className="font-mono text-xs text-muted-foreground">
          {t('relationshipEditorFrom')}: <span className="font-bold text-black">{fromName}</span>
          {' → '}
          {t('relationshipEditorTo')}: <span className="font-bold text-black">{toName}</span>
        </span>
      </div>

      {/* Type options */}
      <div className="space-y-1.5 mb-3">
        {TYPE_OPTIONS.map((opt) => (
          <button
            key={opt.type}
            type="button"
            onClick={() => setSelected(opt.type)}
            className={[
              'flex items-center gap-2.5 w-full px-3 py-2 text-left border-2 transition-colors duration-100',
              selected === opt.type
                ? 'border-black bg-brutal-primary-light'
                : 'border-transparent hover:border-brutal-muted hover:bg-white',
            ].join(' ')}
          >
            {/* Edge preview */}
            <svg width="32" height="12" className="flex-shrink-0">
              <line x1="0" y1="6" x2="32" y2="6"
                stroke={opt.stroke}
                strokeWidth={2}
                strokeDasharray={opt.dash || undefined}
              />
            </svg>
            <span className="font-heading text-xs font-bold uppercase tracking-wider">
              {t(opt.label)}
            </span>
          </button>
        ))}
      </div>

      {/* Confirm */}
      <div className="flex items-center justify-end gap-2">
        <button
          type="button"
          onClick={onCancel}
          className="btn-brutal-xs px-3 py-1.5"
        >
          {t('cancel')}
        </button>
        <button
          type="button"
          onClick={() => onSelect(selected)}
          className="btn-brutal-xs px-3 py-1.5 bg-brutal-success text-black"
        >
          {t('confirm')}
        </button>
      </div>
    </div>
  );
}
