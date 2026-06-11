'use client';

import { useEffect } from 'react';

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    console.error('[GlobalError]', error);
  }, [error]);

  return (
    <html lang="en">
      <body className="min-h-screen bg-brutal-cream font-sans">
        <div className="flex min-h-screen items-center justify-center p-8">
          <div className="border-brutal-4 shadow-brutal-xl bg-white rounded-none p-8 max-w-lg w-full">
            <h1 className="font-heading font-black text-3xl text-black mb-4">
              Something went wrong
            </h1>
            <p className="font-body text-lg text-black/70 mb-2">
              An unexpected error occurred. Please try again.
            </p>
            {error.digest && (
              <p className="font-mono text-xs text-black/40 mb-6">
                Error ID: {error.digest}
              </p>
            )}
            <button
              onClick={reset}
              className="btn-brutal w-full"
            >
              Retry
            </button>
          </div>
        </div>
      </body>
    </html>
  );
}
