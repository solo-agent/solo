import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  turbopack: {
    root: __dirname,
  },
  // v1.5: Redirect /agents/* to /teams
  async redirects() {
    return [
      {
        source: '/agents',
        destination: '/teams',
        permanent: true,
      },
      {
        source: '/agents/:path*',
        destination: '/teams',
        permanent: true,
      },
    ];
  },
};

export default nextConfig;
