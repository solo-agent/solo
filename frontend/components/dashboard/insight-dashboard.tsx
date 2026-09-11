'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { BarChart3, Bot, CalendarDays, CheckSquare, MessageSquare } from 'lucide-react';
import { apiClient } from '@/lib/api-client';
import { Spinner } from '@/components/ui/spinner';
import { DashboardTopTabs } from '@/components/dashboard/live-monitor';
import { tabButtonClass } from '@/components/ui/tab-bar';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';

interface DeliveryCohort {
  cohort: string; models: string[]; budgets: unknown[]; tasks: number; qualified: number;
  comparable_tasks: number; comparable_qualified: number; comparable_tokens: number;
  actual_tokens: number; accounted_tokens: number; unknown_runs: number; shared_runs: number; active_runs: number; failed_runs: number;
  execution_seconds: number; qualified_elapsed_seconds: number; human_minutes: number; human_recorded_tasks: number;
  reworks: number; rework_reasons: Record<string, number>; observations: Record<string, number>;
  recovery_samples: number; recovery_seconds: number; first_actions_recorded: number; first_actions_correct: number;
}

interface DashboardInsight {
  delivery?: DeliveryCohort[];
  since: string;
  generated_at: string;
  messages: number;
  agent_runs: number;
  tasks: number;
  tokens: {
    input: number;
    output: number;
    total: number;
  };
  series?: DashboardSeriesPoint[];
  run_status: DashboardCount[];
  task_status: DashboardCount[];
  agent_usage: DashboardUsageCount[];
  task_usage: DashboardTaskUsage[];
  terms?: DashboardCount[];
}

interface DashboardSeriesPoint {
  date: string;
  messages: number;
  agent_runs: number;
  tasks: number;
  tokens: number;
}

interface DashboardCount {
  key: string;
  label: string;
  count: number;
}

interface DashboardUsageCount {
  id: string;
  name: string;
  count: number;
  last_at?: string;
}

interface DashboardTaskUsage {
  id: string;
  task_number: number;
  title: string;
  status: string;
  count: number;
  last_at?: string;
}

type SeriesKey = 'agent_runs' | 'messages' | 'tokens' | 'tasks';

const RANGE_OPTIONS = [
  { label: t('observabilityRangeToday'), days: 1 },
  { label: t('observabilityRangeWeek'), days: 7 },
  { label: t('observabilityRangeMonth'), days: 30 },
  { label: t('observabilityRangeAll'), days: 3650 },
];

const SERIES_CONFIG: Record<SeriesKey, { label: string; color: string }> = {
  agent_runs: { label: t('observabilitySeriesSessions'), color: 'var(--color-brutal-info)' },
  messages: { label: t('observabilitySeriesMessages'), color: 'var(--color-brutal-success)' },
  tokens: { label: t('observabilitySeriesTokens'), color: 'var(--color-brutal-accent)' },
  tasks: { label: t('observabilitySeriesTasks'), color: 'var(--color-brutal-violet)' },
};

const WORD_COLORS = [
  'color-mix(in srgb, var(--color-brutal-info) 72%, var(--skin-ink))',
  'color-mix(in srgb, var(--color-brutal-success) 72%, var(--skin-ink))',
  'color-mix(in srgb, var(--color-brutal-accent) 72%, var(--skin-ink))',
  'color-mix(in srgb, var(--color-brutal-violet) 72%, var(--skin-ink))',
  'color-mix(in srgb, var(--color-brutal-warning) 72%, var(--skin-ink))',
];
const CHART_LEFT = 72;
const CHART_TOP = 32;
const CHART_BOTTOM = 204;

export function InsightDashboard() {
  const [data, setData] = useState<DashboardInsight | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [windowDays, setWindowDays] = useState(7);

  useEffect(() => {
    let cancelled = false;
    setIsLoading(true);
    apiClient.get<DashboardInsight>('/api/v1/dashboard/insight', { window_days: String(windowDays) })
      .then((next) => {
        if (!cancelled) setData(next);
      })
      .catch(() => {
        if (!cancelled) setData(null);
      })
      .finally(() => {
        if (!cancelled) setIsLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [windowDays]);

  return (
    <div className="h-full min-h-0 overflow-auto bg-brutal-cream animate-fade-in">
      <DashboardTopTabs active="insight" />
      <div className="border-b-2 border-black bg-brutal-cream px-5 py-4">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h1 className="font-heading text-2xl font-black">{t('observabilityInsight')}</h1>
            <p className="mt-1 font-body text-sm text-muted-foreground">{t('observabilityInsightDesc')}</p>
          </div>
          <TimeRangeSelector value={windowDays} onChange={setWindowDays} />
        </div>
        {data && (
          <div className="mt-3 inline-flex items-center gap-2 border-2 border-black bg-white px-3 py-1 font-mono text-xs shadow-brutal-sm">
            <CalendarDays className="h-4 w-4" />
            {t('observabilityDataRange', { from: formatShortDate(data.since), to: formatShortDate(data.generated_at) })}
          </div>
        )}
      </div>

      {isLoading ? (
        <div className="flex h-64 items-center justify-center">
          <Spinner size="md" />
        </div>
      ) : !data ? (
        <div className="flex h-64 items-center justify-center font-body text-sm text-muted-foreground">{t('observabilityNoStats')}</div>
      ) : (
        <div className="space-y-5 p-5">
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            <InsightMetric icon={<MessageSquare className="h-5 w-5" />} label={t('observabilityMetricMessages')} value={data.messages} />
            <InsightMetric icon={<Bot className="h-5 w-5" />} label={t('observabilityMetricAgentRuns')} value={data.agent_runs} />
            <InsightMetric icon={<CheckSquare className="h-5 w-5" />} label={t('observabilityMetricTasks')} value={data.tasks} />
            <InsightMetric icon={<BarChart3 className="h-5 w-5" />} label={t('observabilityMetricTokens')} value={data.tokens.total} />
          </div>

          <InsightPanel title="合格交付与长期成本">
            <p className="mb-3 font-body text-sm leading-relaxed text-muted-foreground">按任务创建时间选择样本，累计其全部执行、审核、失败和返工。相同任务组、实际模型与预算快照才放在一起；未填人工时间表示未知。共享 Run 在组内只计一次，各组存在共享时不能直接相加。</p>
            <div className="overflow-x-auto rounded-lg border border-border" data-testid="delivery-cohorts">
              <table className="w-full min-w-[56rem] text-left font-body text-xs leading-relaxed tabular-nums"><thead className="bg-brutal-primary-light"><tr>{['任务组与条件', '交付与成本', '人工与返工', '恢复与协作'].map((label) => <th key={label} scope="col" className="border-b border-border p-3 font-heading text-xs">{label}</th>)}</tr></thead>
                <tbody>{(data.delivery ?? []).map((group, index) => <tr key={index} className="border-b border-border last:border-0 align-top">
                  <td className="w-1/4 max-w-64 break-words p-3"><p className="font-bold">{group.cohort || '未设置对照组'}</p><p>{group.models.join(', ') || '尚无运行'}</p><details className="mt-2"><summary className="cursor-pointer font-heading font-bold text-muted-foreground">运行时预算</summary><pre className="mt-2 max-h-48 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-brutal-primary-light p-2 font-mono text-xs">{JSON.stringify(group.budgets, null, 2)}</pre><p className="text-xs">null 为历史未知；空对象表示当时未启用额度。</p></details></td>
                  <td className="w-1/4 p-3"><p>合格 {group.qualified} / {group.tasks} 个任务</p><p>已确认 Token {group.actual_tokens.toLocaleString()} · 含预留 {group.accounted_tokens.toLocaleString()}</p><p>失败 {group.failed_runs} · 未结束 {group.active_runs} · 未知用量 {group.unknown_runs} · 共享 {group.shared_runs}</p><p>可比样本 {group.comparable_tasks} · 排除 {group.tasks - group.comparable_tasks}</p><p>{group.comparable_qualified ? `每次合格交付 ${(group.comparable_tokens / group.comparable_qualified).toFixed(0)} Token（含同组失败成本）` : '尚无可比的合格交付'}</p><p>执行 {(group.execution_seconds / 60).toFixed(1)} 分钟 · {group.qualified ? `平均验收历时 ${(group.qualified_elapsed_seconds / group.qualified / 60).toFixed(1)} 分钟` : '无验收历时'}</p></td>
                  <td className="w-1/4 p-3"><p>已记录人工 {group.human_minutes} 分钟 · 覆盖 {group.human_recorded_tasks} / {group.tasks} 个任务</p><p>退回 {group.reworks} 次</p>{Object.entries(group.rework_reasons).map(([reason, count]) => <p key={reason}>{reason}：{count} 次{count > 1 ? '（重复出现）' : ''}</p>)}</td>
                  <td className="w-1/4 p-3"><p>重复工作 {group.observations.duplicate_work ?? 0} · 重复解释 {group.observations.reexplanation ?? 0} · 额外交接 {group.observations.handoff ?? 0}</p><p>恢复到首个工具：{group.recovery_samples ? `${(group.recovery_seconds / group.recovery_samples).toFixed(1)} 秒，${group.recovery_samples} 个样本` : '尚无样本'}</p><p>首个动作正确 {group.first_actions_correct} / {group.first_actions_recorded} 次人工观察</p></td>
                </tr>)}</tbody>
              </table>
              {!data.delivery?.length && <p className="p-4 font-body text-sm text-muted-foreground">该时间范围还没有可见任务。</p>}
            </div>
          </InsightPanel>

          <InsightPanel title={t('observabilityTrendStats')}>
            <TrendChart points={data.series ?? []} />
          </InsightPanel>

          <div className="grid gap-5 xl:grid-cols-3">
            <InsightPanel title={t('observabilityTaskStatus')}>
              <CountBars items={data.task_status ?? []} />
            </InsightPanel>
            <InsightPanel title={t('observabilityAgentUsageTop')}>
              <UsageBars items={(data.agent_usage ?? []).filter((item) => item.count > 0).slice(0, 5).map((item) => ({
                id: item.id,
                label: item.name,
                detail: formatDate(item.last_at),
                count: item.count,
                color: 'var(--color-brutal-info)',
              }))} />
            </InsightPanel>
            <InsightPanel title={t('observabilityTaskUsageTop')}>
              <UsageBars items={(data.task_usage ?? []).slice(0, 5).map((item) => ({
                id: item.id,
                label: `#${item.task_number} ${item.title}`,
                detail: `${item.status} · ${formatDate(item.last_at)}`,
                count: item.count,
                color: taskColor(item.status),
              }))} />
            </InsightPanel>
          </div>

          <InsightPanel title={t('observabilityWordCloud')}>
            <WordCloud terms={data.terms ?? []} />
          </InsightPanel>
        </div>
      )}
    </div>
  );
}

function TimeRangeSelector({ value, onChange }: { value: number; onChange: (value: number) => void }) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      {RANGE_OPTIONS.map((item) => (
        <button
          key={item.days}
          type="button"
          className={tabButtonClass(value === item.days, 'h-9 text-sm')}
          onClick={() => onChange(item.days)}
        >
          {item.label}
        </button>
      ))}
    </div>
  );
}

function InsightMetric({ icon, label, value }: { icon: ReactNode; label: string; value: number }) {
  return (
    <div className="border-2 border-black bg-white p-4 shadow-brutal-sm">
      <div className="flex items-center justify-between">
        <span className="font-heading text-sm font-black">{label}</span>
        <span className="flex h-8 w-8 items-center justify-center border-2 border-black bg-white">
          {icon}
        </span>
      </div>
      <div className="mt-3 font-heading text-3xl font-black">{formatNumber(value)}</div>
    </div>
  );
}

function InsightPanel({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="overflow-hidden border-2 border-black bg-white shadow-brutal-sm">
      <div className="flex items-center gap-2 border-b-2 border-black bg-brutal-cream px-3 py-2">
        <h2 className="font-heading text-base font-black">{title}</h2>
      </div>
      <div className="p-3">{children}</div>
    </section>
  );
}

function TrendChart({ points }: { points: DashboardSeriesPoint[] }) {
  const chartRef = useRef<SVGSVGElement>(null);
  const [chartWidth, setChartWidth] = useState(760);
  const [visible, setVisible] = useState<Record<SeriesKey, boolean>>({
    agent_runs: true,
    messages: true,
    tokens: false,
    tasks: false,
  });
  const activeKeys = (Object.keys(SERIES_CONFIG) as SeriesKey[]).filter((key) => visible[key]);
  const maxValue = useMemo(() => trendMax(points, activeKeys), [activeKeys, points]);
  const paths = useMemo(() => activeKeys.map((key) => trendPath(points, key, maxValue, chartWidth)), [activeKeys, chartWidth, maxValue, points]);
  const ticks = useMemo(() => trendTicks(maxValue), [maxValue]);

  useEffect(() => {
    const el = chartRef.current;
    if (!el) return;
    const updateWidth = () => setChartWidth(Math.max(360, Math.round(el.clientWidth)));
    updateWidth();
    const observer = new ResizeObserver(updateWidth);
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  if (!points.length) return <EmptyText text={t('observabilityNoTrendData')} />;

  return (
    <div>
      <div className="mb-3 flex flex-wrap items-center gap-2">
        {(Object.keys(SERIES_CONFIG) as SeriesKey[]).map((key) => {
          const item = SERIES_CONFIG[key];
          return (
            <button
              key={key}
              type="button"
              aria-pressed={visible[key]}
              onClick={() => setVisible((current) => ({ ...current, [key]: !current[key] }))}
              className={cn(
                'tab-button inline-flex items-center gap-2 border-2 border-black bg-white px-2 py-1 font-mono text-xs shadow-brutal-sm transition-[transform,box-shadow,opacity] hover:-translate-y-px hover:shadow-brutal active:translate-x-0.5 active:translate-y-0.5 active:shadow-none',
                !visible[key] && 'opacity-40',
              )}
            >
              <span className="h-3 w-3 border-2 border-black" style={{ backgroundColor: item.color }} />
              {item.label}
            </button>
          );
        })}
        <span className="font-mono text-xs text-muted-foreground">{t('observabilityTrueValues')}</span>
      </div>
      <svg ref={chartRef} viewBox={`0 0 ${chartWidth} 250`} className="h-[300px] w-full border-2 border-black bg-brutal-cream">
        {ticks.map((tick) => {
          const y = chartY(tick, maxValue);
          return (
            <g key={tick}>
              <line x1={CHART_LEFT} x2={chartRight(chartWidth)} y1={y} y2={y} stroke="var(--skin-ink)" strokeWidth="1" opacity="0.18" />
              <text x={CHART_LEFT - 10} y={y + 4} textAnchor="end" className="fill-muted-foreground font-mono text-[11px]">
                {formatAxisNumber(tick)}
              </text>
            </g>
          );
        })}
        <line x1={CHART_LEFT} x2={CHART_LEFT} y1={CHART_TOP} y2={CHART_BOTTOM} stroke="var(--skin-ink)" strokeWidth="1" opacity="0.35" />
        <line x1={CHART_LEFT} x2={chartRight(chartWidth)} y1={CHART_BOTTOM} y2={CHART_BOTTOM} stroke="var(--skin-ink)" strokeWidth="1" opacity="0.35" />
        {paths.map((path) => (
          <polyline key={path.key} points={path.points} fill="none" stroke={path.color} strokeWidth="4" strokeLinejoin="round" strokeLinecap="round" />
        ))}
        {points.map((point, index) => {
          if (points.length > 10 && index % Math.ceil(points.length / 8) !== 0 && index !== points.length - 1) return null;
          const x = chartX(index, points.length, chartWidth);
          return (
            <text key={point.date} x={x} y="232" textAnchor="middle" className="fill-muted-foreground font-mono text-[11px]">
              {point.date.slice(5)}
            </text>
          );
        })}
      </svg>
    </div>
  );
}

function trendPath(points: DashboardSeriesPoint[], key: SeriesKey, max: number, width: number) {
  return {
    key,
    color: SERIES_CONFIG[key].color,
    points: points.map((point, index) => `${chartX(index, points.length, width)},${chartY(seriesValue(point, key), max)}`).join(' '),
  };
}

function chartRight(width: number) {
  return Math.max(CHART_LEFT + 120, width - 30);
}

function chartX(index: number, total: number, width: number) {
  const right = chartRight(width);
  if (total <= 1) return (CHART_LEFT + right) / 2;
  return CHART_LEFT + (index / (total - 1)) * (right - CHART_LEFT);
}

function chartY(value: number, max: number) {
  if (max <= 0) return CHART_BOTTOM;
  return CHART_BOTTOM - (value / max) * (CHART_BOTTOM - CHART_TOP);
}

function seriesValue(point: DashboardSeriesPoint, key: SeriesKey) {
  return point[key] || 0;
}

function trendMax(points: DashboardSeriesPoint[], keys: SeriesKey[]) {
  const max = Math.max(0, ...points.flatMap((point) => keys.map((key) => seriesValue(point, key))));
  return niceAxisMax(max);
}

function niceAxisMax(value: number) {
  if (value <= 4) return 4;
  const rawStep = value / 4;
  const magnitude = 10 ** Math.floor(Math.log10(rawStep));
  const normalized = rawStep / magnitude;
  const step = normalized <= 1 ? 1 : normalized <= 2 ? 2 : normalized <= 5 ? 5 : 10;
  return step * magnitude * 4;
}

function trendTicks(max: number) {
  const step = max / 4;
  return [4, 3, 2, 1, 0].map((multiplier) => multiplier * step);
}

function CountBars({ items }: { items: DashboardCount[] }) {
  const visibleItems = items.filter((item) => item.count > 0);
  if (!visibleItems.length) return <EmptyText text={t('observabilityNoDataShort')} />;
  const max = Math.max(...visibleItems.map((item) => item.count), 1);
  return (
    <div className="space-y-3">
      {visibleItems.map((item) => (
        <div key={item.key}>
          <div className="mb-1 flex items-center justify-between font-mono text-xs">
            <span>{item.label}</span>
            <span>{item.count}</span>
          </div>
          <div className="h-3 overflow-hidden border-2 border-black bg-brutal-cream">
            <div className="h-full" style={{ width: `${Math.max(6, (item.count / max) * 100)}%`, backgroundColor: taskColor(item.key) }} />
          </div>
        </div>
      ))}
    </div>
  );
}

function UsageBars({ items }: { items: { id: string; label: string; detail: string; count: number; color: string }[] }) {
  const visibleItems = items.filter((item) => item.count > 0);
  if (!visibleItems.length) return <EmptyText text={t('observabilityNoDataShort')} />;
  const max = Math.max(...visibleItems.map((item) => item.count), 1);
  return (
    <div className="space-y-3">
      {visibleItems.map((item) => (
        <div key={item.id}>
          <div className="mb-1 flex items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="truncate font-heading text-sm font-black">{item.label}</div>
              <div className="font-mono text-[11px] text-muted-foreground">{item.detail}</div>
            </div>
            <span className="font-heading text-base font-black">{item.count}</span>
          </div>
          <div className="h-3 overflow-hidden border-2 border-black bg-brutal-cream">
            <div className="h-full" style={{ width: `${Math.max(6, (item.count / max) * 100)}%`, backgroundColor: item.color }} />
          </div>
        </div>
      ))}
    </div>
  );
}

function WordCloud({ terms }: { terms: DashboardCount[] }) {
  const visibleTerms = terms.filter((term) => term.count > 0).slice(0, 80);
  if (!visibleTerms.length) return <EmptyText text={t('observabilityNoWordCloud')} />;
  const max = Math.max(...visibleTerms.map((term) => term.count), 1);
  return (
    <div className="flex min-h-52 flex-wrap items-center justify-center gap-x-4 gap-y-2 bg-brutal-cream p-5">
      {visibleTerms.map((term, index) => (
        <span
          key={term.key}
          className="font-heading font-black leading-none"
          style={{
            color: WORD_COLORS[index % WORD_COLORS.length],
            fontSize: `${14 + Math.round((term.count / max) * 26)}px`,
          }}
        >
          {term.label}
        </span>
      ))}
    </div>
  );
}

function EmptyText({ text }: { text: string }) {
  return <div className="py-8 text-center font-mono text-xs text-muted-foreground">{text}</div>;
}

function taskColor(status: string) {
  switch (status) {
  case 'todo':
    return 'var(--color-brutal-warning)';
  case 'in_progress':
    return 'var(--color-brutal-info)';
  case 'in_review':
    return 'var(--color-brutal-violet)';
  case 'done':
    return 'var(--color-brutal-success)';
  case 'closed':
    return 'var(--color-brutal-muted)';
  default:
    return 'var(--color-brutal-accent)';
  }
}

function formatNumber(value: number) {
  return new Intl.NumberFormat('zh-CN').format(value || 0);
}

function formatAxisNumber(value: number) {
  if (value >= 1_000_000) return `${trimNumber(value / 1_000_000)}M`;
  if (value >= 1_000) return `${trimNumber(value / 1_000)}K`;
  return formatNumber(Math.round(value));
}

function trimNumber(value: number) {
  return Number.isInteger(value) ? String(value) : value.toFixed(1);
}

function formatShortDate(value?: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '-';
  return date.toLocaleDateString('zh-CN');
}

function formatDate(value?: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '-';
  return date.toLocaleString('zh-CN', { hour12: false });
}
