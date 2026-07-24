// ============================================================================
// CreateChannelModal — modal form for creating a new channel
// ============================================================================

'use client';

import { useState, type ReactNode } from 'react';
import { ArrowLeft, ArrowRight, Hash, LayoutTemplate, Sparkles } from 'lucide-react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import {
  Dialog,
  DialogCloseButton,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { t } from '@/lib/i18n';
import type { CreateChannelInput } from '@/lib/types';

const createChannelSchema = z.object({
  name: z
    .string()
    .min(1, t('channelNameRequired'))
    .max(80, t('channelNameMaxLen'))
    .regex(/^[a-z0-9_-]+$/, t('channelNamePattern')),
  description: z.string().max(200, t('channelDescMaxLen')).optional(),
});

type FormValues = z.infer<typeof createChannelSchema>;

interface CreateChannelModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (input: CreateChannelInput) => Promise<void>;
  onChooseTemplate: () => void;
  onAskLucy: () => void;
}

export function CreateChannelModal({
  open,
  onOpenChange,
  onSubmit,
  onChooseTemplate,
  onAskLucy,
}: CreateChannelModalProps) {
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [mode, setMode] = useState<'choose' | 'blank'>('choose');
  const [submitError, setSubmitError] = useState<string | null>(null);

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<FormValues>({
    resolver: zodResolver(createChannelSchema),
    defaultValues: { name: '', description: '' },
  });

  // Reset form when opening
  const handleOpenChange = (next: boolean) => {
    if (!next) {
      reset();
      setMode('choose');
      setSubmitError(null);
    }
    onOpenChange(next);
  };

  const onFormSubmit = async (data: FormValues) => {
    setIsSubmitting(true);
    setSubmitError(null);
    try {
      await onSubmit({ name: data.name, description: data.description });
      handleOpenChange(false);
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : t('channelCreateError'));
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange} width={mode === 'choose' ? 'lg' : 'md'}>
      <DialogHeader>
        <DialogTitle>{mode === 'choose' ? t('startChannelTitle') : t('blankChannelTitle')}</DialogTitle>
        <DialogCloseButton onClick={() => handleOpenChange(false)} />
      </DialogHeader>

      {mode === 'choose' ? (
        <>
          <DialogDescription className="max-w-md leading-relaxed">
            {t('startChannelDescription')}
          </DialogDescription>
          <div className="mt-5 space-y-3">
            <ChannelStartChoice
              icon={<Sparkles className="h-5 w-5 text-brutal-accent" />}
              eyebrow={t('startChannelRecommended')}
              title={t('startChannelAskLucyTitle')}
              description={t('startChannelAskLucyDesc')}
              featured
              onClick={() => {
                handleOpenChange(false);
                onAskLucy();
              }}
            />
            <div className="flex items-center gap-3 py-1">
              <span className="h-px flex-1 bg-black/15" />
              <span className="font-mono text-[9px] font-bold uppercase tracking-widest text-black/45">
                {t('startChannelDivider')}
              </span>
              <span className="h-px flex-1 bg-black/15" />
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <ChannelStartChoice
                icon={<LayoutTemplate className="h-5 w-5" />}
                title={t('startChannelTemplateTitle')}
                description={t('startChannelTemplateDesc')}
                onClick={() => {
                  handleOpenChange(false);
                  onChooseTemplate();
                }}
              />
              <ChannelStartChoice
                icon={<Hash className="h-5 w-5" />}
                title={t('startChannelBlankTitle')}
                description={t('startChannelBlankDesc')}
                onClick={() => setMode('blank')}
              />
            </div>
          </div>
        </>
      ) : (
        <form onSubmit={handleSubmit(onFormSubmit)} className="space-y-4">
          <button
            type="button"
            onClick={() => setMode('choose')}
            className="inline-flex items-center gap-1 font-mono text-xs font-bold uppercase hover:underline"
          >
            <ArrowLeft className="h-3.5 w-3.5" />
            {t('back')}
          </button>
          <DialogDescription>
            {t('blankChannelDescription')}
          </DialogDescription>
          <div className="space-y-2">
            <Label htmlFor="channel-name">{t('channelNameLabel')}</Label>
            <Input
              id="channel-name"
              placeholder={t('channelNamePlaceholder')}
              disabled={isSubmitting}
              autoFocus
              aria-invalid={!!errors.name}
              {...register('name')}
            />
            {errors.name && (
              <p className="text-xs text-destructive" role="alert">
                {errors.name.message}
              </p>
            )}
          </div>

          <div className="space-y-2">
            <Label htmlFor="channel-desc">{t('channelDescLabel')}</Label>
            <Input
              id="channel-desc"
              placeholder={t('channelDescPlaceholder')}
              disabled={isSubmitting}
              aria-invalid={!!errors.description}
              {...register('description')}
            />
            {errors.description && (
              <p className="text-xs text-destructive" role="alert">
                {errors.description.message}
              </p>
            )}
          </div>

          {submitError && (
            <p className="border-2 border-black bg-brutal-danger-light p-2 font-mono text-xs text-brutal-danger" role="alert">
              {submitError}
            </p>
          )}

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => handleOpenChange(false)}
              disabled={isSubmitting}
            >
              {t('cancel')}
            </Button>
            <Button type="submit" variant="success" disabled={isSubmitting}>
              {isSubmitting ? t('creating') : t('create')}
            </Button>
          </DialogFooter>
        </form>
      )}
    </Dialog>
  );
}

function ChannelStartChoice({
  icon,
  eyebrow,
  title,
  description,
  featured = false,
  onClick,
}: {
  icon: ReactNode;
  eyebrow?: string;
  title: string;
  description: string;
  featured?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`group flex w-full items-center gap-3 border-2 border-black p-3 text-left shadow-brutal-sm transition-[transform,box-shadow,background-color] hover:-translate-y-px hover:shadow-brutal ${
        featured ? 'bg-brutal-accent-light' : 'bg-white hover:bg-brutal-primary-light'
      }`}
    >
      <span className="flex h-10 w-10 shrink-0 items-center justify-center border-2 border-black bg-white shadow-brutal-sm">
        {icon}
      </span>
      <span className="min-w-0 flex-1">
        {eyebrow && (
          <span className="block font-mono text-[9px] font-bold uppercase tracking-widest text-black/45">
            {eyebrow}
          </span>
        )}
        <span className="block font-heading text-base font-black">{title}</span>
        <span className="mt-0.5 block font-body text-xs leading-relaxed text-muted-foreground">{description}</span>
      </span>
      <ArrowRight className="h-4 w-4 shrink-0 transition-transform group-hover:translate-x-0.5" />
    </button>
  );
}
