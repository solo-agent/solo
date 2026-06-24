'use client';

import { useCallback, useRef, useState } from 'react';
import { apiClient } from '@/lib/api-client';
import type { TaskArtifact } from '@/lib/types';

const ARTIFACT_POLL_INTERVAL_MS = 1500;
const ARTIFACT_POLL_ATTEMPTS = 40;

export class TaskArtifactGenerationInProgressError extends Error {
  constructor() {
    super('Task artifact generation is already in progress for a different task');
    this.name = 'TaskArtifactGenerationInProgressError';
  }
}

const sleep = (ms: number) => new Promise((resolve) => window.setTimeout(resolve, ms));

export function useTaskArtifact() {
  const [isGenerating, setIsGenerating] = useState(false);
  const isGeneratingRef = useRef(false);
  const inFlightPromiseRef = useRef<Promise<TaskArtifact> | null>(null);
  const inFlightTaskIdRef = useRef<string | null>(null);

  const waitForPublishedArtifact = useCallback(async (taskId: string, mode: 'latest' | 'final', baseline: TaskArtifact): Promise<TaskArtifact> => {
    if (baseline.summary !== 'pending') return baseline;
    const baselineTime = Date.parse(baseline.updated_at);
    for (let attempt = 0; attempt < ARTIFACT_POLL_ATTEMPTS; attempt += 1) {
      await sleep(ARTIFACT_POLL_INTERVAL_MS);
      try {
        const artifact = await apiClient.get<TaskArtifact>(`/api/v1/tasks/${taskId}/artifact/latest?mode=${mode}`);
        if (artifact.summary !== 'pending' && Date.parse(artifact.updated_at) > baselineTime) {
          return artifact;
        }
      } catch {
        // The agent may not have published yet.
      }
    }
    return baseline;
  }, []);

  const runArtifactMutation = useCallback(async (taskId: string, endpoint: string, mode: 'latest' | 'final'): Promise<TaskArtifact> => {
    if (isGeneratingRef.current && inFlightPromiseRef.current) {
      if (inFlightTaskIdRef.current !== taskId) {
        throw new TaskArtifactGenerationInProgressError();
      }
      return inFlightPromiseRef.current;
    }
    isGeneratingRef.current = true;
    inFlightTaskIdRef.current = taskId;
    setIsGenerating(true);
    const promise = apiClient.post<TaskArtifact>(endpoint).then((artifact) => waitForPublishedArtifact(taskId, mode, artifact));
    inFlightPromiseRef.current = promise;
    try {
      return await promise;
    } finally {
      isGeneratingRef.current = false;
      inFlightPromiseRef.current = null;
      inFlightTaskIdRef.current = null;
      setIsGenerating(false);
    }
  }, [waitForPublishedArtifact]);

  const generateArtifact = useCallback(
    (taskId: string): Promise<TaskArtifact> => runArtifactMutation(taskId, `/api/v1/tasks/${taskId}/artifact`, 'latest'),
    [runArtifactMutation],
  );

  const finalizeArtifact = useCallback(
    (taskId: string): Promise<TaskArtifact> => runArtifactMutation(taskId, `/api/v1/tasks/${taskId}/artifact/finalize`, 'final'),
    [runArtifactMutation],
  );

  const fetchArtifactHTML = useCallback((artifact: TaskArtifact): Promise<string> => {
    return apiClient.getText(artifact.url);
  }, []);

  return { generateArtifact, finalizeArtifact, fetchArtifactHTML, isGenerating };
}
