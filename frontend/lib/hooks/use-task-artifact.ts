'use client';

import { useCallback, useState } from 'react';
import { apiClient } from '@/lib/api-client';
import type { TaskArtifact } from '@/lib/types';

export function useTaskArtifact() {
  const [isGenerating, setIsGenerating] = useState(false);

  const generateArtifact = useCallback(async (taskId: string): Promise<TaskArtifact> => {
    setIsGenerating(true);
    try {
      return await apiClient.post<TaskArtifact>(`/api/v1/tasks/${taskId}/artifact`);
    } finally {
      setIsGenerating(false);
    }
  }, []);

  return { generateArtifact, isGenerating };
}
