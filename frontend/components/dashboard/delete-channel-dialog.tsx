// ============================================================================
// DeleteChannelDialog — confirmation dialog before deleting a channel
// ============================================================================

'use client';

import { useState } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';

interface DeleteChannelDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  channelName: string;
  onConfirm: () => Promise<void>;
}

export function DeleteChannelDialog({
  open,
  onOpenChange,
  channelName,
  onConfirm,
}: DeleteChannelDialogProps) {
  const [isDeleting, setIsDeleting] = useState(false);

  const handleDelete = async () => {
    setIsDeleting(true);
    try {
      await onConfirm();
      onOpenChange(false);
    } finally {
      setIsDeleting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogTitle>删除频道</DialogTitle>
      <DialogDescription>
        确定要删除 <strong>#{channelName}</strong> 吗？删除后频道内所有消息将被移除，且不可恢复。
      </DialogDescription>

      <DialogFooter>
        <Button
          type="button"
          variant="outline"
          onClick={() => onOpenChange(false)}
          disabled={isDeleting}
        >
          取消
        </Button>
        <Button
          type="button"
          variant="destructive"
          onClick={handleDelete}
          disabled={isDeleting}
        >
          {isDeleting ? '删除中...' : '删除频道'}
        </Button>
      </DialogFooter>
    </Dialog>
  );
}
