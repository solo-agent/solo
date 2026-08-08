"use client";

import { useEffect, useState } from "react";
import Link from "next/link";

import { ApiError, apiClient } from "@/lib/api-client";
import { t } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { BrutalAlert } from "@/components/ui/brutal-alert";

export default function ForgotPasswordPage() {
  const [email, setEmail] = useState('');
  const [code, setCode] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [step, setStep] = useState<'request' | 'reset' | 'done'>('request');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [resendIn, setResendIn] = useState(0);

  useEffect(() => {
    if (resendIn <= 0) return;
    const timer = window.setTimeout(() => setResendIn((value) => value - 1), 1000);
    return () => window.clearTimeout(timer);
  }, [resendIn]);

  async function requestCode(event?: React.FormEvent) {
    event?.preventDefault();
    if (!email.trim() || resendIn > 0) return;
    setBusy(true);
    setError('');
    try {
      await apiClient.post('/api/v1/auth/password/forgot', { email });
      setStep('reset');
      setResendIn(60);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('passwordResetRequestError'));
    } finally {
      setBusy(false);
    }
  }

  async function resetPassword(event: React.FormEvent) {
    event.preventDefault();
    setError('');
    if (password.length < 8) {
      setError(t('passwordMinLength'));
      return;
    }
    if (password !== confirmPassword) {
      setError(t('passwordsMismatch'));
      return;
    }
    setBusy(true);
    try {
      await apiClient.post('/api/v1/auth/password/reset', { email, code, new_password: password });
      setStep('done');
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('passwordResetError'));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="card-brutal-heavy p-8 w-full relative" style={{ transform: 'rotate(-0.8deg)' }}>
      <div className="text-center mb-6">
        <div className="inline-flex h-14 w-14 items-center justify-center bg-brutal-accent border-brutal border-black shadow-brutal mb-4">
          <span className="font-heading font-bold text-2xl text-black">↻</span>
        </div>
        <h1 className="font-heading font-bold text-3xl mb-1">{t('resetPassword')}</h1>
        <p className="font-sans text-sm text-muted-foreground">{t('resetPasswordHint')}</p>
      </div>

      {error && <div className="mb-4"><BrutalAlert variant="error">{error}</BrutalAlert></div>}

      {step === 'request' && (
        <form onSubmit={requestCode} className="space-y-4">
          <label htmlFor="reset-email" className="font-heading font-bold text-sm block">{t('email')}</label>
          <input id="reset-email" type="email" required autoComplete="email" className="input-brutal" value={email} onChange={(event) => setEmail(event.target.value)} />
          <Button type="submit" className="w-full" disabled={busy}>{busy ? t('sendingCode') : t('sendResetCode')}</Button>
        </form>
      )}

      {step === 'reset' && (
        <form onSubmit={resetPassword} className="space-y-4">
          <p className="font-sans text-sm">{t('verificationCodeSent')} <strong>{email}</strong></p>
          <div className="space-y-2">
            <label htmlFor="reset-code" className="font-heading font-bold text-sm block">{t('verificationCode')}</label>
            <input id="reset-code" inputMode="numeric" autoComplete="one-time-code" required pattern="[0-9]{6}" maxLength={6} className="input-brutal text-center font-mono text-2xl tracking-[0.35em]" value={code} onChange={(event) => setCode(event.target.value.replace(/\D/g, '').slice(0, 6))} />
          </div>
          <div className="space-y-2">
            <label htmlFor="new-password" className="font-heading font-bold text-sm block">{t('newPassword')}</label>
            <input id="new-password" type="password" required minLength={8} autoComplete="new-password" className="input-brutal" value={password} onChange={(event) => setPassword(event.target.value)} />
          </div>
          <div className="space-y-2">
            <label htmlFor="confirm-new-password" className="font-heading font-bold text-sm block">{t('confirmPassword')}</label>
            <input id="confirm-new-password" type="password" required minLength={8} autoComplete="new-password" className="input-brutal" value={confirmPassword} onChange={(event) => setConfirmPassword(event.target.value)} />
          </div>
          <Button type="submit" className="w-full" disabled={busy || code.length !== 6}>{busy ? t('resettingPassword') : t('resetPassword')}</Button>
          <button type="button" className="w-full font-heading text-sm font-bold underline disabled:opacity-50" disabled={busy || resendIn > 0} onClick={() => requestCode()}>
            {resendIn > 0 ? t('resendIn').replace('{seconds}', String(resendIn)) : t('resendCode')}
          </button>
        </form>
      )}

      {step === 'done' && (
        <div className="space-y-4">
          <BrutalAlert variant="success">{t('passwordResetSuccess')}</BrutalAlert>
          <Link href="/auth/login" className="btn-brutal inline-flex w-full items-center justify-center">{t('backToLogin')}</Link>
        </div>
      )}

      {step !== 'done' && (
        <div className="text-center mt-6 pt-4 border-t-2 border-black">
          <Link href="/auth/login" className="font-heading text-sm font-bold underline">{t('backToLogin')}</Link>
        </div>
      )}
    </div>
  );
}
