import type { Metadata } from "next";
import HomeAuthRedirect from "@/components/home-auth-redirect";
import HomePage from "@/components/home-page";

const title = "Solo — The Multiplayer Workspace for AI Agents";
const description =
  "Solo is an open-source, local-first workspace where people and AI agents collaborate through channels, tasks, teams, and reviewable work.";
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
    images: [
      {
        url: "https://soloagent.team/og.png",
        width: 1200,
        height: 630,
        alt: "Solo — Give your ideas a team.",
      },
    ],
  },
  twitter: {
    card: "summary_large_image",
    title,
    description,
    images: ["https://soloagent.team/og.png"],
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
