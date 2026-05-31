export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="min-h-screen flex items-center justify-center bg-brutal-cream px-4">
      <div className="w-full max-w-sm">{children}</div>
    </div>
  );
}
