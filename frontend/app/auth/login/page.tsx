"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import { useAuth } from "@/lib/auth-context";
import { t } from '@/lib/i18n';
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { BrutalAlert } from "@/components/ui/brutal-alert";

const loginFormSchema = z.object({
  email: z
    .string()
    .min(1, t('emailRequired'))
    .email(t('emailInvalid')),
  password: z
    .string()
    .min(1, t('passwordRequired'))
    .min(8, t('passwordMinLength')),
});

type LoginFormValues = z.infer<typeof loginFormSchema>;

export default function LoginPage() {
  const router = useRouter();
  const { login, isAuthenticated, isLoading, error, clearError } = useAuth();

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<LoginFormValues>({
    resolver: zodResolver(loginFormSchema),
    defaultValues: {
      email: "",
      password: "",
    },
  });

  // If already authenticated, redirect to dashboard
  useEffect(() => {
    if (!isLoading && isAuthenticated) {
      router.push("/dashboard");
    }
  }, [isAuthenticated, isLoading, router]);

  async function onSubmit(data: LoginFormValues) {
    clearError();
    try {
      await login({ email: data.email, password: data.password });
      router.push("/dashboard");
    } catch {
      // Error is set in auth context, displayed below
    }
  }

  if (isLoading) {
    return (
      <div className="card-brutal p-12 w-full">
        <div className="flex flex-col items-center gap-3">
          <Spinner size="md" />
          <p className="text-sm text-muted-foreground">{t('checkingAuth')}</p>
        </div>
      </div>
    );
  }

  return (
    <div className="card-brutal p-6 w-full">
      {/* Logo + Title */}
      <div className="text-center mb-6">
        <div className="inline-flex h-10 w-10 items-center justify-center bg-brutal-primary border-2 border-black shadow-brutal-sm mb-3">
          <span className="font-heading font-bold text-lg text-black">S</span>
        </div>
        <h1 className="font-heading font-bold text-xl text-black">{t('welcomeBack')}</h1>
        <p className="font-sans text-sm text-muted-foreground mt-1">{t('loginToSolo')}</p>
      </div>

      <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
        {/* API Error */}
        {error && <BrutalAlert variant="error">{error}</BrutalAlert>}

        {/* Email field */}
        <div className="space-y-2">
          <label
            htmlFor="email"
            className="font-heading font-bold text-sm block"
          >
            {t('email')}
          </label>
          <input
            id="email"
            type="email"
            placeholder="name@example.com"
            autoComplete="email"
            disabled={isSubmitting}
            aria-invalid={!!errors.email}
            className={`input-brutal ${errors.email ? "input-error" : ""}`}
            {...register("email")}
          />
          {errors.email && (
            <p className="text-destructive text-sm" role="alert">
              {errors.email.message}
            </p>
          )}
        </div>

        {/* Password field */}
        <div className="space-y-2">
          <label
            htmlFor="password"
            className="font-heading font-bold text-sm block"
          >
            {t('password')}
          </label>
          <input
            id="password"
            type="password"
            placeholder={t('enterPassword')}
            autoComplete="current-password"
            disabled={isSubmitting}
            aria-invalid={!!errors.password}
            className={`input-brutal ${errors.password ? "input-error" : ""}`}
            {...register("password")}
          />
          {errors.password && (
            <p className="text-destructive text-sm" role="alert">
              {errors.password.message}
            </p>
          )}
        </div>

        {/* Submit button */}
        <Button
          type="submit"
          variant="default"
          className="w-full"
          disabled={isSubmitting}
        >
          {isSubmitting ? t('loggingIn') : t('login')}
        </Button>
      </form>

      {/* Register link */}
      <div className="text-center mt-6 pt-4 border-t-2 border-black">
        <p className="font-sans text-sm text-muted-foreground">
          {t('noAccount')}{" "}
          <Link
            href="/auth/register"
            className="font-heading font-bold text-black hover:text-brutal-primary transition-colors"
          >
            {t('register')}
          </Link>
        </p>
      </div>
    </div>
  );
}
