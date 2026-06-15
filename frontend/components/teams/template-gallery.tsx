// ============================================================================
// TemplateGallery (1.7) — smart-default UI for picking a built-in team template.
//
// Lists all built-in templates from GET /api/v1/templates. Each card shows
// icon, name, description, category, and an "Use this template" button. On
// click, POST /api/v1/templates/{id}/apply to create the full agent set + edges.
//
// State is intentionally minimal: no AI inference, no automatic actions —
// the user explicitly clicks before anything happens. This is the
// "manual over automatic" preference (see memory feedback-manual-over-automatic).
//
// Authentication: the current user_id is read from localStorage (matches the
// pattern used by other Teams components). If absent, the apply call will
// fail and the user will see a console error — the gallery still renders.
// ============================================================================

'use client';

import { useEffect, useState } from 'react';
import {
  listTemplates,
  applyTemplate,
  type Template,
} from '@/lib/templates-api';

export function TemplateGallery() {
  const [templates, setTemplates] = useState<Template[]>([]);
  const [loading, setLoading] = useState(true);
  const [applying, setApplying] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    listTemplates()
      .then((ts) => {
        if (!cancelled) setTemplates(ts);
      })
      .catch((err) => {
        if (!cancelled) {
          console.error('template-gallery load failed', err);
          setError(err?.message ?? 'Failed to load templates');
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, []);

  const handleApply = async (id: string) => {
    const userId =
      typeof window !== 'undefined' ? localStorage.getItem('user_id') : null;
    if (!userId) {
      setError('Not signed in');
      return;
    }
    setApplying(id);
    setError(null);
    try {
      const result = await applyTemplate(id, userId);
      console.info(
        `template applied: created ${result.created_agent_ids.length} agents`,
      );
    } catch (e) {
      const msg = e instanceof Error ? e.message : 'Failed to apply template';
      console.error('template apply failed', e);
      setError(msg);
    } finally {
      setApplying(null);
    }
  };

  if (loading) {
    return (
      <div className="card-brutal p-4" data-testid="template-gallery-loading">
        <div className="text-xs font-bold uppercase tracking-wider text-muted-foreground">
          Loading templates…
        </div>
      </div>
    );
  }

  return (
    <div
      className="grid grid-cols-1 md:grid-cols-3 gap-4"
      data-testid="template-gallery"
    >
      {templates.map((t) => (
        <div
          key={t.id}
          className="card-brutal p-4 border-4 border-black bg-white shadow-[6px_6px_0_#000]"
          data-testid={`template-card-${t.id}`}
        >
          <div className="text-3xl mb-2" aria-hidden>
            {t.icon}
          </div>
          <h3 className="text-lg font-bold uppercase">{t.name}</h3>
          <p className="text-sm text-gray-700 mb-3">{t.description}</p>
          <div className="text-xs text-gray-500 mb-3">
            Category: {t.category}
          </div>
          <button
            type="button"
            onClick={() => handleApply(t.id)}
            disabled={applying === t.id}
            className="brutal-button w-full px-3 py-2 bg-blue-300 border-2 border-black font-bold hover:bg-blue-400 disabled:opacity-50"
            data-testid={`template-apply-${t.id}`}
          >
            {applying === t.id ? 'Creating…' : 'Use this template'}
          </button>
        </div>
      ))}
      {error && (
        <div
          className="card-brutal p-3 col-span-full border-2 border-red-600"
          role="alert"
        >
          <div className="text-xs font-bold text-red-700">{error}</div>
        </div>
      )}
    </div>
  );
}
