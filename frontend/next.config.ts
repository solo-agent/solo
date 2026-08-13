import type { NextConfig } from "next";

const apiOrigin = (() => {
  try {
    return new URL(process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080').origin;
  } catch {
    return '';
  }
})();

const guestScriptSource = process.env.NODE_ENV === 'production'
  ? "'self' 'unsafe-inline'"
  : "'self' 'unsafe-inline' 'unsafe-eval'";

const nextConfig: NextConfig = {
  devIndicators: false,
  turbopack: {
    root: __dirname,
  },
  // The retired standalone Agent page now lives in the Channel workspace.
  // /teams intentionally has no redirect: it is gone and should return 404.
  async redirects() {
    return [
      {
        source: '/agents',
        destination: '/dashboard',
        permanent: true,
      },
      {
        source: '/agents/:path*',
        destination: '/dashboard',
        permanent: true,
      },
    ];
  },
  // v1.5: Content Security Policy header (production only — dev needs inline scripts for HMR)
  async headers() {
    const embedHeaders = {
      source: '/guest/:path*',
      headers: [
        { key: 'Content-Security-Policy', value: `default-src 'self'; script-src ${guestScriptSource}; style-src 'self' 'unsafe-inline'; connect-src 'self' ${apiOrigin}; img-src 'self' data: blob: ${apiOrigin}; font-src 'self'; frame-ancestors *;` },
        { key: 'Referrer-Policy', value: 'no-referrer' },
      ],
    };
    if (process.env.NODE_ENV !== 'production') return [embedHeaders];
    return [embedHeaders,
      {
        source: '/((?!guest/).*)',
        headers: [
          {
            key: 'Content-Security-Policy',
            value:
              `default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data: blob: ${apiOrigin}; font-src 'self';`,
          },
        ],
      },
    ];
  },
};

export default nextConfig;
