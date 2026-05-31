import type { Metadata } from "next";
import { Inter, Space_Grotesk, Space_Mono, Syne } from "next/font/google";
import { AuthProvider } from "@/lib/auth-context";
import { WSProvider } from "@/lib/ws-context";
import { WSAuthBridge } from "@/components/ws-auth-bridge";
import { ConnectionBanner } from "@/components/connection-banner";
import { NetworkStatus } from "@/components/network-status";
import { ToastProvider } from "@/components/ui/toast";
import { GlobalSearchTrigger } from "@/components/search/global-search-trigger";
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

const syne = Syne({
  subsets: ["latin"],
  weight: ["700", "800"],
  variable: "--font-syne",
});

export const metadata: Metadata = {
  title: "Solo - 频道式多 Agent 协作平台",
  description: "像 Slack 一样组织人+AI 的协作空间",
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
      lang="zh-CN"
      className={`${inter.variable} ${spaceGrotesk.variable} ${spaceMono.variable} ${syne.variable}`}
    >
      <body className="min-h-screen antialiased">
        <AuthProvider>
          <WSProvider>
            <ToastProvider>
              <WSAuthBridge />
              <ConnectionBanner />
              <NetworkStatus />
              <GlobalSearchTrigger />
              {children}
            </ToastProvider>
          </WSProvider>
        </AuthProvider>
      </body>
    </html>
  );
}
