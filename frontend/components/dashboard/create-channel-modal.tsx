// ============================================================================
// CreateChannelModal — modal form for creating a new channel
// ============================================================================

'use client';

import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import {
  Dialog,
  DialogCloseButton,
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
}

export function CreateChannelModal({
  open,
  onOpenChange,
  onSubmit,
}: CreateChannelModalProps) {
  const [isSubmitting, setIsSubmitting] = useState(false);

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
    }
    onOpenChange(next);
  };

  const onFormSubmit = async (data: FormValues) => {
    setIsSubmitting(true);
    try {
      await onSubmit({ name: data.name, description: data.description });
      reset();
      onOpenChange(false);
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <div className="flex items-center justify-between">
        <DialogTitle>Create Channel</DialogTitle>
        <DialogCloseButton onClick={() => handleOpenChange(false)} />
      </div>
      <DialogDescription>
        Channels are collaboration spaces around specific topics. Use a suitable name like general, random, or project-alpha.
      </DialogDescription>

      <form onSubmit={handleSubmit(onFormSubmit)} className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="channel-name">Name</Label>
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
          <Label htmlFor="channel-desc">Description (optional)</Label>
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

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => handleOpenChange(false)}
            disabled={isSubmitting}
          >
            Cancel
          </Button>
          <Button type="submit" disabled={isSubmitting}>
            {isSubmitting ? 'Creating...' : 'Create'}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>
  );
}
