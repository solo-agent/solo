"use client";

import { useEffect, useRef } from "react";
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
  const { register: authRegister, isAuthenticated, isLoading, error, clearError } = useAuth();
  const submittingRef = useRef(false);

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
    if (!isLoading && isAuthenticated && !submittingRef.current) {
      router.push("/dashboard");
    }
  }, [isAuthenticated, isLoading, router]);

  async function onSubmit(data: RegisterFormValues) {
    clearError();
    submittingRef.current = true;
    try {
      const onboardingChannelId = await authRegister({
        email: data.email,
        password: data.password,
        display_name: data.displayName || data.email.split("@")[0],
      });
      if (onboardingChannelId) {
        router.push(`/dashboard?channel=${onboardingChannelId}`);
      } else {
        router.push("/dashboard");
      }
    } catch {
      submittingRef.current = false;
      // Error is set in auth context, displayed below
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

  return (
    <div
      className="card-brutal-heavy p-8 w-full relative"
      style={{ transform: 'rotate(1.2deg)' }}
    >
      {/* Sticker badge */}
      <div
        className="absolute -top-3 -left-3 h-12 w-12 rounded-full bg-brutal-info border-2 border-black shadow-brutal-sm flex items-center justify-center animate-spin-slow z-10"
      >
        <span className="font-heading font-bold text-[10px] text-black leading-none text-center">✦<br />JOIN</span>
      </div>

      {/* Logo + Title */}
      <div className="text-center mb-6">
        <div className="inline-flex h-14 w-14 items-center justify-center bg-brutal-primary border-brutal border-black shadow-brutal mb-4 animate-pulse-brutal">
          <span className="font-heading font-bold text-2xl text-black">S</span>
        </div>
        <h1
          className="font-heading font-bold text-3xl mb-1"
          style={{
            WebkitTextStroke: '1.5px #000',
            color: 'transparent',
            letterSpacing: '-0.02em',
          }}
        >
          {t('createAccount')}
        </h1>
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
      <div className="text-center mt-6 pt-4 border-t-2 border-black">
        <p className="font-sans text-sm text-muted-foreground">
          {t('hasAccount')}{" "}
          <Link
            href="/auth/login"
            className="font-heading font-bold text-black hover:text-brutal-primary transition-colors"
          >
            {t('login')}
          </Link>
        </p>
      </div>
    </div>
  );
}
