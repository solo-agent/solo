'use client';

import { cn } from '@/lib/utils';

export interface TabBarTab {
  key: string;
  label: string;
  icon?: React.ReactNode;
}

interface TabBarProps {
  tabs: TabBarTab[];
  activeKey: string;
  onChange: (key: string) => void;
  variant?: 'pill' | 'segment';
  className?: string;
  children?: React.ReactNode;
}

export function TabBar({
  tabs,
  activeKey,
  onChange,
  variant = 'pill',
  className,
  children,
}: TabBarProps) {
  return (
    <div
      className={cn(
        'flex items-center border-b-2 border-black bg-brutal-cream',
        variant === 'pill' && 'h-10 px-4 gap-1',
        className,
      )}
      role="tablist"
    >
      {tabs.map((tab, index) => {
        const isActive = tab.key === activeKey;
        const isLast = index === tabs.length - 1;

        return (
          <button
            key={tab.key}
            type="button"
            role="tab"
            aria-selected={isActive}
            onClick={() => onChange(tab.key)}
            className={cn(
              'flex items-center gap-1.5 font-heading text-xs font-bold transition-all',
              'active:translate-x-0.5 active:translate-y-0.5 active:shadow-none',
              variant === 'pill'
                ? cn(
                    'px-3 py-1 border-2 border-black',
                    isActive
                      ? 'bg-brutal-primary text-black shadow-brutal-sm -translate-y-px'
                      : 'bg-white text-muted-foreground hover:text-foreground hover:shadow-brutal-sm hover:-translate-y-px',
                  )
                : cn(
                    'justify-center px-3 py-1 border-r-2 border-black',
                    isLast && 'border-r-0',
                    isActive
                      ? 'bg-brutal-primary text-black shadow-brutal-sm -translate-y-px'
                      : 'bg-white text-muted-foreground hover:bg-brutal-accent-light',
                  ),
            )}
          >
            {tab.icon}
            {tab.label}
          </button>
        );
      })}
      {children}
    </div>
  );
}
