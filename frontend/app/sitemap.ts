import type { MetadataRoute } from "next";

export default function sitemap(): MetadataRoute.Sitemap {
  return [
    {
      url: "https://soloagent.team/",
      changeFrequency: "weekly",
      priority: 1,
    },
  ];
}
