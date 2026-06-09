'use client';

import { useState, useMemo } from 'react';
import { Check, Monitor, Cpu, Sparkles, AlertCircle, ChevronDown } from 'lucide-react';
import { useCliDetection } from '@/lib/hooks/use-cli-detection';
import { useComputers } from '@/lib/hooks/use-computers';
import { useOnboarding } from '@/lib/hooks/use-onboarding';
import { Button } from '@/components/ui/button';
import { Spinner } from '@/components/ui/spinner';
import { Select, type SelectOption } from '@/components/ui/select';

interface WizardCardProps {
  channelId: string;
  onComplete?: () => void;
}

export function WizardCard({ channelId, onComplete }: WizardCardProps) {
  const { results: cliResults, isLoaded: cliLoaded } = useCliDetection();
  const { computers, isLoading: computersLoading } = useComputers();
  const { createLucy, isCreating, error: createError } = useOnboarding();

  const [selectedRuntime, setSelectedRuntime] = useState<string>('');
  const [done, setDone] = useState(false);

  const computerOnline = computers.some((c) => c.status === 'online');
  const computerName = computers.find((c) => c.status === 'online')?.name ?? 'Unknown';

  const runtimeOptions: SelectOption[] = useMemo(() => {
    const options: SelectOption[] = [];
    for (const [type, item] of Object.entries(cliResults)) {
      if (item.available) {
        const label = item.version
          ? `${item.display_name} (${item.version})`
          : item.display_name;
        options.push({ value: type, label });
      }
    }
    return options;
  }, [cliResults]);

  const hasAvailableRuntime = runtimeOptions.length > 0;

  const handleCreateLucy = async () => {
    if (!selectedRuntime || isCreating || done) return;
    // Auto-bind to the first online computer
    const onlineComputer = computers.find((c) => c.status === 'online');
    try {
      await createLucy({
        runtime_type: selectedRuntime,
        channel_id: channelId,
        computer_id: onlineComputer?.id,
      });
      setDone(true);
      onComplete?.();
    } catch {
      // error state handled by hook
    }
  };

  return (
    <div className="card-brutal mb-4">
      {/* Header */}
      <div className="flex items-center gap-2 border-b-2 border-black px-5 py-3">
        <Sparkles className="h-4 w-4 text-brutal-primary" />
        <h3 className="font-heading font-bold text-base text-foreground">
          Set Up Your Workspace
        </h3>
      </div>

      <div className="space-y-1 px-5 py-4">
        {/* Step 1: Computer */}
        <div className="flex items-start gap-3 py-2">
          <div className="mt-0.5 flex h-5 w-5 flex-shrink-0 items-center justify-center border-2 border-black bg-brutal-success">
            <Check className="h-3 w-3 text-black" />
          </div>
          <div className="min-w-0">
            <p className="font-heading text-sm font-bold text-foreground">
              Computer Connected
            </p>
            <p className="font-sans text-xs text-muted-foreground">
              {computersLoading ? (
                'Detecting...'
              ) : computerOnline ? (
                <span>
                  <Monitor className="mr-1 inline h-3 w-3" />
                  {computerName} — online
                </span>
              ) : (
                <span className="text-brutal-danger">
                  No computer detected. Run <code className="font-mono text-[11px]">make start</code> to connect.
                </span>
              )}
            </p>
          </div>
        </div>

        {/* Step 2: Select Runtime */}
        <div className="flex items-start gap-3 py-2">
          <div className="mt-0.5 flex h-5 w-5 flex-shrink-0 items-center justify-center border-2 border-black bg-brutal-info">
            <Cpu className="h-3 w-3 text-black" />
          </div>
          <div className="min-w-0 flex-1">
            <p className="font-heading text-sm font-bold text-foreground">
              Select Runtime CLI
            </p>
            <p className="mb-2 font-sans text-xs text-muted-foreground">
              Pick the AI CLI tool installed on your computer.
            </p>
            {!cliLoaded ? (
              <Spinner size="sm" />
            ) : hasAvailableRuntime ? (
              <Select
                options={runtimeOptions}
                value={selectedRuntime}
                onChange={setSelectedRuntime}
                placeholder="Choose a CLI backend..."
              />
            ) : (
              <p className="font-mono text-xs text-brutal-danger">
                No supported CLI runtime detected. Install Claude Code, Codex, or
                another supported tool.
              </p>
            )}
          </div>
        </div>

        {/* Step 3: Create Lucy */}
        <div className="flex items-start gap-3 py-2">
          <div
            className={`mt-0.5 flex h-5 w-5 flex-shrink-0 items-center justify-center border-2 border-black ${
              done ? 'bg-brutal-success' : 'bg-brutal-primary'
            }`}
          >
            {done ? (
              <Check className="h-3 w-3 text-black" />
            ) : isCreating ? (
              <Spinner size="sm" />
            ) : (
              <Sparkles className="h-3 w-3 text-black" />
            )}
          </div>
          <div className="min-w-0">
            <p className="font-heading text-sm font-bold text-foreground">
              {done ? 'Lucy is Ready!' : 'Create Lucy'}
            </p>
            <p className="font-sans text-xs text-muted-foreground">
              {done
                ? 'Lucy has joined the channel. Start chatting below!'
                : 'Create your onboarding agent to help you get set up.'}
            </p>

            {!done && (
              <div className="mt-2">
                <Button
                  variant="default"
                  size="sm"
                  disabled={!selectedRuntime || isCreating || !hasAvailableRuntime}
                  onClick={handleCreateLucy}
                  className="gap-1.5"
                >
                  {isCreating ? (
                    <>
                      <Spinner size="sm" />
                      Creating Lucy...
                    </>
                  ) : (
                    <>
                      <Sparkles className="h-3.5 w-3.5" />
                      Create Lucy
                    </>
                  )}
                </Button>
              </div>
            )}

            {createError && (
              <div className="mt-2 flex items-center gap-1.5 font-mono text-xs text-brutal-danger">
                <AlertCircle className="h-3.5 w-3.5" />
                {createError}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
