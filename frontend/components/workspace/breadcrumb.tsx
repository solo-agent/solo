'use client';

interface BreadcrumbProps {
  path: string;
  onNavigate: (path: string) => void;
}

export function Breadcrumb({ path, onNavigate }: BreadcrumbProps) {
  // Remove the root indicator if path starts with '/'
  const cleanPath = path.startsWith('/') ? path : '/' + path;
  const segments = cleanPath.split('/').filter(Boolean);

  return (
    <div className="flex items-center gap-1 px-3 py-1.5 bg-white border-b-2 border-black overflow-x-auto">
      {segments.map((seg, i) => {
        const segPath = '/' + segments.slice(0, i + 1).join('/');
        const isLast = i === segments.length - 1;
        return (
          <span key={i} className="flex items-center gap-1">
            {i > 0 && <span className="text-muted-foreground text-xs">/</span>}
            {isLast ? (
              <span className="font-mono text-xs font-bold">{seg}</span>
            ) : (
              <button
                type="button"
                onClick={() => onNavigate(segPath)}
                className="font-mono text-xs text-muted-foreground hover:text-foreground hover:underline"
              >
                {seg}
              </button>
            )}
          </span>
        );
      })}
    </div>
  );
}
