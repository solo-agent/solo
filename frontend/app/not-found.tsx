// ============================================================================
// 404 Not Found page — neubrutalist styling
// ============================================================================

import Link from 'next/link';
import { FileQuestion } from 'lucide-react';

export default function NotFoundPage() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center bg-brutal-cream px-4">
      <div className="mx-auto flex max-w-sm flex-col items-center text-center">
        <div className="mb-6 flex h-20 w-20 items-center justify-center border-2 border-black bg-brutal-pink shadow-brutal">
          <FileQuestion className="h-10 w-10 text-black" />
        </div>
        <h1 className="font-heading text-4xl font-bold text-foreground">404</h1>
        <p className="mt-2 font-heading text-lg font-bold text-foreground">
          页面不存在
        </p>
        <p className="mt-2 font-body text-sm text-muted-foreground">
          你访问的页面不存在或已被移除。请检查链接是否正确。
        </p>
        <div className="mt-8 flex gap-4">
          <Link
            href="/dashboard"
            className="btn-brutal btn-brutal-pink px-5 py-2.5 text-sm"
          >
            返回工作台
          </Link>
          <Link
            href="/auth/login"
            className="btn-brutal bg-white px-5 py-2.5 text-sm"
          >
            返回登录
          </Link>
        </div>
      </div>
    </div>
  );
}
