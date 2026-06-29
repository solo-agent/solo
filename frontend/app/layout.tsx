import type { Metadata } from "next";
import { Inter, Space_Grotesk, Space_Mono } from "next/font/google";
import { AuthProvider } from "@/lib/auth-context";
import { t } from '@/lib/i18n';
import { WSProvider } from "@/lib/ws-context";
import { WSAuthBridge } from "@/components/ws-auth-bridge";
import { ConnectionBanner } from "@/components/connection-banner";
import { NetworkStatus } from "@/components/network-status";
import { ToastProvider } from "@/components/ui/toast";
import { GlobalSearchTrigger } from "@/components/search/global-search-trigger";
import { LocaleHydrator } from "@/components/locale-hydrator";
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

export const metadata: Metadata = {
  title: t('appTitle'),
  description: t('appDescription'),
  icons: {
    icon: "/favicon.svg",
  },
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html
      lang="en"
      className={`${inter.variable} ${spaceGrotesk.variable} ${spaceMono.variable}`}
    >
      <body className="min-h-screen antialiased">
        <AuthProvider>
          <WSProvider>
            <ToastProvider>
              <WSAuthBridge />
              <LocaleHydrator>
                <ConnectionBanner />
                <NetworkStatus />
                <GlobalSearchTrigger />
                {children}
              </LocaleHydrator>
            </ToastProvider>
          </WSProvider>
        </AuthProvider>
      </body>
    </html>
  );
}
