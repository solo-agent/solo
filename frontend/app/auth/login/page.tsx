"use client";

import { useEffect, useState } from "react";
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
import { apiClient } from "@/lib/api-client";

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
  const [signupAvailable, setSignupAvailable] = useState(true);
  const [returnTo, setReturnTo] = useState('/home');

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
    const requested = new URLSearchParams(window.location.search).get('return_to');
    if (requested && requested.startsWith('/') && !requested.startsWith('//')) setReturnTo(requested);
  }, []);

  useEffect(() => {
    if (!isLoading && isAuthenticated) {
      router.push(returnTo);
    }
  }, [isAuthenticated, isLoading, returnTo, router]);

  useEffect(() => {
    apiClient.get<{ signup_available: boolean }>('/api/v1/auth/config')
      .then((config) => setSignupAvailable(config.signup_available))
      .catch(() => undefined);
  }, []);

  async function onSubmit(data: LoginFormValues) {
    clearError();
    try {
      await login({ email: data.email, password: data.password });
      router.push(returnTo);
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
    <div className="w-full rounded-2xl border border-black bg-white p-8 shadow-brutal-lg">
      <div className="text-center mb-6">
        <div className="mb-4 inline-flex h-14 w-14 items-center justify-center rounded-2xl border border-black bg-brutal-primary shadow-brutal-sm">
          <span className="font-heading text-2xl font-bold">S</span>
        </div>
        <h1 className="mb-1 font-heading text-3xl font-bold">{t('welcomeBack')}</h1>
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
          <div className="text-right">
            <Link href="/auth/forgot-password" className="font-heading text-xs font-bold underline hover:text-brutal-primary">
              {t('forgotPassword')}
            </Link>
          </div>
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
      <div className="mt-6 border-t border-black pt-4 text-center">
        {signupAvailable ? (
        <p className="font-sans text-sm text-muted-foreground">
          {t('noAccount')}{" "}
          <Link
            href={returnTo === '/dashboard' ? '/auth/register' : `/auth/register?return_to=${encodeURIComponent(returnTo)}`}
            className="font-heading font-bold text-black underline decoration-transparent underline-offset-4 transition-colors hover:decoration-current"
          >
            {t('register')}
          </Link>
        </p>
        ) : (
          <p className="font-sans text-sm text-muted-foreground">{t('registrationClosed')}</p>
        )}
      </div>
    </div>
  );
}
