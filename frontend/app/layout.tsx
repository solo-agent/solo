import type { Metadata, Viewport } from "next";
import Script from "next/script";
import {
  Inter,
  Literata,
  Noto_Sans_SC,
  Noto_Serif_SC,
  Space_Grotesk,
  Space_Mono,
} from "next/font/google";
import { AuthProvider } from "@/lib/auth-context";
import { t } from '@/lib/i18n';
import { WSProvider } from "@/lib/ws-context";
import { WorkspaceProvider } from "@/lib/workspace-context";
import { WSAuthBridge } from "@/components/ws-auth-bridge";
import { ConnectionBanner } from "@/components/connection-banner";
import { NetworkStatus } from "@/components/network-status";
import { ToastProvider } from "@/components/ui/toast";
import { GlobalSearchTrigger } from "@/components/search/global-search-trigger";
import { LocaleHydrator } from "@/components/locale-hydrator";
import { FirstRunWizard } from "@/components/onboarding/first-run-wizard";
import "./globals.brutal.css";

const inter = Inter({
  subsets: ["latin"],
  variable: "--font-inter",
});

const spaceGrotesk = Space_Grotesk({
  subsets: ["latin"],
  variable: "--font-space-grotesk",
});

const spaceMono = Space_Mono({
  subsets: ["latin"],
  weight: ["400", "700"],
  variable: "--font-space-mono",
});

const literata = Literata({
  subsets: ["latin"],
  weight: ["400", "600", "700"],
  variable: "--font-literata",
});

const notoSansSC = Noto_Sans_SC({
  weight: "variable",
  display: "swap",
  preload: false,
  variable: "--font-noto-sans-sc",
});

const notoSerifSC = Noto_Serif_SC({
  weight: "variable",
  display: "swap",
  preload: false,
  variable: "--font-noto-serif-sc",
});

const bootstrapScript = `try{const skin=localStorage.getItem("solo.skin");document.documentElement.dataset.skin=["archive","classic"].includes(skin)?skin:"archive";const storedLocale=localStorage.getItem("solo.locale");document.documentElement.lang=storedLocale==="zh-CN"||(!storedLocale&&navigator.language.toLowerCase().startsWith("zh"))?"zh-CN":"en"}catch{document.documentElement.dataset.skin="archive"}`;

export const metadata: Metadata = {
  title: t('appTitle'),
  description: t('appDescription'),
  icons: {
    icon: "/favicon.svg",
  },
};

export const viewport: Viewport = {
  themeColor: '#f8f5ef',
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html
      lang="en"
      data-skin="archive"
      suppressHydrationWarning
      className={`${inter.variable} ${spaceGrotesk.variable} ${spaceMono.variable} ${literata.variable} ${notoSansSC.variable} ${notoSerifSC.variable}`}
    >
      <head>
        <Script id="solo-bootstrap" strategy="beforeInteractive">{bootstrapScript}</Script>
      </head>
      <body className="min-h-screen antialiased">
        <AuthProvider>
          <WorkspaceProvider>
            <WSProvider>
              <ToastProvider>
                <WSAuthBridge />
                <LocaleHydrator>
                  <FirstRunWizard />
                  <ConnectionBanner />
                  <NetworkStatus />
                  <GlobalSearchTrigger />
                  {children}
                </LocaleHydrator>
              </ToastProvider>
            </WSProvider>
          </WorkspaceProvider>
        </AuthProvider>
      </body>
    </html>
  );
}
