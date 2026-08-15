import type { Metadata } from "next";
import HomeAuthRedirect from "@/components/home-auth-redirect";
import HomePage from "@/components/home-page";

const title = "Solo Agent — Open-Source Multi-Agent Workspace";
const description =
  "Solo Agent (SoloAgent) is an open-source, local-first workspace where humans and AI coding agents collaborate through channels, tasks, teams, and persistent memory.";
const structuredData = {
  "@context": "https://schema.org",
  "@type": "SoftwareApplication",
  name: "Solo Agent",
  alternateName: ["Solo", "SoloAgent", "solo-agent"],
  url: "https://soloagent.team/",
  sameAs: ["https://github.com/solo-agent/solo"],
  applicationCategory: "DeveloperApplication",
  operatingSystem: "macOS, Windows, Linux",
  description,
};

export const metadata: Metadata = {
  title,
  description,
  alternates: {
    canonical: "https://soloagent.team/",
  },
  openGraph: {
    type: "website",
    url: "https://soloagent.team/",
    siteName: "Solo Agent",
    title,
    description,
  },
  twitter: {
    card: "summary",
    title,
    description,
  },
};

export default function Page() {
  return (
    <>
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(structuredData) }}
      />
      <HomeAuthRedirect />
      <HomePage />
    </>
  );
}
