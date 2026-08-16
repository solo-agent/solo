import { LanguageSwitcher } from '@/components/language-switcher';

export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <main id="main-content" className="relative flex min-h-screen items-center justify-center bg-brutal-cream px-4 py-16">
      <div className="fixed right-4 top-4 z-20"><LanguageSwitcher /></div>
      <div className="pointer-events-none absolute inset-0 bg-grid opacity-20" aria-hidden />
      <div className="relative w-full max-w-md">{children}</div>
    </main>
  );
}
