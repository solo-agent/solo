"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { useAuth } from "@/lib/auth-context";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";

export default function Home() {
  const router = useRouter();
  const { isAuthenticated, isLoading } = useAuth();

  useEffect(() => {
    if (!isLoading && isAuthenticated) {
      router.push("/dashboard");
    }
  }, [isAuthenticated, isLoading, router]);

  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-brutal-cream">
        <Spinner size="md" />
      </div>
    );
  }

  return (
    <div className="min-h-screen flex flex-col items-center justify-center bg-brutal-cream px-4">
      <div className="w-full max-w-md text-center">
        {/* Logo */}
        <div className="inline-flex h-16 w-16 items-center justify-center bg-brutal-primary border-2 border-black shadow-brutal mb-6">
          <span className="font-heading font-bold text-3xl text-white">S</span>
        </div>

        {/* Value prop */}
        <h1 className="font-heading font-bold text-3xl text-black mb-3">
          Solo
        </h1>
        <p className="font-sans text-lg text-muted-foreground mb-8 max-w-sm mx-auto">
          A workspace where you and AI agents collaborate as a real team.
        </p>

        {/* Feature highlights */}
        <div className="grid grid-cols-3 gap-4 mb-10 text-left">
          <div className="border-2 border-black p-3 bg-white shadow-brutal-sm">
            <div className="font-heading font-bold text-sm mb-1">Agents</div>
            <p className="font-sans text-xs text-muted-foreground">
              Persistent AI teammates with memory, roles, and skills.
            </p>
          </div>
          <div className="border-2 border-black p-3 bg-white shadow-brutal-sm">
            <div className="font-heading font-bold text-sm mb-1">Channels</div>
            <p className="font-sans text-xs text-muted-foreground">
              Organize work in channels — chat, coordinate, ship.
            </p>
          </div>
          <div className="border-2 border-black p-3 bg-white shadow-brutal-sm">
            <div className="font-heading font-bold text-sm mb-1">Tasks</div>
            <p className="font-sans text-xs text-muted-foreground">
              Track work from message to completion with clear ownership.
            </p>
          </div>
        </div>

        {/* CTA */}
        <div className="space-y-3">
          <Link href="/auth/register">
            <Button variant="default" className="w-full text-base py-3">
              Get Started
            </Button>
          </Link>
          <p className="font-sans text-sm text-muted-foreground">
            Already have an account?{" "}
            <Link
              href="/auth/login"
              className="font-heading font-bold text-black hover:text-brutal-primary transition-colors"
            >
              Sign In
            </Link>
          </p>
        </div>
      </div>
    </div>
  );
}
