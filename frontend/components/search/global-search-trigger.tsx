// ============================================================================
// GlobalSearchTrigger — client component that registers CMD+K listener
// and renders the GlobalSearch panel.
// Must be placed in the root layout so it's always mounted.
// ============================================================================

'use client';

import { useState, useEffect, useCallback } from 'react';
import { GlobalSearch } from './global-search';

export function GlobalSearchTrigger() {
  const [open, setOpen] = useState(false);

  const handleClose = useCallback(() => setOpen(false), []);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault();
        setOpen(true);
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, []);

  return <GlobalSearch open={open} onClose={handleClose} />;
}
