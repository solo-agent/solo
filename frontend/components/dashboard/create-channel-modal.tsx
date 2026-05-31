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
import type { CreateChannelInput } from '@/lib/types';

const createChannelSchema = z.object({
  name: z
    .string()
    .min(1, '请输入频道名称')
    .max(80, '频道名称不能超过 80 个字符')
    .regex(/^[a-z0-9_-]+$/, '频道名称只能包含小写字母、数字、下划线和连字符'),
  description: z.string().max(200, '频道描述不能超过 200 个字符').optional(),
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
        <DialogTitle>创建频道</DialogTitle>
        <DialogCloseButton onClick={() => handleOpenChange(false)} />
      </div>
      <DialogDescription>
        频道是团队成员围绕特定主题进行协作的空间。请使用合适的名称，例如 general、random、project-alpha。
      </DialogDescription>

      <form onSubmit={handleSubmit(onFormSubmit)} className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="channel-name">名称</Label>
          <Input
            id="channel-name"
            placeholder="例如：project-alpha"
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
          <Label htmlFor="channel-desc">描述（选填）</Label>
          <Input
            id="channel-desc"
            placeholder="这个频道是做什么的？"
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
            取消
          </Button>
          <Button type="submit" disabled={isSubmitting}>
            {isSubmitting ? '创建中...' : '创建'}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>
  );
}
