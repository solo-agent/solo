"use client";

import Link from "next/link";
import { Button } from "@/components/ui/button";
import { LanguageSwitcher } from "@/components/language-switcher";
import { t } from "@/lib/i18n";

export default function HomePage() {
  return (
    <main id="main-content" className="relative flex min-h-screen items-center justify-center bg-brutal-cream px-4 py-16">
      <div className="absolute right-4 top-4 z-20"><LanguageSwitcher /></div>
      <div className="pointer-events-none absolute inset-0 bg-grid opacity-20" aria-hidden />

      <div className="relative w-full max-w-3xl text-center">
        <div className="mb-6 inline-flex h-16 w-16 items-center justify-center rounded-2xl border border-black bg-brutal-primary shadow-brutal-sm">
          <span className="font-heading text-3xl font-bold">S</span>
        </div>

        <h1 className="mb-3 font-heading text-6xl font-bold tracking-tight">Solo</h1>
        <p className="mx-auto mb-10 max-w-xl font-sans text-lg leading-8 text-muted-foreground">
          {t('homeTagline')}
        </p>

        <div className="mb-10 grid gap-4 text-left sm:grid-cols-3">
          <div className="rounded-xl border border-black bg-white p-5 shadow-brutal-sm">
            <div className="mb-2 font-heading text-sm font-bold">{t('homeAgentsTitle')}</div>
            <p className="font-sans text-sm leading-6 text-muted-foreground">{t('homeAgentsDesc')}</p>
          </div>
          <div className="rounded-xl border border-black bg-white p-5 shadow-brutal-sm">
            <div className="mb-2 font-heading text-sm font-bold">{t('homeChannelsTitle')}</div>
            <p className="font-sans text-sm leading-6 text-muted-foreground">{t('homeChannelsDesc')}</p>
          </div>
          <div className="rounded-xl border border-black bg-white p-5 shadow-brutal-sm">
            <div className="mb-2 font-heading text-sm font-bold">{t('homeTasksTitle')}</div>
            <p className="font-sans text-sm leading-6 text-muted-foreground">{t('homeTasksDesc')}</p>
          </div>
        </div>

        <div className="space-y-3">
          <Link href="/auth/register" className="mx-auto block max-w-sm">
            <Button variant="default" className="h-12 w-full text-base">{t('homeGetStarted')}</Button>
          </Link>
          <p className="font-sans text-sm text-muted-foreground">
            {t('homeExistingAccount')}{" "}
            <Link
              href="/auth/login"
              className="font-heading font-bold text-black underline decoration-transparent underline-offset-4 transition-colors hover:decoration-current"
            >
              {t('homeSignIn')}
            </Link>
          </p>
          <p className="font-sans text-sm text-muted-foreground">
            <a
              href="https://github.com/solo-agent/solo"
              target="_blank"
              rel="noreferrer"
              className="font-heading font-bold text-black underline decoration-transparent underline-offset-4 transition-colors hover:decoration-current"
            >
              Solo Agent on GitHub
            </a>
          </p>
        </div>
      </div>
    </main>
  );
}
