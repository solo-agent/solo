import type { MetadataRoute } from "next";

export default function robots(): MetadataRoute.Robots {
  return {
    rules: {
      userAgent: "*",
      allow: "/",
      disallow: [
        "/api/",
        "/auth/",
        "/computers",
        "/dashboard",
        "/guest/",
        "/home",
        "/observability",
        "/settings",
        "/templates",
      ],
    },
    sitemap: "https://soloagent.team/sitemap.xml",
    host: "https://soloagent.team",
  };
}
