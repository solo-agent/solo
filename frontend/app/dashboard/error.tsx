'use client';

import { useEffect } from 'react';
import Link from 'next/link';

export default function DashboardError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    console.error('[DashboardError]', error);
  }, [error]);

  return (
    <div className="flex flex-1 items-center justify-center p-8 bg-brutal-cream">
      <div className="border-4 border-black shadow-brutal-xl bg-white rounded-none p-8 max-w-lg w-full text-center">
        <h1 className="font-heading font-bold text-3xl text-black mb-4">
          Dashboard failed to load
        </h1>
        <p className="font-body text-lg text-black/70 mb-2">
          The dashboard encountered an error. Please retry or return to the dashboard home.
        </p>
        {error.digest && (
          <p className="font-mono text-xs text-black/40 mb-6">
            Error ID: {error.digest}
          </p>
        )}
        <div className="flex flex-col gap-3">
          <button onClick={reset} className="btn-brutal w-full">
            Retry
          </button>
          <Link
            href="/dashboard"
            className="btn-brutal w-full inline-flex items-center justify-center"
          >
            Back to Dashboard
          </Link>
        </div>
      </div>
    </div>
  );
}
