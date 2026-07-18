import { memo, type ReactNode } from 'react';
import { Brain, MessageSquare, Wrench } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { LiveAgentState } from '@/lib/hooks/use-team-agent-activity';

function shorten(value: string | undefined, max: number) {
  if (!value) return '';
  return value.length > max ? `${value.slice(0, max - 1)}...` : value;
}

function CardShell({
  children,
  className,
  kind,
  title,
}: {
  children: ReactNode;
  className?: string;
  kind: 'human' | 'activity' | 'tool';
  title?: string;
}) {
  return (
    <div
      title={title}
      data-agent-activity-kind={kind}
      className={cn(
        'pointer-events-auto border-2 border-black bg-white px-2 py-1 shadow-brutal-sm',
        'max-w-[190px] animate-in fade-in zoom-in-95 duration-150',
        className,
      )}
    >
      {children}
    </div>
  );
}

export const ActivityCard = memo(function ActivityCard({ text }: { text: string }) {
  return (
    <CardShell kind="activity" className="bg-brutal-info-light" title={text}>
      <div className="flex items-center gap-1.5">
        <Brain className="h-3 w-3 flex-shrink-0 text-brutal-info" />
        <span className="truncate font-mono text-[9px] font-bold text-black">
          {shorten(text, 60)}
        </span>
      </div>
    </CardShell>
  );
});

export const ToolCard = memo(function ToolCard({
  name,
  args,
}: {
  name: string;
  args?: string;
}) {
  const full = args ? `${name}: ${args}` : name;
  return (
    <CardShell kind="tool" className="bg-brutal-primary-light" title={full}>
      <div className="flex min-w-0 items-center gap-1.5">
        <Wrench className="h-3 w-3 flex-shrink-0 text-yellow-700" />
        <span className="font-mono text-[10px] font-black text-black">{shorten(name, 18)}</span>
        {args && (
          <span className="min-w-0 truncate font-mono text-[10px] text-muted-foreground">
            {shorten(args, 30)}
          </span>
        )}
      </div>
    </CardShell>
  );
});

export const HumanMsgCard = memo(function HumanMsgCard({
  author,
  text,
}: {
  author: string;
  text: string;
}) {
  const full = `${author}: ${text}`;
  return (
    <CardShell kind="human" className="bg-brutal-warning-light" title={full}>
      <div className="flex min-w-0 items-center gap-1.5">
        <MessageSquare className="h-3 w-3 flex-shrink-0 text-orange-700" />
        <span className="max-w-[150px] truncate font-mono text-[8px] font-bold text-black">
          {shorten(full, 48)}
        </span>
      </div>
    </CardShell>
  );
});

export type ActivityCardPlacement = 'top' | 'right' | 'bottom' | 'left';

const PLACEMENT_CLASS: Record<ActivityCardPlacement, string> = {
  top: 'bottom-full left-1/2 mb-3 -translate-x-1/2',
  right: 'left-full top-1/2 ml-3 -translate-y-1/2',
  bottom: 'left-1/2 top-full mt-3 -translate-x-1/2',
  left: 'right-full top-1/2 mr-3 -translate-y-1/2',
};

export const RelationshipActivityCard = memo(function RelationshipActivityCard({
  activity,
  placement = 'top',
}: {
  activity?: LiveAgentState;
  placement?: ActivityCardPlacement;
}) {
  if (!activity) return null;
  return (
    <div
      data-agent-activity-stack
      className={cn(
        'pointer-events-none absolute z-20 flex w-max max-w-[220px] flex-col items-stretch gap-2',
        PLACEMENT_CLASS[placement],
      )}
    >
      {activity.currentHumanMsg && (
        <HumanMsgCard
          author={activity.currentHumanMsg.authorName}
          text={activity.currentHumanMsg.text}
        />
      )}
      {activity.currentActivity?.text && (
        <ActivityCard text={activity.currentActivity.text} />
      )}
      {activity.currentTool?.name && (
        <ToolCard name={activity.currentTool.name} args={activity.currentTool.args} />
      )}
    </div>
  );
});
