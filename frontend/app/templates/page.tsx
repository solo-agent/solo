'use client';

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { ArrowRight, LayoutTemplate, Users } from 'lucide-react';
import { AppFrame } from '@/components/layout/app-frame';
import { LucyTeamComposer } from '@/components/templates/lucy-team-composer';
import { Input } from '@/components/ui/input';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { Spinner } from '@/components/ui/spinner';
import { t } from '@/lib/i18n';
import { listTemplates, type Template } from '@/lib/templates-api';

export default function TemplatesPage() {
  const [templates, setTemplates] = useState<Template[]>([]);
  const [query, setQuery] = useState('');
  const [category, setCategory] = useState('All');
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [targetChannelID, setTargetChannelID] = useState('');

  useEffect(() => {
    setTargetChannelID(new URLSearchParams(window.location.search).get('channel') ?? '');
    listTemplates()
      .then(setTemplates)
      .catch((err) => setError(err instanceof Error ? err.message : t('templatesLoadError')))
      .finally(() => setIsLoading(false));
  }, []);

  const categories = useMemo(
    () => ['All', ...new Set(templates.map((template) => template.category))],
    [templates],
  );
  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return templates.filter((template) => {
      if (category !== 'All' && template.category !== category) return false;
      if (!needle) return true;
      return [
        template.name,
        template.description,
        template.category,
        ...(template.roles ?? []),
      ].some((value) => value.toLowerCase().includes(needle));
    });
  }, [category, query, templates]);

  return (
    <AppFrame>
      <main className="min-h-0 flex-1 overflow-y-auto bg-brutal-cream">
        <section className="border-b-4 border-black bg-brutal-primary-light px-5 py-4 lg:px-8">
          <div className={`mx-auto grid max-w-[1480px] gap-4 lg:items-center ${targetChannelID ? 'lg:grid-cols-[minmax(0,1fr)_360px]' : ''}`}>
            <div>
              <div className="mb-2 inline-flex items-center gap-2 border-2 border-black bg-white px-2 py-1 font-mono text-[10px] font-bold uppercase tracking-widest shadow-brutal-sm">
                <LayoutTemplate className="h-3.5 w-3.5 text-brutal-accent" />
                {t('templatesLibraryEyebrow')}
              </div>
              <h1 className="font-heading text-2xl font-black tracking-tight">{t('templatesTitle')}</h1>
              <p className="mt-1 max-w-2xl font-body text-sm text-black/65">
                {t('templatesDescription')}
              </p>
            </div>
            {targetChannelID && (
              <div className="flex items-center gap-3 border-2 border-black bg-brutal-primary-light p-3 shadow-brutal">
                <span className="flex h-10 w-10 shrink-0 items-center justify-center border-2 border-black bg-white shadow-brutal-sm">
                  <LayoutTemplate className="h-5 w-5 text-brutal-accent" />
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block font-mono text-[10px] font-bold uppercase tracking-widest text-black/55">{t('templatesTargetEyebrow')}</span>
                  <span className="mt-0.5 block font-heading text-base font-black">{t('templatesTargetTitle')}</span>
                  <span className="mt-1 block font-body text-xs leading-relaxed text-black/65">
                    {t('templatesTargetDesc')}
                  </span>
                </span>
              </div>
            )}
          </div>
        </section>

        <div className="mx-auto max-w-[1480px] px-5 py-4 lg:px-8">
          <LucyTeamComposer templates={templates} targetChannelID={targetChannelID} />

          <div className="mb-4 flex flex-col gap-2 border-2 border-black bg-white p-2 shadow-brutal md:flex-row md:items-center">
            <div className="min-w-0 flex-1 md:w-80 md:flex-none">
              <Input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder={t('templatesSearchPlaceholder')}
              />
            </div>
            <div className="flex min-w-0 flex-1 gap-2 overflow-x-auto">
              {categories.map((item) => (
                <button
                  key={item}
                  type="button"
                  onClick={() => setCategory(item)}
                  className={`whitespace-nowrap border-2 border-black px-3 py-2 font-mono text-xs font-bold uppercase ${
                    item === category ? 'bg-brutal-primary shadow-brutal-sm' : 'bg-white hover:bg-brutal-muted/20'
                  }`}
                >
                    {item === 'All' ? t('all') : item}
                </button>
              ))}
            </div>
          </div>

          {isLoading ? (
            <div className="flex justify-center py-24"><Spinner size="md" /></div>
          ) : error ? (
            <div className="border-4 border-black bg-brutal-danger-light p-6 font-mono text-sm shadow-brutal">{error}</div>
          ) : filtered.length === 0 ? (
            <div className="border-4 border-dashed border-black bg-white p-12 text-center">
              <p className="font-heading text-xl font-black">{t('templatesNoMatch')}</p>
              <p className="mt-1 font-body text-sm text-muted-foreground">{t('templatesNoMatchHint')}</p>
            </div>
          ) : (
            <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
              {filtered.map((template, index) => (
                <Link
                  key={template.id}
                  href={`/templates/${encodeURIComponent(template.id)}${targetChannelID ? `?channel=${encodeURIComponent(targetChannelID)}` : ''}`}
                  className="group flex min-h-[192px] flex-col border-2 border-black bg-white p-3 shadow-brutal transition-transform hover:-translate-y-1 hover:shadow-brutal-xl"
                >
                  <div className="flex items-start justify-between gap-3">
                    <span className={`flex h-9 w-9 items-center justify-center border-2 border-black text-lg shadow-brutal-sm ${
                      index % 3 === 0 ? 'bg-brutal-accent-light' : index % 3 === 1 ? 'bg-brutal-primary-light' : 'bg-brutal-success-light'
                    }`}>
                      {template.icon || '✦'}
                    </span>
                    <span className="border-2 border-black bg-brutal-cream px-2 py-1 font-mono text-[10px] font-bold uppercase">
                      {template.category}
                    </span>
                  </div>
                  <h2 className="mt-2 font-heading text-base font-black">{template.name}</h2>
                  <p className="mt-1 line-clamp-2 min-h-9 flex-1 font-body text-xs leading-relaxed text-black/65">{template.description}</p>
                  <div className="mt-2 flex -space-x-1.5" aria-label={t('templateIncludedRoles')}>
                    {(template.avatar_urls ?? []).slice(0, 5).map((avatarUrl, avatarIndex) => (
                      <PixelAvatar
                        key={avatarUrl}
                        agentId={`${template.id}:${avatarIndex}`}
                        avatarUrl={avatarUrl}
                        size="sm"
                        className="bg-brutal-cream"
                      />
                    ))}
                  </div>
                  <div className="mt-2 flex items-end justify-between gap-3 border-t-2 border-black pt-2">
                    <div>
                      <span className="flex items-center gap-1.5 font-mono text-xs font-bold">
                        <Users className="h-3.5 w-3.5" />
                        {t('templatesRoleCount', { n: template.member_count })}
                      </span>
                      <p className="mt-1 line-clamp-1 font-mono text-[10px] uppercase text-black/50">
                        {(template.roles ?? []).join(' · ')}
                      </p>
                    </div>
                    <ArrowRight className="h-5 w-5 transition-transform group-hover:translate-x-1" />
                  </div>
                </Link>
              ))}
            </div>
          )}
        </div>
      </main>
    </AppFrame>
  );
}
