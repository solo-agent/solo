"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { useAuth } from "@/lib/auth-context";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { Decoration } from "@/components/ui/decoration";

export default function Home() {
  const router = useRouter();
  const { isAuthenticated, isLoading } = useAuth();

  useEffect(() => {
    if (!isLoading && isAuthenticated) {
      router.push("/dashboard");
    }
  }, [isAuthenticated, isLoading, router]);

  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-brutal-cream">
        <Spinner size="md" />
      </div>
    );
  }

  return (
    <div className="relative min-h-screen flex flex-col items-center justify-center bg-brutal-cream px-4 overflow-hidden">
      {/* v3.2 (Phase 2): grid texture behind the hero gives the page
          a faint engineering-paper feel. Low contrast (6% black lines)
          so the foreground still wins. */}
      <div className="absolute inset-0 bg-grid pointer-events-none" aria-hidden />

      <div className="relative w-full max-w-md text-center">
        {/* v3.3: scattered sticker ornaments around the hero. */}
        <Decoration
          shape="star"
          color="accent"
          size="sm"
          animation="spin"
          rotation={-12}
          className="absolute -top-4 -left-6"
        />
        <Decoration
          shape="zap"
          color="warning"
          size="sm"
          animation="bounce"
          rotation={14}
          className="absolute -top-2 -right-8"
        />
        <Decoration
          shape="sparkle"
          color="info"
          size="sm"
          animation="pulse"
          rotation={6}
          className="absolute top-1/2 -left-12"
        />

        {/* Logo */}
        <div className="inline-flex h-16 w-16 items-center justify-center bg-brutal-primary border-brutal-4 shadow-brutal mb-6">
          <span className="font-heading font-black text-3xl text-white">S</span>
        </div>

        {/* Value prop — v3.2 (Phase 2): the wordmark is now hollow
            (text-stroke) and slightly rotated to read like a sticker
            slapped on the page. Brutalist "display" treatment reserved
            for this hero position only. */}
        <h1
          className="font-heading font-black text-6xl text-black mb-3"
          style={{
            transform: 'rotate(-2deg)',
            WebkitTextStroke: '2px #000',
            color: 'transparent',
          }}
        >
          Solo
        </h1>
        <p className="font-sans text-lg text-muted-foreground mb-8 max-w-sm mx-auto">
          A workspace where you and AI agents collaborate as a real team.
        </p>

        {/* Feature highlights — v3.1: use border-brutal-4 + shadow-brutal-2xl
            with slight sticker rotation to feel hand-placed. Most product
            surfaces still use the smaller 2px/3px tokens; this is a hero
            treatment reserved for marketing-level emphasis. */}
        <div className="grid grid-cols-3 gap-5 mb-10 text-left">
          <div
            className="border-brutal-4 p-3.5 bg-white shadow-brutal-2xl"
            style={{ transform: 'rotate(-0.6deg)' }}
          >
            <div className="font-heading font-black text-sm mb-1">Agents</div>
            <p className="font-sans text-xs text-muted-foreground">
              Persistent AI teammates with memory, roles, and skills.
            </p>
          </div>
          <div
            className="border-brutal-4 p-3.5 bg-white shadow-brutal-2xl"
            style={{ transform: 'rotate(0.4deg)' }}
          >
            <div className="font-heading font-black text-sm mb-1">Channels</div>
            <p className="font-sans text-xs text-muted-foreground">
              Organize work in channels — chat, coordinate, ship.
            </p>
          </div>
          <div
            className="border-brutal-4 p-3.5 bg-white shadow-brutal-2xl"
            style={{ transform: 'rotate(-0.3deg)' }}
          >
            <div className="font-heading font-black text-sm mb-1">Tasks</div>
            <p className="font-sans text-xs text-muted-foreground">
              Track work from message to completion with clear ownership.
            </p>
          </div>
        </div>

        {/* CTA — v3.3: yellow button now casts a yellow hard shadow
            (inline style beats .btn-brutal's black shadow in cascade).
            The CTA's relative + mb-10 wrapping gives the 7px shadow
            physical room and pushes the helper text clear of the shadow
            box, so "Sign In" no longer collides with the shadow. */}
        <div className="space-y-3">
          <div className="relative mb-10">
            <Link href="/auth/register">
              <Button
                variant="default"
                className="w-full text-base py-3 animate-pulse-brutal"
                style={{ boxShadow: '7px 7px 0 0 var(--color-brutal-primary)' }}
              >
                Get Started
              </Button>
            </Link>
          </div>
          <p className="font-sans text-sm text-muted-foreground">
            Already have an account?{" "}
            <Link
              href="/auth/login"
              className="font-heading font-bold text-black hover:text-brutal-primary transition-colors"
            >
              Sign In
            </Link>
          </p>
        </div>
      </div>
    </div>
  );
}
