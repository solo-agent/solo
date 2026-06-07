// ============================================================================
// Select — brutal-styled custom dropdown
// - Closes + opens as a custom listbox so the open state matches the rest of
//   the brutalist design system (hard border, hard offset shadow, mono/heading
//   typography). Native <select> open state is browser-rendered and untouchable
//   across browsers, so we use a custom panel instead.
// - Keyboard: Esc closes, Enter/Space toggles or selects, ArrowUp/Down navigate.
// - Click-outside closes; clicking an option selects and closes.
// - Native form integration: pass `name` to render a hidden <input>, so the
//   value participates in <form> submission. For react-hook-form, wrap in
//   <Controller> rather than using register's ref directly.
// - API: onChange receives the raw value string (not an event) — cleaner call
//   sites and matches standard controlled-component patterns.
// ============================================================================

'use client';

import * as React from 'react';
import { ChevronDown } from 'lucide-react';
import { cn } from '@/lib/utils';

export interface SelectOption {
  value: string;
  label: string;
  disabled?: boolean;
}

export interface SelectProps {
  options: SelectOption[];
  value?: string;
  /** Called with the selected option's value. */
  onChange?: (value: string) => void;
  /** Placeholder shown when no value is selected. */
  placeholder?: string;
  /** Visual size; default matches btn-brutal-sm proportions. */
  size?: 'sm' | 'md';
  className?: string;
  disabled?: boolean;
  id?: string;
  /** Hidden form input name for native form submission. */
  name?: string;
  /** Forwarded to the trigger button on blur. */
  onBlur?: () => void;
  'aria-label'?: string;
}

const SIZE_CLASSES: Record<NonNullable<SelectProps['size']>, string> = {
  sm: 'h-8 px-2.5 text-xs',
  md: 'h-10 px-3 text-sm',
};

export const Select = React.forwardRef<HTMLDivElement, SelectProps>(
  (
    {
      options,
      value,
      onChange,
      placeholder,
      size = 'sm',
      className,
      disabled,
      id,
      name,
      onBlur,
      ...props
    },
    ref,
  ) => {
    const [open, setOpen] = React.useState(false);
    const [activeIndex, setActiveIndex] = React.useState(-1);
    const containerRef = React.useRef<HTMLDivElement | null>(null);

    const selected = options.find((o) => o.value === value);
    const displayLabel = selected?.label ?? placeholder;

    React.useImperativeHandle(ref, () => containerRef.current as HTMLDivElement);

    // Close on outside click
    React.useEffect(() => {
      if (!open) return;
      const onDown = (e: MouseEvent) => {
        if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
          setOpen(false);
        }
      };
      document.addEventListener('mousedown', onDown);
      return () => document.removeEventListener('mousedown', onDown);
    }, [open]);

    // Reset active highlight when opening or when value/options change
    React.useEffect(() => {
      if (open) {
        const idx = options.findIndex((o) => o.value === value);
        setActiveIndex(idx >= 0 ? idx : 0);
      } else {
        setActiveIndex(-1);
      }
    }, [open, value, options]);

    const selectOption = React.useCallback(
      (opt: SelectOption) => {
        if (opt.disabled) return;
        onChange?.(opt.value);
        setOpen(false);
      },
      [onChange],
    );

    const handleKeyDown = (e: React.KeyboardEvent) => {
      if (disabled) return;
      switch (e.key) {
        case 'Escape':
          setOpen(false);
          break;
        case 'Enter':
        case ' ':
          e.preventDefault();
          if (open && activeIndex >= 0) {
            selectOption(options[activeIndex]);
          } else {
            setOpen((v) => !v);
          }
          break;
        case 'ArrowDown':
          e.preventDefault();
          if (!open) {
            setOpen(true);
          } else {
            setActiveIndex((i) => Math.min(i + 1, options.length - 1));
          }
          break;
        case 'ArrowUp':
          e.preventDefault();
          if (!open) {
            setOpen(true);
          } else {
            setActiveIndex((i) => Math.max(i - 1, 0));
          }
          break;
      }
    };

    return (
      <div
        ref={containerRef}
        className={cn('relative inline-block', className)}
        {...props}
      >
        {name && <input type="hidden" name={name} value={value ?? ''} />}
        <button
          type="button"
          id={id}
          onClick={() => !disabled && setOpen((v) => !v)}
          onKeyDown={handleKeyDown}
          onBlur={onBlur}
          disabled={disabled}
          aria-haspopup="listbox"
          aria-expanded={open}
          aria-label={props['aria-label']}
          className={cn(
            'inline-flex w-full items-center justify-between gap-1',
            'bg-white text-black font-heading font-bold',
            'border-2 border-black shadow-brutal-sm',
            'focus-visible:outline-none focus-visible:shadow-brutal',
            'disabled:opacity-50 disabled:pointer-events-none',
            'rounded-none',
            SIZE_CLASSES[size],
          )}
        >
          <span className={cn('truncate', !selected && 'opacity-60')}>
            {displayLabel ?? ''}
          </span>
          <ChevronDown
            aria-hidden
            className={cn(
              'h-3.5 w-3.5 flex-shrink-0 transition-transform',
              open && 'rotate-180',
            )}
          />
        </button>
        {open && (
          <ul
            role="listbox"
            className="absolute left-0 right-0 top-full z-30 mt-1 max-h-60 overflow-y-auto border-2 border-black bg-white shadow-brutal"
          >
            {options.map((opt, i) => {
              const isSelected = opt.value === value;
              const isActive = i === activeIndex;
              return (
                <li
                  key={opt.value}
                  role="option"
                  aria-selected={isSelected}
                  aria-disabled={opt.disabled}
                  // preventDefault on mousedown keeps focus on the trigger so
                  // keyboard navigation continues to work after selection.
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={() => selectOption(opt)}
                  onMouseEnter={() => setActiveIndex(i)}
                  className={cn(
                    'cursor-pointer px-3 py-1.5 font-heading text-xs font-bold transition-colors',
                    opt.disabled && 'cursor-not-allowed opacity-50',
                    // Selected: invert — black bg, pink text. High contrast,
                    // distinctly different from the hover state.
                    isSelected
                      ? 'bg-black text-brutal-primary'
                      : // Hover/keyboard-active: bold primary CTA — no pastels.
                        isActive
                        ? 'bg-brutal-primary text-black'
                        : // Default: white card, black ink.
                          'bg-white text-black',
                  )}
                >
                  {opt.label}
                </li>
              );
            })}
          </ul>
        )}
      </div>
    );
  },
);
Select.displayName = 'Select';
