export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="relative min-h-screen flex items-center justify-center bg-brutal-cream px-4 overflow-hidden">
      <div
        className="absolute inset-0 bg-halftone pointer-events-none opacity-60"
        aria-hidden
      />
      <div
        className="absolute inset-0 bg-grid pointer-events-none opacity-40"
        aria-hidden
      />
      <div className="relative w-full max-w-sm">{children}</div>
    </div>
  );
}
