'use client';

import { useCallback, useRef, useState } from 'react';
import { apiClient } from '@/lib/api-client';
import type { TaskArtifact } from '@/lib/types';

export function useTaskArtifact() {
  const [isGenerating, setIsGenerating] = useState(false);
  const isGeneratingRef = useRef(false);

  const generateArtifact = useCallback(async (taskId: string): Promise<TaskArtifact> => {
    if (isGeneratingRef.current) {
      throw new Error('Artifact generation is already in progress');
    }
    isGeneratingRef.current = true;
    setIsGenerating(true);
    try {
      return await apiClient.post<TaskArtifact>(`/api/v1/tasks/${taskId}/artifact`);
    } finally {
      isGeneratingRef.current = false;
      setIsGenerating(false);
    }
  }, []);

  return { generateArtifact, isGenerating };
}
