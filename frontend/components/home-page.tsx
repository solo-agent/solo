"use client";

import Link from "next/link";
import { Button } from "@/components/ui/button";
import { Decoration } from "@/components/ui/decoration";
import { LanguageSwitcher } from "@/components/language-switcher";
import { t } from "@/lib/i18n";

export default function HomePage() {
  return (
    <div className="relative min-h-screen flex flex-col items-center justify-center bg-brutal-cream px-4 overflow-hidden">
      <div className="absolute right-4 top-4 z-20"><LanguageSwitcher /></div>
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
          {t('homeTagline')}
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
            <div className="font-heading font-black text-sm mb-1">{t('homeAgentsTitle')}</div>
            <p className="font-sans text-xs text-muted-foreground">
              {t('homeAgentsDesc')}
            </p>
          </div>
          <div
            className="border-brutal-4 p-3.5 bg-white shadow-brutal-2xl"
            style={{ transform: 'rotate(0.4deg)' }}
          >
            <div className="font-heading font-black text-sm mb-1">{t('homeChannelsTitle')}</div>
            <p className="font-sans text-xs text-muted-foreground">
              {t('homeChannelsDesc')}
            </p>
          </div>
          <div
            className="border-brutal-4 p-3.5 bg-white shadow-brutal-2xl"
            style={{ transform: 'rotate(-0.3deg)' }}
          >
            <div className="font-heading font-black text-sm mb-1">{t('homeTasksTitle')}</div>
            <p className="font-sans text-xs text-muted-foreground">
              {t('homeTasksDesc')}
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
                {t('homeGetStarted')}
              </Button>
            </Link>
          </div>
          <p className="font-sans text-sm text-muted-foreground">
            {t('homeExistingAccount')}{" "}
            <Link
              href="/auth/login"
              className="font-heading font-bold text-black hover:text-brutal-primary transition-colors"
            >
              {t('homeSignIn')}
            </Link>
          </p>
          <p className="font-sans text-sm text-muted-foreground">
            <a
              href="https://github.com/solo-agent/solo"
              target="_blank"
              rel="noreferrer"
              className="font-heading font-bold text-black hover:text-brutal-primary transition-colors"
            >
              Solo Agent on GitHub
            </a>
          </p>
        </div>
      </div>
    </div>
  );
}
