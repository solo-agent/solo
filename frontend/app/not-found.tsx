// ============================================================================
// 404 Not Found page — neubrutalist styling (v3.2 Phase 2)
// - Halftone dot grid texture behind the hero block (print/risograph feel)
// - 404 wordmark rotated 2° + hollow text-stroke (display treatment)
// ============================================================================

import Link from 'next/link';
import { FileQuestion } from 'lucide-react';

export default function NotFoundPage() {
  return (
    <div className="relative flex min-h-screen flex-col items-center justify-center bg-brutal-cream px-4 overflow-hidden">
      {/* v3.2 (Phase 2): halftone texture behind the hero block.
          Pointer-events-none so it never intercepts clicks. */}
      <div className="absolute inset-0 bg-halftone pointer-events-none" aria-hidden />

      <div className="relative mx-auto flex max-w-sm flex-col items-center text-center">
        <div className="mb-6 flex h-20 w-20 items-center justify-center border-brutal-4 bg-brutal-primary shadow-brutal">
          <FileQuestion className="h-10 w-10 text-black" />
        </div>
        {/* v3.2 (Phase 2): 404 wordmark treated as a display sticker —
            rotated + hollow (text-stroke). The number becomes a piece
            of the page rather than just a label. */}
        <h1
          className="font-heading text-7xl font-black text-foreground"
          style={{
            transform: 'rotate(2deg)',
            WebkitTextStroke: '2px #000',
            color: 'transparent',
          }}
        >
          404
        </h1>
        <p className="mt-2 font-heading text-lg font-bold text-foreground">
          Page not found
        </p>
        <p className="mt-2 font-body text-sm text-muted-foreground">
          The page you are looking for does not exist or has been removed. Please check the link.
        </p>
        <div className="mt-8 flex gap-4">
          <Link
            href="/dashboard"
            className="btn-brutal btn-brutal-primary px-5 py-2.5 text-sm"
          >
            Back to Dashboard
          </Link>
          <Link
            href="/auth/login"
            className="btn-brutal bg-white px-5 py-2.5 text-sm"
          >
            Back to Login
          </Link>
        </div>
      </div>
    </div>
  );
}
