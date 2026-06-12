import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  devIndicators: false,
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
  // v1.5: Content Security Policy header (production only — dev needs inline scripts for HMR)
  async headers() {
    if (process.env.NODE_ENV !== 'production') return [];
    return [
      {
        source: '/(.*)',
        headers: [
          {
            key: 'Content-Security-Policy',
            value:
              "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data: blob:; font-src 'self';",
          },
        ],
      },
    ];
  },
};

export default nextConfig;
