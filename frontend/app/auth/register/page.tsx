"use client";

import { useEffect, useRef, useState } from "react";
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

const registerFormSchema = z
  .object({
    displayName: z
      .string()
      .min(1, t('displayNameRequired'))
      .max(50, t('displayNameMaxLength')),
    email: z
      .string()
      .min(1, t('emailRequired'))
      .email(t('emailInvalid')),
    password: z
      .string()
      .min(1, t('passwordRequired'))
      .min(8, t('passwordMinLength')),
    confirmPassword: z.string().min(1, t('confirmPasswordRequired')),
  })
  .refine((data) => data.password === data.confirmPassword, {
    message: t('passwordsMismatch'),
    path: ["confirmPassword"],
  });

type RegisterFormValues = z.infer<typeof registerFormSchema>;

export default function RegisterPage() {
  const router = useRouter();
  const { register: authRegister, verifyRegistration, isAuthenticated, isLoading, error, clearError } = useAuth();
  const submittingRef = useRef(false);
  const [pending, setPending] = useState<RegisterFormValues | null>(null);
  const [verificationCode, setVerificationCode] = useState('');
  const [resendIn, setResendIn] = useState(0);
  const [signupAvailable, setSignupAvailable] = useState(true);
  const [returnTo, setReturnTo] = useState('/home?onboarding=1');

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<RegisterFormValues>({
    resolver: zodResolver(registerFormSchema),
    defaultValues: {
      displayName: "",
      email: "",
      password: "",
      confirmPassword: "",
    },
  });

  // If already authenticated, redirect to dashboard
  useEffect(() => {
    const requested = new URLSearchParams(window.location.search).get('return_to');
    if (requested && requested.startsWith('/') && !requested.startsWith('//')) setReturnTo(requested);
  }, []);

  useEffect(() => {
    if (!isLoading && isAuthenticated && !submittingRef.current) {
      router.push(returnTo);
    }
  }, [isAuthenticated, isLoading, returnTo, router]);

  useEffect(() => {
    apiClient.get<{ signup_available: boolean }>('/api/v1/auth/config')
      .then((config) => setSignupAvailable(config.signup_available))
      .catch(() => undefined);
  }, []);

  async function onSubmit(data: RegisterFormValues) {
    clearError();
    submittingRef.current = true;
    try {
      const localVerificationCode = await authRegister({
        email: data.email,
        password: data.password,
        display_name: data.displayName || data.email.split("@")[0],
      });
      if (localVerificationCode) {
        await verifyRegistration({ email: data.email, code: localVerificationCode });
        router.replace(returnTo);
        return;
      }
      setPending(data);
      setResendIn(60);
      submittingRef.current = false;
    } catch {
      submittingRef.current = false;
      // Error is set in auth context, displayed below
    }
  }

  useEffect(() => {
    if (resendIn <= 0) return;
    const timer = window.setTimeout(() => setResendIn((value) => value - 1), 1000);
    return () => window.clearTimeout(timer);
  }, [resendIn]);

  async function verifyEmail(event: React.FormEvent) {
    event.preventDefault();
    if (!pending || verificationCode.trim().length !== 6) return;
    clearError();
    submittingRef.current = true;
    try {
      await verifyRegistration({ email: pending.email, code: verificationCode.trim() });
      router.replace(returnTo);
    } catch {
      submittingRef.current = false;
    }
  }

  async function resendCode() {
    if (!pending || resendIn > 0) return;
    clearError();
    try {
      await authRegister({
        email: pending.email,
        password: pending.password,
        display_name: pending.displayName || pending.email.split('@')[0],
      });
      setResendIn(60);
    } catch {
      // Auth context exposes the API error.
    }
  }

  if (isLoading) {
    return (
      <div className="card-brutal p-12 w-full">
        <div className="flex flex-col items-center gap-3">
          <Spinner size="md" />
          <p className="font-sans text-sm text-muted-foreground">{t('checkingAuth')}</p>
        </div>
      </div>
    );
  }

  if (pending) {
    return (
      <div className="w-full rounded-2xl border border-black bg-white p-8 shadow-brutal-lg">
        <div className="text-center mb-6">
          <div className="mb-4 inline-flex h-14 w-14 items-center justify-center rounded-2xl border border-black bg-brutal-info-light shadow-brutal-sm">
            <span className="font-heading text-2xl font-bold">@</span>
          </div>
          <h1 className="font-heading font-bold text-3xl mb-2">{t('verifyEmail')}</h1>
          <p className="font-sans text-sm text-muted-foreground">
            {t('verificationCodeSent')} <strong className="text-black">{pending.email}</strong>
          </p>
        </div>
        <form onSubmit={verifyEmail} className="space-y-4">
          {error && <BrutalAlert variant="error">{error}</BrutalAlert>}
          <div className="space-y-2">
            <label htmlFor="verification-code" className="font-heading font-bold text-sm block">{t('verificationCode')}</label>
            <input
              id="verification-code"
              inputMode="numeric"
              autoComplete="one-time-code"
              pattern="[0-9]{6}"
              maxLength={6}
              autoFocus
              value={verificationCode}
              onChange={(event) => setVerificationCode(event.target.value.replace(/\D/g, '').slice(0, 6))}
              className="input-brutal text-center font-mono text-2xl tracking-[0.35em]"
              aria-label={t('verificationCode')}
            />
            <p className="font-sans text-xs text-muted-foreground">{t('verificationCodeExpires')}</p>
          </div>
          <Button type="submit" className="w-full" disabled={verificationCode.length !== 6}>{t('verifyAndContinue')}</Button>
          <div className="flex items-center justify-between gap-3 text-sm">
            <button type="button" className="font-heading font-bold underline" onClick={() => { clearError(); setPending(null); setVerificationCode(''); }}>{t('changeEmail')}</button>
            <button type="button" className="font-heading font-bold underline disabled:opacity-50" disabled={resendIn > 0} onClick={resendCode}>
              {resendIn > 0 ? t('resendIn').replace('{seconds}', String(resendIn)) : t('resendCode')}
            </button>
          </div>
        </form>
      </div>
    );
  }

  if (!signupAvailable) {
    return (
      <div className="w-full space-y-5 rounded-2xl border border-black bg-white p-8 shadow-brutal-lg">
        <h1 className="font-heading text-3xl font-bold">{t('registrationClosed')}</h1>
        <p className="font-sans text-sm text-muted-foreground">{t('registrationClosedHint')}</p>
        <Link href={returnTo === '/dashboard' ? '/auth/login' : `/auth/login?return_to=${encodeURIComponent(returnTo)}`} className="btn-brutal inline-flex w-full items-center justify-center">{t('backToLogin')}</Link>
      </div>
    );
  }

  return (
    <div className="w-full rounded-2xl border border-black bg-white p-8 shadow-brutal-lg">
      <div className="text-center mb-6">
        <div className="mb-4 inline-flex h-14 w-14 items-center justify-center rounded-2xl border border-black bg-brutal-primary shadow-brutal-sm">
          <span className="font-heading text-2xl font-bold">S</span>
        </div>
        <h1 className="mb-1 font-heading text-3xl font-bold">{t('createAccount')}</h1>
        <p className="font-sans text-sm text-muted-foreground mt-1">{t('registerToSolo')}</p>
      </div>

      <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
        {/* API Error */}
        {error && <BrutalAlert variant="error">{error}</BrutalAlert>}

        {/* Display Name field */}
        <div className="space-y-2">
          <label
            htmlFor="displayName"
            className="font-heading font-bold text-sm block"
          >
            {t('displayName')}
          </label>
          <input
            id="displayName"
            type="text"
            placeholder={t('displayNamePlaceholder')}
            autoComplete="name"
            disabled={isSubmitting}
            aria-invalid={!!errors.displayName}
            className={`input-brutal ${errors.displayName ? "input-error" : ""}`}
            {...register("displayName")}
          />
          {errors.displayName && (
            <p className="text-destructive text-sm" role="alert">
              {errors.displayName.message}
            </p>
          )}
        </div>

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
            placeholder={t('passwordMinLength')}
            autoComplete="new-password"
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

        {/* Confirm Password field */}
        <div className="space-y-2">
          <label
            htmlFor="confirmPassword"
            className="font-heading font-bold text-sm block"
          >
            {t('confirmPassword')}
          </label>
          <input
            id="confirmPassword"
            type="password"
            placeholder={t('enterPasswordAgain')}
            autoComplete="new-password"
            disabled={isSubmitting}
            aria-invalid={!!errors.confirmPassword}
            className={`input-brutal ${errors.confirmPassword ? "input-error" : ""}`}
            {...register("confirmPassword")}
          />
          {errors.confirmPassword && (
            <p className="text-destructive text-sm" role="alert">
              {errors.confirmPassword.message}
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
          {isSubmitting ? t('registering') : t('createAccount')}
        </Button>
      </form>

      {/* Login link */}
      <div className="mt-6 border-t border-black pt-4 text-center">
        <p className="font-sans text-sm text-muted-foreground">
          {t('hasAccount')}{" "}
          <Link
            href={returnTo === '/dashboard' ? '/auth/login' : `/auth/login?return_to=${encodeURIComponent(returnTo)}`}
            className="font-heading font-bold text-black underline decoration-transparent underline-offset-4 transition-colors hover:decoration-current"
          >
            {t('login')}
          </Link>
        </p>
      </div>
    </div>
  );
}
