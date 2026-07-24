'use client';

import Link from 'next/link';
import { useRef, useState, type MutableRefObject } from 'react';
import { useRouter } from 'next/navigation';
import { ArrowRight, LayoutTemplate, Sparkles, Users } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { Textarea } from '@/components/ui/textarea';
import { apiClient } from '@/lib/api-client';
import { getLocale, t } from '@/lib/i18n';
import type { Template } from '@/lib/templates-api';
import type { Channel } from '@/lib/types';

interface MessageResponse {
  id: string;
  sender_type: 'user' | 'agent' | 'system';
  content: string;
  created_at: string;
}

interface MessageListResponse {
  messages: MessageResponse[];
}

interface LucyRecommendation {
  reply: string;
  template: Template;
}

const POLL_INTERVAL_MS = 1500;
const POLL_TIMEOUT_MS = 120000;

export function LucyTeamComposer({
  templates,
  targetChannelID,
}: {
  templates: Template[];
  targetChannelID: string;
}) {
  const router = useRouter();
  const requestIDRef = useRef(0);
  const [goal, setGoal] = useState('');
  const [channelName, setChannelName] = useState('');
  const [recommendation, setRecommendation] = useState<LucyRecommendation | null>(null);
  const [isMatching, setIsMatching] = useState(false);
  const [isCreating, setIsCreating] = useState(false);
  const [createdChannelID, setCreatedChannelID] = useState('');
  const [error, setError] = useState('');

  const askLucy = async () => {
    const trimmedGoal = goal.trim();
    if (!trimmedGoal || isMatching || templates.length === 0) return;

    const requestID = ++requestIDRef.current;
    setIsMatching(true);
    setRecommendation(null);
    setCreatedChannelID('');
    setError('');

    try {
      const lucyChannel = await apiClient.get<Channel>('/api/v1/channels/lucy');
      const source = await apiClient.post<MessageResponse>(
        `/api/v1/channels/${encodeURIComponent(lucyChannel.id)}/messages`,
        { content: recommendationPrompt(trimmedGoal) },
      );
      const reply = await waitForLucyReply(lucyChannel.id, source.id, requestID, requestIDRef);
      if (requestID !== requestIDRef.current) return;

      const template = matchTemplate(reply.content, templates);
      if (!template) {
        setError(t('templatesLucyUnrecognized'));
        return;
      }
      setRecommendation({ reply: reply.content, template });
      setChannelName(slugify(template.name));
    } catch (err) {
      if (requestID === requestIDRef.current) {
        setError(err instanceof Error ? err.message : t('templatesLucyRecommendError'));
      }
    } finally {
      if (requestID === requestIDRef.current) setIsMatching(false);
    }
  };

  const createAndStart = async () => {
    if (!recommendation || isCreating) return;
    if (!targetChannelID && !channelName.trim()) return;

    setIsCreating(true);
    setError('');
    let channelID = createdChannelID;
    try {
      if (!channelID) {
        if (targetChannelID) {
          await apiClient.post(`/api/v1/channels/${encodeURIComponent(targetChannelID)}/template`, {
            template_id: recommendation.template.id,
            locale: getLocale(),
          });
          channelID = targetChannelID;
        } else {
          const channel = await apiClient.post<Channel>('/api/v1/channels', {
            name: channelName.trim(),
            description: goal.trim(),
            template_id: recommendation.template.id,
            locale: getLocale(),
          });
          channelID = channel.id;
        }
        setCreatedChannelID(channelID);
      }

      await apiClient.post(`/api/v1/channels/${encodeURIComponent(channelID)}/messages`, {
        content: goal.trim(),
      });
      router.push(`/dashboard?channel=${encodeURIComponent(channelID)}`);
    } catch (err) {
      const fallback = channelID ? t('templatesLucyStartError') : t('templatesLucyCreateError');
      setError(`${fallback}${err instanceof Error ? ` ${err.message}` : ''}`);
      setIsCreating(false);
    }
  };

  return (
    <section className="mx-auto mb-4 max-w-6xl border-2 border-black bg-brutal-accent-light shadow-brutal">
      <div className="grid gap-3 p-3 lg:grid-cols-[150px_minmax(0,1fr)_auto] lg:items-center">
        <div>
          <div className="flex items-center gap-2 font-heading text-base font-black">
            <span className="flex h-9 w-9 items-center justify-center border-2 border-black bg-white shadow-brutal-sm">
              <Sparkles className="h-4 w-4 text-brutal-accent" />
            </span>
            {t('templatesLucyComposerTitle')}
          </div>
        </div>
        <Textarea
          value={goal}
          onChange={(event) => setGoal(event.target.value)}
          placeholder={t('templatesLucyComposerPlaceholder')}
          rows={2}
          disabled={isMatching || isCreating}
          className="min-h-14 resize-none bg-white"
        />
        <Button
          type="button"
          onClick={askLucy}
          disabled={!goal.trim() || isMatching || isCreating || templates.length === 0}
          className="h-14 min-w-36"
        >
          <Sparkles className="h-4 w-4" />
          {isMatching ? t('templatesLucyMatching') : t('templatesLucyRecommend')}
        </Button>
      </div>

      {(recommendation || error || isMatching) && (
        <div className="border-t-2 border-black bg-brutal-cream p-3">
          {isMatching && (
            <div className="flex items-center gap-2 font-mono text-xs font-bold uppercase tracking-wider text-black/60">
              <span className="h-2.5 w-2.5 animate-pulse rounded-full bg-brutal-accent" />
              {t('templatesLucyWaiting')}
            </div>
          )}

          {recommendation && (
            <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_minmax(320px,0.72fr)]">
              <div className="border-l-4 border-brutal-accent bg-white px-4 py-3">
                <div className="font-mono text-[10px] font-bold uppercase tracking-widest text-brutal-accent">
                  {t('templatesLucyReplyLabel')}
                </div>
                <p className="mt-2 whitespace-pre-wrap font-body text-sm leading-relaxed text-black/75">
                  {recommendation.reply}
                </p>
              </div>

              <div className="border-2 border-black bg-white p-3 shadow-brutal-sm">
                <div className="flex items-start justify-between gap-3">
                  <span className="flex h-10 w-10 items-center justify-center border-2 border-black bg-brutal-primary-light text-lg shadow-brutal-sm">
                    {recommendation.template.icon || '✦'}
                  </span>
                  <span className="border-2 border-black bg-brutal-cream px-2 py-1 font-mono text-[10px] font-bold uppercase">
                    {recommendation.template.category}
                  </span>
                </div>
                <div className="mt-2 font-mono text-[10px] font-bold uppercase tracking-widest text-black/45">
                  {t('templatesLucyRecommendedLabel')}
                </div>
                <h3 className="mt-0.5 font-heading text-lg font-black">{recommendation.template.name}</h3>
                <p className="mt-1 font-body text-xs leading-relaxed text-black/60">
                  {recommendation.template.description}
                </p>
                <div className="mt-3 flex items-center justify-between gap-3 border-t-2 border-black pt-3">
                  <div className="flex items-center gap-3">
                    <div className="flex -space-x-1.5">
                      {(recommendation.template.avatar_urls ?? []).slice(0, 5).map((avatarURL, index) => (
                        <PixelAvatar
                          key={avatarURL}
                          agentId={`${recommendation.template.id}:${index}`}
                          avatarUrl={avatarURL}
                          size="sm"
                          className="bg-brutal-cream"
                        />
                      ))}
                    </div>
                    <span className="flex items-center gap-1 font-mono text-[10px] font-bold uppercase">
                      <Users className="h-3.5 w-3.5" />
                      {t('templatesRoleCount', { n: recommendation.template.member_count })}
                    </span>
                  </div>
                  <Link
                    href={`/templates/${encodeURIComponent(recommendation.template.id)}${targetChannelID ? `?channel=${encodeURIComponent(targetChannelID)}` : ''}`}
                    className="flex items-center gap-1 font-mono text-[10px] font-bold uppercase hover:text-brutal-accent"
                  >
                    {t('templatesLucyViewTeam')}
                    <ArrowRight className="h-3.5 w-3.5" />
                  </Link>
                </div>

                {!targetChannelID && (
                  <label className="mt-3 block">
                    <span className="mb-1 block font-mono text-[10px] font-bold uppercase tracking-widest">
                      {t('templateChannelName')}
                    </span>
                    <Input
                      value={channelName}
                      onChange={(event) => setChannelName(slugify(event.target.value))}
                      disabled={isCreating || Boolean(createdChannelID)}
                    />
                  </label>
                )}

                <Button
                  type="button"
                  onClick={createAndStart}
                  disabled={isCreating || (!targetChannelID && !channelName.trim())}
                  className="mt-3 w-full"
                >
                  {isCreating ? t('templatesLucyCreating') : createdChannelID ? t('templatesLucyRetryStart') : t('templatesLucyCreateStart')}
                  <ArrowRight className="h-4 w-4" />
                </Button>

                {createdChannelID && error && (
                  <Link
                    href={`/dashboard?channel=${encodeURIComponent(createdChannelID)}`}
                    className="mt-2 flex items-center justify-center gap-1 font-mono text-[10px] font-bold uppercase hover:text-brutal-accent"
                  >
                    {t('templatesLucyOpenCreated')}
                    <LayoutTemplate className="h-3.5 w-3.5" />
                  </Link>
                )}
              </div>
            </div>
          )}

          {error && (
            <p className="mt-3 border-2 border-black bg-brutal-danger-light px-3 py-2 font-body text-xs">
              {error}
            </p>
          )}
        </div>
      )}
    </section>
  );
}

async function waitForLucyReply(
  channelID: string,
  sourceMessageID: string,
  requestID: number,
  requestIDRef: MutableRefObject<number>,
): Promise<MessageResponse> {
  const deadline = Date.now() + POLL_TIMEOUT_MS;
  while (Date.now() < deadline) {
    await new Promise((resolve) => window.setTimeout(resolve, POLL_INTERVAL_MS));
    if (requestID !== requestIDRef.current) throw new Error(t('templatesLucyRecommendCancelled'));

    const result = await apiClient.get<MessageListResponse>(
      `/api/v1/channels/${encodeURIComponent(channelID)}/messages`,
      { limit: '100' },
    );
    const sourceIndex = result.messages.findIndex((message) => message.id === sourceMessageID);
    if (sourceIndex >= 0) {
      const reply = result.messages.slice(sourceIndex + 1).find((message) => message.sender_type === 'agent');
      if (reply) return reply;
    }
  }
  throw new Error(t('templatesLucyTimeout'));
}

function matchTemplate(content: string, templates: Template[]): Template | undefined {
  const normalized = content.toLocaleLowerCase();
  return templates
    .map((template) => {
      const positions = [template.id, template.name]
        .map((value) => normalized.indexOf(value.toLocaleLowerCase()))
        .filter((position) => position >= 0);
      return { template, position: positions.length > 0 ? Math.min(...positions) : -1 };
    })
    .filter(({ position }) => position >= 0)
    .sort((a, b) => a.position - b.position)[0]?.template;
}

function recommendationPrompt(goal: string): string {
  if (getLocale().startsWith('zh')) {
    return `请根据下面的目标，从 Solo 内置模板中只推荐一个最合适的团队。现在只推荐，不要创建频道或团队。请简短说明理由，并在回答中明确写出模板 ID。\n\n目标：${goal}`;
  }
  return `Recommend exactly one team from Solo's built-in templates for the goal below. Recommend only; do not create a Channel or team yet. Briefly explain why and include the exact template ID in your answer.\n\nGoal: ${goal}`;
}

function slugify(value: string): string {
  return value
    .trim()
    .toLocaleLowerCase()
    .replace(/[^\p{L}\p{N}]+/gu, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 100);
}
