'use client';

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { useParams, useRouter } from 'next/navigation';
import { ArrowLeft, ArrowLeftRight, ArrowRight, GitFork, Handshake, Network, Users } from 'lucide-react';
import { AppFrame } from '@/components/layout/app-frame';
import { TemplateGraph } from '@/components/templates/template-graph';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { Spinner } from '@/components/ui/spinner';
import { Textarea } from '@/components/ui/textarea';
import { apiClient } from '@/lib/api-client';
import { getLocale, t } from '@/lib/i18n';
import { getTemplate, type Template, type TemplateRelationship } from '@/lib/templates-api';
import type { Channel } from '@/lib/types';

export default function TemplateDetailPage() {
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const [template, setTemplate] = useState<Template | null>(null);
  const [selectedRef, setSelectedRef] = useState<string | null>(null);
  const [selectedRelationship, setSelectedRelationship] = useState<TemplateRelationship | null>(null);
  const [selectedRelationshipIndex, setSelectedRelationshipIndex] = useState<number | null>(null);
  const [channelName, setChannelName] = useState('');
  const [description, setDescription] = useState('');
  const [isCreating, setIsCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [targetChannelID, setTargetChannelID] = useState('');
  const [targetChannel, setTargetChannel] = useState<Channel | null>(null);

  useEffect(() => {
    const targetID = new URLSearchParams(window.location.search).get('channel') ?? '';
    setTargetChannelID(targetID);
    if (targetID) {
      apiClient.get<Channel>(`/api/v1/channels/${encodeURIComponent(targetID)}`)
        .then(setTargetChannel)
        .catch((err) => setError(err instanceof Error ? err.message : t('templateTargetLoadError')));
    }

    const id = decodeURIComponent(params.id);
    getTemplate(id)
      .then((result) => {
        setTemplate(result);
        setSelectedRef(result.members?.[0]?.ref ?? null);
        setSelectedRelationship(null);
        setSelectedRelationshipIndex(null);
        setChannelName(slugify(result.name));
        setDescription(result.description);
      })
      .catch((err) => setError(err instanceof Error ? err.message : t('templateNotFound')));
  }, [params.id]);

  const selectedMember = useMemo(
    () => template?.members?.find((member) => member.ref === selectedRef),
    [selectedRef, template],
  );
  const selectedRelationshipMembers = useMemo(() => ({
    from: template?.members?.find((member) => member.ref === selectedRelationship?.from_ref),
    to: template?.members?.find((member) => member.ref === selectedRelationship?.to_ref),
  }), [selectedRelationship, template]);

  const createOrApplyTeam = async () => {
    if (!template || (!targetChannelID && !channelName.trim()) || isCreating) return;
    setIsCreating(true);
    setError(null);
    try {
      if (targetChannelID) {
        await apiClient.post(`/api/v1/channels/${encodeURIComponent(targetChannelID)}/template`, {
          template_id: template.id,
          locale: getLocale(),
        });
        router.push(`/dashboard?channel=${encodeURIComponent(targetChannelID)}`);
        return;
      }
      const channel = await apiClient.post<Channel>('/api/v1/channels', {
        name: channelName.trim(),
        description: description.trim(),
        template_id: template.id,
        locale: getLocale(),
      });
      router.push(`/dashboard?channel=${channel.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : t(targetChannelID ? 'templateApplyError' : 'templateCreateError'));
      setIsCreating(false);
    }
  };

  return (
    <AppFrame>
      <main className="flex min-h-0 flex-1 flex-col overflow-hidden bg-brutal-cream">
        <header className="flex flex-shrink-0 items-center gap-4 border-b-4 border-black bg-white px-5 py-3">
          <Link
            href={`/templates${targetChannelID ? `?channel=${encodeURIComponent(targetChannelID)}` : ''}`}
            className="flex h-9 w-9 items-center justify-center border-2 border-black bg-white shadow-brutal-sm hover:-translate-y-px"
            aria-label={t('templateBack')}
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div className="min-w-0 flex-1">
            <div className="font-mono text-[10px] font-bold uppercase tracking-widest text-black/50">{t('templateOfficial')}</div>
            <h1 className="truncate font-heading text-xl font-black">{template?.name ?? t('templateLoading')}</h1>
          </div>
          {template && (
            <span className="border-2 border-black bg-brutal-primary-light px-2 py-1 font-mono text-[10px] font-bold uppercase">
              {template.category}
            </span>
          )}
        </header>

        {!template && !error ? (
          <div className="flex flex-1 items-center justify-center"><Spinner size="md" /></div>
        ) : !template ? (
          <div className="m-8 border-4 border-black bg-brutal-danger-light p-6 shadow-brutal">{error}</div>
        ) : (
          <div className="grid min-h-0 flex-1 lg:grid-cols-[minmax(0,1fr)_410px]">
            <section className="flex min-h-[520px] flex-col border-b-4 border-black bg-brutal-cream lg:min-h-0 lg:border-b-0 lg:border-r-4">
              <div className="flex items-center justify-between border-b-2 border-black bg-white px-4 py-3">
                <div>
                  <h2 className="flex items-center gap-2 font-heading text-sm font-black uppercase tracking-wider">
                    <Network className="h-4 w-4" />
                    {t('templatePreviewTitle')}
                  </h2>
                  <p className="mt-0.5 font-mono text-[10px] text-black/55">{t('templatePreviewDesc')}</p>
                </div>
                <div className="flex items-center gap-3">
                    <span className="flex items-center gap-1.5 rounded-md border border-[var(--skin-rule)] bg-[var(--skin-surface)] px-2 py-1 font-mono text-[9px] font-bold uppercase text-black/65 shadow-[var(--archive-shadow-sm)]">
                      <GitFork className="h-3.5 w-3.5 text-brutal-accent" />
                    {t('assignsTo')}
                  </span>
                  {template.relationships?.some((relationship) => relationship.type === 'collaborates_with') && (
                    <span className="flex items-center gap-1.5 rounded-md border border-[var(--skin-rule)] bg-[var(--skin-surface)] px-2 py-1 font-mono text-[9px] font-bold uppercase text-black/65 shadow-[var(--archive-shadow-sm)]">
                      <Handshake className="h-3.5 w-3.5 text-brutal-success" />
                      {t('collaboratesWith')}
                    </span>
                  )}
                  <span className="flex items-center gap-1.5 border-2 border-black bg-white px-2 py-1 font-mono text-xs font-bold">
                    <Users className="h-3.5 w-3.5" />
                    {template.member_count}
                  </span>
                </div>
              </div>
              <div className="min-h-0 flex-1">
                <TemplateGraph
                  template={template}
                  selectedRef={selectedRef}
                  selectedRelationshipIndex={selectedRelationshipIndex}
                  onSelect={(ref) => {
                    setSelectedRef(ref);
                    setSelectedRelationship(null);
                    setSelectedRelationshipIndex(null);
                  }}
                  onSelectRelationship={(relationship, index) => {
                    setSelectedRef(null);
                    setSelectedRelationship(relationship);
                    setSelectedRelationshipIndex(index);
                  }}
                />
              </div>
              {selectedMember && (
                <div className="border-t-4 border-black bg-white p-4">
                  <div className="flex items-start gap-4">
                    <div className="flex min-w-48 items-start gap-3">
                      <PixelAvatar
                        agentId={`${template.id}:${selectedMember.ref}`}
                        avatarUrl={selectedMember.avatar_url}
                        size="md"
                        ariaLabel={selectedMember.name}
                      />
                      <div className="min-w-0">
                        <div className="font-mono text-[10px] font-bold uppercase tracking-wider text-brutal-accent">{selectedMember.role}</div>
                        <div className="font-heading text-base font-black">{selectedMember.name}</div>
                        <p className="mt-1 font-body text-xs text-black/60">{selectedMember.description}</p>
                      </div>
                    </div>
                    <div className="border-l-2 border-black pl-4">
                      <div className="font-mono text-[10px] font-bold uppercase tracking-wider text-black/50">{t('templateRoleInstructions')}</div>
                      <p className="mt-1 font-body text-sm leading-relaxed">{selectedMember.instructions}</p>
                    </div>
                  </div>
                </div>
              )}
              {selectedRelationship && selectedRelationshipMembers.from && selectedRelationshipMembers.to && (
                <div className="border-t-4 border-black bg-white p-4">
                  <div className="flex items-start gap-5">
                    <div className="flex min-w-64 items-center gap-3">
                      <PixelAvatar
                        agentId={`${template.id}:${selectedRelationshipMembers.from.ref}`}
                        avatarUrl={selectedRelationshipMembers.from.avatar_url}
                        size="sm"
                        ariaLabel={selectedRelationshipMembers.from.name}
                      />
                      <div className="min-w-0 font-heading text-sm font-black">
                        {selectedRelationshipMembers.from.name}
                      </div>
                      {selectedRelationship.type === 'assigns_to'
                        ? <ArrowRight className="h-4 w-4 shrink-0 text-brutal-accent" />
                        : <ArrowLeftRight className="h-4 w-4 shrink-0 text-brutal-success" />}
                      <PixelAvatar
                        agentId={`${template.id}:${selectedRelationshipMembers.to.ref}`}
                        avatarUrl={selectedRelationshipMembers.to.avatar_url}
                        size="sm"
                        ariaLabel={selectedRelationshipMembers.to.name}
                      />
                      <div className="min-w-0 font-heading text-sm font-black">
                        {selectedRelationshipMembers.to.name}
                      </div>
                    </div>
                    <div className="min-w-0 flex-1 border-l-2 border-black pl-4">
                      <div className="font-mono text-[10px] font-bold uppercase tracking-wider text-black/50">
                        {t(selectedRelationship.type === 'assigns_to'
                          ? 'relationshipCriteriaDelegation'
                          : 'relationshipCriteriaCollaboration')}
                      </div>
                      <p className="mt-1 whitespace-pre-wrap font-body text-sm leading-relaxed">
                        {selectedRelationship.instruction || t(selectedRelationship.type === 'assigns_to'
                          ? 'relationshipNoDelegationCriteria'
                          : 'relationshipNoCollaborationCriteria')}
                      </p>
                    </div>
                  </div>
                </div>
              )}
            </section>

            <aside className="overflow-y-auto bg-white p-5">
              <span className="inline-block border-2 border-black bg-brutal-primary-light px-2 py-1 font-mono text-[10px] font-bold uppercase shadow-brutal-sm">
                {t(targetChannelID ? 'templateApplyFresh' : 'templateCreateFresh')}
              </span>
              <h2 className="mt-4 font-heading text-2xl font-black">
                {targetChannelID
                  ? t('templateAddToChannel', { name: targetChannel?.name ?? t('templateTargetFallback') })
                  : t('templateMakeChannel')}
              </h2>
              <p className="mt-2 font-body text-sm leading-relaxed text-black/65">{template.description}</p>

              <div className="my-5 border-y-2 border-black py-4">
                <p className="font-mono text-[10px] font-bold uppercase tracking-widest text-black/50">{t('templateIncludedRoles')}</p>
                <div className="mt-2 flex flex-wrap gap-2">
                  {(template.roles ?? []).map((role) => (
                    <span key={role} className="border-2 border-black bg-brutal-cream px-2 py-1 font-mono text-[10px] font-bold">{role}</span>
                  ))}
                </div>
              </div>

              <div className="space-y-4">
                {!targetChannelID && (
                  <>
                    <label className="block">
                      <span className="mb-1.5 block font-heading text-xs font-black uppercase tracking-wider">{t('templateChannelName')}</span>
                      <Input
                        value={channelName}
                        onChange={(event) => setChannelName(slugify(event.target.value))}
                        placeholder="project-name"
                      />
                    </label>
                    <label className="block">
                      <span className="mb-1.5 block font-heading text-xs font-black uppercase tracking-wider">{t('templateGoalOptional')}</span>
                      <Textarea
                        value={description}
                        onChange={(event) => setDescription(event.target.value)}
                        rows={4}
                        className="!h-28 resize-y font-body leading-relaxed"
                        maxLength={200}
                      />
                    </label>
                  </>
                )}
                {error && <p className="border-2 border-black bg-brutal-danger-light p-2 font-mono text-xs">{error}</p>}
                <Button
                  type="button"
                  variant="success"
                  className="w-full justify-between"
                  disabled={(!targetChannelID && !channelName.trim()) || isCreating}
                  onClick={createOrApplyTeam}
                >
                  {isCreating
                    ? t(targetChannelID ? 'templateApplyingTeam' : 'templateCreatingTeam')
                    : t(targetChannelID ? 'templateApplyTeam' : 'templateCreateChannelTeam')}
                  {!isCreating && <ArrowRight className="h-4 w-4" />}
                </Button>
                <p className="font-mono text-[10px] leading-relaxed text-black/45">
                  {t(targetChannelID ? 'templateApplyHint' : 'templateFineTuneHint')}
                </p>
              </div>
            </aside>
          </div>
        )}
      </main>
    </AppFrame>
  );
}

function slugify(value: string) {
  return value
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9_-]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 80);
}
