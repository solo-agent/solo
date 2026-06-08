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
import { t } from '@/lib/i18n';

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
      <DialogTitle>Delete Channel</DialogTitle>
      <DialogDescription>
        Are you sure you want to delete <strong>#{channelName}</strong>? All messages in the channel will be permanently removed.
      </DialogDescription>

      <DialogFooter>
        <Button
          type="button"
          variant="outline"
          onClick={() => onOpenChange(false)}
          disabled={isDeleting}
        >
          Cancel
        </Button>
        <Button
          type="button"
          variant="destructive"
          onClick={handleDelete}
          disabled={isDeleting}
        >
          {isDeleting ? 'Deleting...' : 'Delete Channel'}
        </Button>
      </DialogFooter>
    </Dialog>
  );
}
