"use client";

import Image from "next/image";
import Link from "next/link";
import {
  ArrowRight,
  Bot,
  CheckCircle2,
  Github,
  Languages,
  Monitor,
  Network,
  PanelsTopLeft,
  Radio,
  ShieldCheck,
  Sparkles,
  UsersRound,
  Workflow,
} from "lucide-react";
import { PixelAvatar } from "@/components/ui/pixel-avatar";
import { getLocale, languageOptions, setLocale, t, type Locale } from "@/lib/i18n";

export default function HomePage() {
  return (
    <main id="main-content" className="min-h-screen bg-skin-canvas text-skin-ink">
      <header className="border-b border-skin-rule bg-skin-canvas/95">
        <nav className="mx-auto flex h-20 max-w-[1280px] items-center justify-between gap-6 px-5 lg:px-10" aria-label={t('homeNavLabel')}>
          <Link href="/" className="flex items-center gap-3" aria-label={t('homeLogoLabel')}>
            <Image src="/favicon.svg" width={42} height={42} alt="" priority />
            <span className="font-display text-2xl font-bold tracking-[-0.04em]">Solo</span>
          </Link>

          <div className="hidden items-center gap-7 font-heading text-sm font-semibold md:flex">
            <a href="#product" className="hover:text-skin-accent">{t('homeNavProduct')}</a>
            <a href="#workflow" className="hover:text-skin-accent">{t('homeNavWorkflow')}</a>
            <a href="#open-source" className="hover:text-skin-accent">{t('homeNavOpenSource')}</a>
            <a href="https://github.com/solo-agent/solo" target="_blank" rel="noreferrer" className="inline-flex items-center gap-1.5 hover:text-skin-accent">
              GitHub <span aria-hidden>↗</span>
            </a>
          </div>

          <div className="flex items-center gap-3">
            <label className="relative inline-flex h-10 w-10 items-center justify-center rounded-xl border border-skin-rule bg-skin-surface sm:hidden">
              <Languages className="h-4 w-4 text-skin-subtle-text" aria-hidden />
              <span className="sr-only">{t('languageLabel')}</span>
              <select
                aria-label={t('languageLabel')}
                value={getLocale()}
                onChange={(event) => setLocale(event.target.value as Locale)}
                className="absolute inset-0 cursor-pointer opacity-0"
              >
                {languageOptions.map((option) => (
                  <option key={option.value} value={option.value}>{option.label}</option>
                ))}
              </select>
            </label>
            <label className="hidden items-center gap-2 rounded-xl border border-skin-rule bg-skin-surface px-3 py-2 sm:inline-flex">
              <Languages className="h-4 w-4 text-skin-subtle-text" aria-hidden />
              <span className="sr-only">{t('languageLabel')}</span>
              <select
                aria-label={t('languageLabel')}
                value={getLocale()}
                onChange={(event) => setLocale(event.target.value as Locale)}
                className="bg-transparent font-heading text-xs font-semibold outline-none"
              >
                {languageOptions.map((option) => (
                  <option key={option.value} value={option.value}>{option.label}</option>
                ))}
              </select>
            </label>
            <Link
              href="/auth/login"
              className="inline-flex h-10 items-center gap-2 rounded-xl border border-skin-rule bg-skin-surface px-4 font-heading text-sm font-bold shadow-[0_3px_10px_var(--skin-shadow)] transition-transform hover:-translate-y-0.5"
            >
              {t('homeOpenSolo')} <ArrowRight className="h-4 w-4" aria-hidden />
            </Link>
          </div>
        </nav>
      </header>

      <section className="relative overflow-hidden border-b border-skin-rule bg-skin-primary-light" aria-labelledby="home-hero-title">
        <div className="pointer-events-none absolute inset-0 bg-halftone opacity-[0.1]" aria-hidden />
        <div className="relative mx-auto grid max-w-[1280px] gap-12 px-5 py-16 lg:grid-cols-[0.9fr_1.1fr] lg:items-center lg:px-10 lg:py-20">
          <div className="relative z-10 max-w-[580px]">
            <p className="mb-7 inline-flex rounded-full border border-skin-rule bg-skin-surface px-4 py-2 font-mono text-[10px] font-bold uppercase tracking-[0.18em] text-skin-subtle-text shadow-[0_4px_12px_var(--skin-shadow)]">
              {t('homeHeroEyebrow')}
            </p>
            <h1 id="home-hero-title" className="font-display text-[clamp(3.25rem,5vw,4.5rem)] font-bold leading-[0.98] tracking-[-0.045em]">
              {t('homeHeroTitle')}
            </h1>
            <p className="mt-7 max-w-[560px] font-body text-base leading-7 text-skin-subtle-text lg:text-lg lg:leading-8">
              {t('homeHeroDescription')}
            </p>
            <div className="mt-9 flex flex-wrap items-center gap-4">
              <Link
                href="/auth/register"
                className="inline-flex h-13 items-center gap-2 rounded-xl border border-skin-rule bg-skin-accent px-6 font-heading text-base font-bold text-skin-accent-foreground shadow-[0_5px_14px_var(--skin-shadow)] transition-transform hover:-translate-y-0.5"
              >
                {t('homeHeroCta')} <ArrowRight className="h-4 w-4" aria-hidden />
              </Link>
              <a
                href="https://github.com/solo-agent/solo"
                target="_blank"
                rel="noreferrer"
                className="inline-flex h-13 items-center gap-2 rounded-xl border border-skin-rule bg-skin-surface px-6 font-heading text-base font-bold shadow-[0_5px_14px_var(--skin-shadow)] transition-transform hover:-translate-y-0.5"
              >
                <Github className="h-4 w-4" aria-hidden /> {t('homeHeroGithub')}
              </a>
            </div>
            <p className="mt-6 font-mono text-[11px] uppercase tracking-[0.13em] text-skin-subtle-text">
              {t('homeHeroProof')}
            </p>
            <div className="mt-8 grid max-w-[500px] grid-cols-3 gap-5">
              {[
                ['02', 'humans'],
                ['03', 'agents'],
                ['02', 'computers'],
              ].map(([value, label], index) => (
                <div key={label} className={`py-2 ${index > 0 ? 'border-l border-skin-rule pl-5' : ''}`}>
                  <p className="font-display text-xl font-bold">{value}</p>
                  <p className="font-mono text-[9px] font-bold uppercase tracking-[0.14em] text-skin-subtle-text">{label}</p>
                </div>
              ))}
            </div>
          </div>

          <div className="relative mx-auto w-full max-w-[720px]" id="product">
            <div className="relative z-10 overflow-hidden rounded-[28px] border border-skin-rule bg-skin-surface shadow-[0_26px_70px_var(--skin-shadow)]">
              <div className="flex items-center justify-between border-b border-skin-rule px-5 py-4">
                <div className="flex items-center gap-2">
                  <span className="h-2.5 w-2.5 rounded-full bg-skin-accent" />
                  <span className="h-2.5 w-2.5 rounded-full bg-skin-warning" />
                  <span className="h-2.5 w-2.5 rounded-full bg-skin-success" />
                </div>
                <p className="font-mono text-[10px] font-bold uppercase tracking-[0.15em] text-skin-subtle-text">Solo / launch-room</p>
                <Radio className="h-4 w-4 text-skin-success" aria-hidden />
              </div>

              <div className="grid min-h-[470px] grid-cols-[112px_1fr] sm:grid-cols-[190px_1fr]">
                <aside className="border-r border-skin-rule bg-skin-primary-light p-4">
                  <p className="font-mono text-[9px] font-bold uppercase tracking-[0.16em] text-skin-subtle-text">Channels</p>
                  <div className="mt-3 rounded-xl border border-skin-rule bg-skin-surface px-3 py-2 font-heading text-sm font-bold"># launch-room</div>
                  <p className="mt-6 font-mono text-[9px] font-bold uppercase tracking-[0.16em] text-skin-subtle-text">People</p>
                  <div className="mt-3 space-y-3">
                    {[
                      ['lucy', 'Lucy', 'online'],
                      ['research', 'Research', 'running'],
                      ['builder', 'Builder', 'online'],
                    ].map(([id, name, status]) => (
                      <div key={id} className="flex items-center gap-2.5">
                        <PixelAvatar agentId={id} size="sm" className="[border-color:var(--skin-rule)] [border-width:1px] shadow-none" />
                        <div className="min-w-0">
                          <p className="truncate font-heading text-xs font-bold">{name}</p>
                          <p className="font-mono text-[8px] uppercase tracking-[0.1em] text-skin-subtle-text">{status}</p>
                        </div>
                      </div>
                    ))}
                  </div>
                  <div className="mt-7 border-t border-skin-rule pt-4">
                    <p className="flex items-center gap-2 font-mono text-[9px] font-bold uppercase tracking-[0.12em] text-skin-subtle-text">
                      <Monitor className="h-3.5 w-3.5" aria-hidden /> 2 computers
                    </p>
                  </div>
                </aside>

                <div className="flex min-w-0 flex-col">
                  <div className="flex items-center justify-between border-b border-skin-rule px-4 py-4 sm:px-6">
                    <div>
                      <p className="font-display text-lg font-bold"># launch-room</p>
                      <p className="mt-0.5 font-mono text-[9px] uppercase tracking-[0.12em] text-skin-subtle-text">3 agents · 2 humans</p>
                    </div>
                    <UsersRound className="h-5 w-5 text-skin-subtle-text" aria-hidden />
                  </div>

                  <div className="flex-1 space-y-5 p-4 sm:p-6">
                    <div className="flex gap-3">
                      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-skin-rule bg-skin-warning-light font-heading text-xs font-bold">F</div>
                      <div>
                        <p className="font-heading text-xs font-bold">Fred <span className="ml-1 font-mono text-[9px] font-normal text-skin-subtle-text">09:41</span></p>
                        <p className="mt-1 max-w-[410px] font-body text-sm leading-6">{t('homeDemoRequest')}</p>
                      </div>
                    </div>

                    <div className="ml-2 rounded-2xl border border-skin-rule bg-skin-accent-light p-4 shadow-[0_4px_12px_var(--skin-shadow)] sm:ml-10">
                      <div className="flex items-start gap-3">
                        <PixelAvatar agentId="lucy" size="sm" className="[border-color:var(--skin-rule)] [border-width:1px] shadow-none" />
                        <div>
                          <p className="font-heading text-xs font-bold">Lucy <span className="ml-1 rounded-full bg-skin-success-light px-2 py-0.5 font-mono text-[8px] uppercase">coordinating</span></p>
                          <p className="mt-2 font-body text-sm leading-6">{t('homeDemoLucy')}</p>
                        </div>
                      </div>
                      <div className="mt-4 grid gap-2 sm:grid-cols-2">
                        <div className="rounded-xl border border-skin-rule bg-skin-surface p-3">
                          <p className="font-mono text-[8px] font-bold uppercase tracking-[0.1em] text-skin-subtle-text">Research</p>
                          <p className="mt-1 font-heading text-xs font-bold">{t('homeDemoResearch')}</p>
                        </div>
                        <div className="rounded-xl border border-skin-rule bg-skin-surface p-3">
                          <p className="font-mono text-[8px] font-bold uppercase tracking-[0.1em] text-skin-subtle-text">Builder</p>
                          <p className="mt-1 font-heading text-xs font-bold">{t('homeDemoBuilder')}</p>
                        </div>
                      </div>
                    </div>
                  </div>

                  <div className="border-t border-skin-rule px-4 py-4 sm:px-6">
                    <div className="flex items-center justify-between rounded-xl border border-skin-rule bg-skin-primary-light px-4 py-3">
                      <div>
                        <p className="font-mono text-[8px] font-bold uppercase tracking-[0.12em] text-skin-subtle-text">Task #12</p>
                        <p className="mt-1 truncate font-heading text-xs font-bold">{t('homeDemoTask')}</p>
                      </div>
                      <span className="ml-3 shrink-0 rounded-full bg-skin-success-light px-2.5 py-1 font-mono text-[8px] font-bold uppercase">in progress</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section className="border-b border-skin-rule bg-skin-surface" aria-labelledby="home-thesis-title">
        <div className="mx-auto grid max-w-[1280px] gap-10 px-5 py-16 lg:grid-cols-[1.2fr_0.8fr] lg:items-end lg:px-10 lg:py-24">
          <div>
            <p className="font-mono text-[11px] font-bold uppercase tracking-[0.2em] text-skin-subtle-text">{t('homeThesisEyebrow')}</p>
            <h2 id="home-thesis-title" className="mt-5 max-w-[780px] font-display text-[clamp(2.2rem,3.6vw,3.25rem)] font-bold leading-[1.04] tracking-[-0.04em]">
              {t('homeThesisTitle')}
            </h2>
          </div>
          <div className="space-y-4 border-l border-skin-rule pl-6">
            {[t('homeThesisPointOne'), t('homeThesisPointTwo'), t('homeThesisPointThree')].map((point) => (
              <p key={point} className="flex items-start gap-3 font-body text-base leading-7 text-skin-subtle-text">
                <CheckCircle2 className="mt-1 h-4 w-4 shrink-0 text-skin-accent" aria-hidden />
                {point}
              </p>
            ))}
          </div>
        </div>
      </section>

      <section id="workflow" className="border-b border-skin-rule" aria-labelledby="home-workflow-title">
        <div className="mx-auto max-w-[1280px] px-5 py-16 lg:px-10 lg:py-24">
          <div className="max-w-[820px]">
            <p className="font-mono text-[11px] font-bold uppercase tracking-[0.2em] text-skin-subtle-text">{t('homeWorkflowEyebrow')}</p>
            <h2 id="home-workflow-title" className="mt-5 font-display text-[clamp(2.2rem,3.6vw,3.25rem)] font-bold leading-[1.04] tracking-[-0.04em]">
              {t('homeWorkflowTitle')}
            </h2>
            <p className="mt-6 max-w-[680px] font-body text-lg leading-8 text-skin-subtle-text">{t('homeWorkflowDescription')}</p>
          </div>

          <div className="mt-14 grid border-y border-skin-rule md:grid-cols-3">
            {[
              { number: '01', icon: Sparkles, title: t('homeStepOneTitle'), description: t('homeStepOneDescription') },
              { number: '02', icon: Workflow, title: t('homeStepTwoTitle'), description: t('homeStepTwoDescription') },
              { number: '03', icon: CheckCircle2, title: t('homeStepThreeTitle'), description: t('homeStepThreeDescription') },
            ].map(({ number, icon: Icon, title, description }, index) => (
              <article key={number} className={`py-8 md:px-8 md:py-10 ${index > 0 ? 'border-t border-skin-rule md:border-l md:border-t-0' : ''}`}>
                <div className="flex items-center justify-between">
                  <span className="font-mono text-xs font-bold tracking-[0.16em] text-skin-subtle-text">{number}</span>
                  <span className="flex h-11 w-11 items-center justify-center rounded-xl border border-skin-rule bg-skin-accent-light">
                    <Icon className="h-5 w-5" aria-hidden />
                  </span>
                </div>
                <h3 className="mt-9 font-display text-2xl font-bold tracking-[-0.035em]">{title}</h3>
                <p className="mt-3 font-body text-base leading-7 text-skin-subtle-text">{description}</p>
              </article>
            ))}
          </div>
        </div>
      </section>

      <section className="overflow-hidden border-b border-skin-rule bg-skin-primary-light" aria-labelledby="home-workspace-title">
        <div className="mx-auto max-w-[1280px] px-5 py-16 lg:px-10 lg:py-24">
          <div className="grid gap-8 lg:grid-cols-[0.8fr_1.2fr] lg:items-end">
            <div>
              <p className="font-mono text-[11px] font-bold uppercase tracking-[0.2em] text-skin-subtle-text">{t('homeWorkspaceEyebrow')}</p>
              <h2 id="home-workspace-title" className="mt-5 font-display text-[clamp(2.2rem,3.6vw,3.25rem)] font-bold leading-[1.04] tracking-[-0.04em]">
                {t('homeWorkspaceTitle')}
              </h2>
            </div>
            <p className="max-w-[610px] font-body text-lg leading-8 text-skin-subtle-text lg:justify-self-end">{t('homeWorkspaceDescription')}</p>
          </div>

          <div className="mt-12 overflow-hidden rounded-[28px] border border-skin-rule bg-skin-surface p-2 shadow-[0_26px_70px_var(--skin-shadow)] lg:p-3">
            <Image
              src="/marketing/workspace.png"
              width={1908}
              height={1096}
              sizes="(max-width: 1280px) 94vw, 1200px"
              alt={t('homeWorkspaceImageAlt')}
              className="h-auto w-full rounded-[20px] border border-skin-rule"
            />
          </div>
          <div className="mt-8 grid gap-3 sm:grid-cols-3">
            {[t('homeWorkspaceNoteOne'), t('homeWorkspaceNoteTwo'), t('homeWorkspaceNoteThree')].map((note) => (
              <p key={note} className="rounded-xl border border-skin-rule bg-skin-surface px-4 py-3 font-mono text-[10px] font-bold uppercase tracking-[0.1em] text-skin-subtle-text">{note}</p>
            ))}
          </div>
        </div>
      </section>

      <section className="border-b border-skin-rule" aria-labelledby="home-capabilities-title">
        <div className="mx-auto max-w-[1280px] px-5 py-16 lg:px-10 lg:py-24">
          <div className="max-w-[850px]">
            <p className="font-mono text-[11px] font-bold uppercase tracking-[0.2em] text-skin-subtle-text">{t('homeCapabilitiesEyebrow')}</p>
            <h2 id="home-capabilities-title" className="mt-5 font-display text-[clamp(2.2rem,3.6vw,3.25rem)] font-bold leading-[1.04] tracking-[-0.04em]">
              {t('homeCapabilitiesTitle')}
            </h2>
          </div>

          <div className="mt-14 grid gap-px overflow-hidden rounded-[26px] border border-skin-rule bg-skin-rule md:grid-cols-2">
            {[
              { icon: UsersRound, title: t('homeCapabilityOneTitle'), description: t('homeCapabilityOneDescription') },
              { icon: Network, title: t('homeCapabilityTwoTitle'), description: t('homeCapabilityTwoDescription') },
              { icon: Monitor, title: t('homeCapabilityThreeTitle'), description: t('homeCapabilityThreeDescription') },
              { icon: PanelsTopLeft, title: t('homeCapabilityFourTitle'), description: t('homeCapabilityFourDescription') },
            ].map(({ icon: Icon, title, description }) => (
              <article key={title} className="bg-skin-surface p-7 sm:p-10 lg:p-12">
                <span className="flex h-12 w-12 items-center justify-center rounded-xl border border-skin-rule bg-skin-accent-light">
                  <Icon className="h-5 w-5" aria-hidden />
                </span>
                <h3 className="mt-8 font-display text-2xl font-bold tracking-[-0.035em] lg:text-3xl">{title}</h3>
                <p className="mt-4 max-w-[520px] font-body text-base leading-7 text-skin-subtle-text">{description}</p>
              </article>
            ))}
          </div>
        </div>
      </section>

      <section className="overflow-hidden border-b border-skin-rule bg-skin-surface" aria-labelledby="home-templates-title">
        <div className="mx-auto grid max-w-[1280px] gap-12 px-5 py-16 lg:grid-cols-[0.7fr_1.3fr] lg:items-center lg:px-10 lg:py-24">
          <div>
            <span className="inline-flex h-12 w-12 items-center justify-center rounded-xl border border-skin-rule bg-skin-warning-light">
              <Bot className="h-5 w-5" aria-hidden />
            </span>
            <p className="mt-8 font-mono text-[11px] font-bold uppercase tracking-[0.2em] text-skin-subtle-text">{t('homeTemplatesEyebrow')}</p>
            <h2 id="home-templates-title" className="mt-5 font-display text-[clamp(2.2rem,3.6vw,3.25rem)] font-bold leading-[1.04] tracking-[-0.04em]">
              {t('homeTemplatesTitle')}
            </h2>
            <p className="mt-6 max-w-[520px] font-body text-lg leading-8 text-skin-subtle-text">{t('homeTemplatesDescription')}</p>
          </div>
          <div className="overflow-hidden rounded-[26px] border border-skin-rule bg-skin-primary-light p-2 shadow-[0_22px_60px_var(--skin-shadow)]">
            <Image
              src="/marketing/templates.png"
              width={2000}
              height={1017}
              sizes="(max-width: 1024px) 94vw, 760px"
              alt={t('homeTemplatesImageAlt')}
              className="h-auto w-full rounded-[18px] border border-skin-rule"
            />
          </div>
        </div>
      </section>

      <section id="open-source" className="border-b border-skin-rule" aria-labelledby="home-open-source-title">
        <div className="mx-auto max-w-[1280px] px-5 py-16 lg:px-10 lg:py-24">
          <div className="relative overflow-hidden rounded-[30px] border border-skin-rule bg-skin-ink px-6 py-12 text-skin-canvas shadow-[0_24px_65px_var(--skin-shadow)] sm:px-10 lg:px-14 lg:py-16">
            <div className="pointer-events-none absolute inset-0 bg-halftone opacity-[0.08]" aria-hidden />
            <div className="relative grid gap-12 lg:grid-cols-[1fr_0.9fr] lg:items-end">
              <div>
                <p className="font-mono text-[11px] font-bold uppercase tracking-[0.2em] text-skin-muted">{t('homeOpenSourceEyebrow')}</p>
                <h2 id="home-open-source-title" className="mt-5 max-w-[650px] font-display text-[clamp(2.3rem,3.8vw,3.5rem)] font-bold leading-[1.02] tracking-[-0.04em]">
                  {t('homeOpenSourceTitle')}
                </h2>
                <p className="mt-6 max-w-[650px] font-body text-lg leading-8 text-skin-muted-light">{t('homeOpenSourceDescription')}</p>
              </div>
              <div>
                <div className="rounded-2xl border border-white/20 bg-white/10 p-5 font-mono text-xs leading-7 text-white sm:p-6 sm:text-sm">
                  <p><span className="text-skin-warning">$</span> git clone git@github.com:solo-agent/solo.git</p>
                  <p><span className="text-skin-warning">$</span> cd solo &amp;&amp; make dev</p>
                </div>
                <div className="mt-5 flex flex-wrap gap-4">
                  <a href="https://github.com/solo-agent/solo" target="_blank" rel="noreferrer" className="inline-flex h-12 items-center gap-2 rounded-xl border border-white/20 bg-skin-accent px-5 font-heading text-sm font-bold text-skin-accent-foreground transition-transform hover:-translate-y-0.5">
                    <Github className="h-4 w-4" aria-hidden /> {t('homeOpenSourceGithub')}
                  </a>
                  <a href="https://github.com/solo-agent/solo#quick-start" target="_blank" rel="noreferrer" className="inline-flex h-12 items-center gap-2 rounded-xl border border-white/20 bg-white/10 px-5 font-heading text-sm font-bold text-white transition-transform hover:-translate-y-0.5">
                    {t('homeOpenSourceInstall')} <ArrowRight className="h-4 w-4" aria-hidden />
                  </a>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section className="relative overflow-hidden" aria-labelledby="home-final-title">
        <div className="pointer-events-none absolute inset-0 bg-noise opacity-[0.1]" aria-hidden />
        <div className="relative mx-auto max-w-[900px] px-5 py-20 text-center lg:py-28">
          <span className="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl border border-skin-rule bg-skin-accent-light shadow-[0_6px_18px_var(--skin-shadow)]">
            <ShieldCheck className="h-6 w-6" aria-hidden />
          </span>
          <h2 id="home-final-title" className="mt-8 font-display text-[clamp(2.5rem,4.4vw,4rem)] font-bold leading-[1] tracking-[-0.045em]">{t('homeFinalTitle')}</h2>
          <p className="mx-auto mt-7 max-w-[660px] font-body text-lg leading-8 text-skin-subtle-text">{t('homeFinalDescription')}</p>
          <div className="mt-9 flex flex-wrap justify-center gap-4">
            <Link href="/auth/register" className="inline-flex h-13 items-center gap-2 rounded-xl border border-skin-rule bg-skin-accent px-6 font-heading text-base font-bold text-skin-accent-foreground shadow-[0_5px_14px_var(--skin-shadow)] transition-transform hover:-translate-y-0.5">
              {t('homeFinalCta')} <ArrowRight className="h-4 w-4" aria-hidden />
            </Link>
            <Link href="/auth/login" className="inline-flex h-13 items-center gap-2 rounded-xl border border-skin-rule bg-skin-surface px-6 font-heading text-base font-bold shadow-[0_5px_14px_var(--skin-shadow)] transition-transform hover:-translate-y-0.5">
              {t('homeFinalSignIn')}
            </Link>
          </div>
        </div>
      </section>

      <footer className="border-t border-skin-rule bg-skin-surface">
        <div className="mx-auto flex max-w-[1280px] flex-col gap-6 px-5 py-8 sm:flex-row sm:items-center sm:justify-between lg:px-10">
          <div className="flex items-center gap-3">
            <Image src="/favicon.svg" width={34} height={34} alt="" />
            <div>
              <p className="font-display text-lg font-bold">Solo</p>
              <p className="font-mono text-[9px] uppercase tracking-[0.12em] text-skin-subtle-text">{t('homeFooterTagline')}</p>
            </div>
          </div>
          <div className="flex flex-wrap gap-6 font-heading text-sm font-semibold">
            <a href="#product" className="hover:text-skin-accent">{t('homeNavProduct')}</a>
            <a href="#workflow" className="hover:text-skin-accent">{t('homeNavWorkflow')}</a>
            <a href="https://github.com/solo-agent/solo" target="_blank" rel="noreferrer" className="hover:text-skin-accent">GitHub ↗</a>
          </div>
        </div>
      </footer>
    </main>
  );
}
