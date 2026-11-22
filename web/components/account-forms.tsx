"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useActionState, useEffect, useState } from "react";
import { acknowledgeCodes, accountChangePassword, accountDeleteAccount, accountLogIn, accountLogOut, accountLogOutEverywhere, accountNewCodes, accountRecover, accountRequestReset, accountReset, accountResendVerify, accountSetEmail, accountSignUp, accountVerifyEmailLink, linkProMembership, type AccountState, type CodesState, type FormState } from "@/app/actions";
import { ACCOUNT_KEY, ADMIN_HINT_KEY, SYNC_KEY } from "@/lib/notebook";
import { forgetSynced } from "@/lib/account-sync";
import { writeLocal } from "@/lib/local";
import ShowPassword from "./show-password";

// What this browser remembers about who is logged in: the name for the bar, the notepad sync flag and the admin hint.
export function forgetAccount() {
  forgetSynced();
  writeLocal(ACCOUNT_KEY, "");
  writeLocal(SYNC_KEY, "");
  writeLocal(ADMIN_HINT_KEY, "");
}

// A form that ends in a session: the action has already put the cookie in place, so what is left is to remember the
// username for the bar (not a secret). Whether this account syncs is decided on the notepad page, which knows the
// membership.
function useRemember(state: AccountState) {
  useEffect(() => {
    if (state?.ok) {
      writeLocal(ACCOUNT_KEY, state.username ?? "");
      writeLocal(SYNC_KEY, "");
    }
  }, [state]);
}

function Note({ state }: { state: AccountState }) {
  return state && !state.ok && state.message ? (
    <p className="note err" role="status">
      {state.message}
    </p>
  ) : null;
}

export function LogInForm() {
  const router = useRouter();
  const [state, action, pending] = useActionState<AccountState, FormData>(accountLogIn, null);
  useRemember(state);
  useEffect(() => {
    if (state?.ok) router.push("/account");
  }, [state, router]);

  return (
    <form action={action} className="auth-form">
      <label>
        Username or email
        <input type="text" name="username" required minLength={3} maxLength={254} autoComplete="username" autoCapitalize="none" spellCheck={false} defaultValue={state?.username ?? ""} />
      </label>
      <label>
        Password
        <input type="password" data-pw name="password" required maxLength={200} autoComplete="current-password" />
      </label>
      <ShowPassword />
      <button type="submit" disabled={pending}>
        {pending ? "Logging in…" : "Log in"}
      </button>
      <Note state={state} />
    </form>
  );
}

// Signing up ends on the recovery code, which is shown once: it is the only way back in after a forgotten password.
export function SignUpForm() {
  const [state, action, pending] = useActionState<AccountState, FormData>(accountSignUp, null);
  useRemember(state);

  if (state?.ok && state.codes?.length) return <RecoveryCodes codes={state.codes} username={state.username ?? ""} />;

  return (
    <form action={action} className="auth-form">
      <label>
        Username
        <input type="text" name="username" required minLength={3} maxLength={24} pattern="[A-Za-z0-9_]+" autoComplete="username" autoCapitalize="none" spellCheck={false} defaultValue={state?.username ?? ""} />
        <small>3 to 24 letters, digits or underscores.</small>
      </label>
      <label>
        Email (optional)
        <input type="email" name="email" maxLength={254} autoComplete="email" autoCapitalize="none" spellCheck={false} />
        <small>Lets you log in with it. Nothing is ever sent to it, and it cannot reset a password.</small>
      </label>
      <label>
        Password
        <input type="password" data-pw name="password" required minLength={10} maxLength={200} autoComplete="new-password" />
        <small>At least 10 characters.</small>
      </label>
      <label>
        Password again
        <input type="password" data-pw name="password2" required minLength={10} maxLength={200} autoComplete="new-password" />
      </label>
      <ShowPassword />
      <button type="submit" disabled={pending}>
        {pending ? "Creating…" : "Create account"}
      </button>
      <Note state={state} />
    </form>
  );
}

// The ten recovery codes, with copy, download and print and a gate so they are not skipped by accident. Each resets
// the password once; they are shown once, because with no email they are the way back in after a forgotten password.
// onDone says what the Continue button does (the account page after signup, closing the panel after "new codes").
export function RecoveryCodes({ codes, username, onDone, doneLabel = "Continue" }: { codes: string[]; username?: string; onDone?: () => void; doneLabel?: string }) {
  const router = useRouter();
  const [copied, setCopied] = useState(false);
  const [saved, setSaved] = useState(false);
  const sheet = `Postpaper recovery codes${username ? ` for ${username}` : ""}\nEach code resets your password once. Keep this sheet somewhere safe.\n\n${codes.join("\n")}\n`;

  function download() {
    const url = URL.createObjectURL(new Blob([sheet], { type: "text/plain;charset=utf-8" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = "postpaper-recovery-codes.txt";
    a.click();
    URL.revokeObjectURL(url);
  }

  return (
    <div className="auth-form recovery-sheet">
      <p>
        {username ? (
          <>
            Your account <b>{username}</b> is ready.{" "}
          </>
        ) : null}
        These are your <b>{codes.length} recovery codes</b>. Each one resets your password once. There is no email to fall back on, and they are shown <b>once</b>: save them now.
      </p>
      <ol className="recovery-codes" aria-label="Your recovery codes">
        {codes.map((c) => (
          <li key={c}>{c}</li>
        ))}
      </ol>
      <div className="auth-row">
        <button
          type="button"
          onClick={() => {
            void navigator.clipboard?.writeText(codes.join("\n")).then(() => setCopied(true));
          }}
        >
          {copied ? "Copied" : "Copy"}
        </button>
        <button type="button" onClick={download}>
          Download
        </button>
        <button type="button" onClick={() => window.print()}>
          Print
        </button>
      </div>
      <label className="auth-check">
        <input type="checkbox" checked={saved} onChange={(e) => setSaved(e.target.checked)} /> I have saved them somewhere safe
      </label>
      <button type="button" disabled={!saved} onClick={() => (onDone ? onDone() : router.push("/account"))}>
        {doneLabel}
      </button>
    </div>
  );
}

// A forgotten password: the username, the recovery code and a new password. It ends on a new recovery code.
export function RecoverForm() {
  const [state, action, pending] = useActionState<AccountState, FormData>(accountRecover, null);
  useRemember(state);

  if (state?.ok) {
    return (
      <div className="auth-form">
        <p>
          Your password is reset and <b>{state.username}</b> is logged in. Every other device was logged out.
        </p>
        <p>
          You have <b>{state.left ?? 0}</b> recovery {state.left === 1 ? "code" : "codes"} left. {(state.left ?? 0) <= 2 && <>Make a new set on your account page.</>}
        </p>
        <Link className="btn primary" href="/account">
          Go to your account
        </Link>
      </div>
    );
  }

  return (
    <form action={action} className="auth-form">
      <label>
        Username
        <input type="text" name="username" required minLength={3} maxLength={24} autoComplete="username" autoCapitalize="none" spellCheck={false} defaultValue={state?.username ?? ""} />
      </label>
      <label>
        Recovery code
        <input type="text" name="recovery_code" required maxLength={40} autoComplete="off" autoCapitalize="characters" spellCheck={false} placeholder="ABCDE-FGHJK-LMNPQ-RSTUV" defaultValue={state?.code ?? ""} />
      </label>
      <label>
        New password
        <input type="password" data-pw name="new_password" required minLength={10} maxLength={200} autoComplete="new-password" />
      </label>
      <label>
        New password again
        <input type="password" data-pw name="new_password2" required minLength={10} maxLength={200} autoComplete="new-password" />
      </label>
      <ShowPassword />
      <button type="submit" disabled={pending}>
        {pending ? "Resetting…" : "Reset the password"}
      </button>
      <Note state={state} />
    </form>
  );
}

// The account page's form: paste the Pro link, or the payment id from the receipt, once to tie the membership to this
// account.
export function LinkProForm() {
  const router = useRouter();
  const [state, action, pending] = useActionState<FormState, FormData>(linkProMembership, null);
  useEffect(() => {
    if (state?.ok) router.refresh();
  }, [state, router]);

  return (
    <form action={action} className="auth-form">
      <label>
        Your Pro link or payment id
        <input type="text" name="link" required maxLength={400} autoComplete="off" spellCheck={false} placeholder="The link from your Pro email, or pay_… from your receipt" />
        <small>No Pro email? The payment id is on the Razorpay receipt and starts with pay_.</small>
      </label>
      <button type="submit" disabled={pending}>
        {pending ? "Linking…" : "Link my membership"}
      </button>
      {state && (
        <p className={state.ok ? "note ok" : "note err"} role="status">
          {state.message}
        </p>
      )}
    </form>
  );
}

export function LogOutButton() {
  const router = useRouter();
  const [busy, setBusy] = useState(false);

  return (
    <button
      type="button"
      className="auth-out"
      disabled={busy}
      onClick={() => {
        setBusy(true);
        void accountLogOut().then(() => {
          forgetAccount();
          router.push("/");
          router.refresh();
        });
      }}
    >
      {busy ? "Logging out…" : "Log out"}
    </button>
  );
}

export function AuthLinks() {
  return (
    <p className="auth-links">
      <Link href="/login">Log in</Link>
      <Link href="/signup">Sign up</Link>
      <Link href="/recover">Forgot your password?</Link>
    </p>
  );
}

// The account page's "Remember me" for the bar: a Google sign-in ends on a redirect, so the username is kept in this
// browser (it is not a secret) by the page that knows it.
export function RememberAccount({ username, admin }: { username: string; admin?: boolean }) {
  useEffect(() => {
    writeLocal(ACCOUNT_KEY, username);
    writeLocal(ADMIN_HINT_KEY, admin ? "1" : "");
  }, [username, admin]);
  return null;
}

// Recovery codes a Google sign-in made, shown once on the account page; the cookie that held them goes when the reader
// says they are saved.
export function FlashCodes({ codes, username }: { codes: string[]; username: string }) {
  const router = useRouter();
  return (
    <RecoveryCodes
      codes={codes}
      username={username}
      doneLabel="Done"
      onDone={() => {
        void acknowledgeCodes().then(() => router.refresh());
      }}
    />
  );
}

const noteOf = (state: FormState | CodesState) =>
  state && state.message ? (
    <p className={state.ok ? "note ok" : "note err"} role="status">
      {state.message}
    </p>
  ) : null;

// A forgotten password, by email: the address of the account, which must have been confirmed.
export function ForgotForm() {
  const [state, action, pending] = useActionState<FormState, FormData>(accountRequestReset, null);

  return (
    <form action={action} className="auth-form">
      <label>
        Email of your account
        <input type="email" name="email" required maxLength={254} autoComplete="email" autoCapitalize="none" spellCheck={false} placeholder="you@example.com" />
      </label>
      <button type="submit" disabled={pending}>
        {pending ? "Sending…" : "Email me a link"}
      </button>
      {noteOf(state)}
    </form>
  );
}

// The page the reset mail links to: choose a new password. It ends logged in, like the recovery form.
export function ResetForm({ token }: { token: string }) {
  const [state, action, pending] = useActionState<AccountState, FormData>(accountReset, null);
  useRemember(state);

  if (state?.ok) {
    return (
      <div className="auth-form">
        <p>
          Your password is reset and <b>{state.username}</b> is logged in. Every other device was logged out.
        </p>
        <Link className="btn primary" href="/account">
          Go to your account
        </Link>
      </div>
    );
  }

  return (
    <form action={action} className="auth-form">
      <input type="hidden" name="token" value={token} />
      <label>
        New password
        <input type="password" data-pw name="new_password" required minLength={10} maxLength={200} autoComplete="new-password" />
      </label>
      <label>
        New password again
        <input type="password" data-pw name="new_password2" required minLength={10} maxLength={200} autoComplete="new-password" />
      </label>
      <ShowPassword />
      <button type="submit" disabled={pending}>
        {pending ? "Saving…" : "Set the new password"}
      </button>
      <Note state={state} />
    </form>
  );
}

// The page the confirmation mail links to: one button, so a mail scanner opening the link does not use it up.
export function VerifyEmailButton({ token }: { token: string }) {
  const [state, run, pending] = useActionState<FormState, FormData>(() => accountVerifyEmailLink(token), null);

  if (state?.ok) {
    return (
      <div className="auth-form">
        <p>{state.message} You can now log in with it and reset your password by email.</p>
        <Link className="btn primary" href="/account/security">
          Go to your account
        </Link>
      </div>
    );
  }

  return (
    <form action={run} className="auth-form">
      <button type="submit" disabled={pending}>
        {pending ? "Confirming…" : "Confirm my email"}
      </button>
      {noteOf(state)}
    </form>
  );
}

// The account page's email: confirmed by a mailed link, then it can be typed in place of the username when logging in
// and receives password-reset links.
export function EmailForm({ email, verified, hasPassword }: { email?: string; verified?: boolean; hasPassword: boolean }) {
  const [state, action, pending] = useActionState<FormState, FormData>(accountSetEmail, null);
  const [resent, resend, sending] = useActionState<FormState, FormData>(() => accountResendVerify(), null);

  return (
    <form action={action} className="auth-form">
      <label>
        Email to log in with
        <input type="email" name="email" maxLength={254} autoComplete="email" autoCapitalize="none" spellCheck={false} defaultValue={email ?? ""} placeholder="you@example.com" />
        <small>
          {email ? (verified ? "Confirmed. " : "Not confirmed yet: we mailed a link to this address. ") : ""}Leave it empty to remove it. We only mail it a confirmation link and the password-reset links you ask for.
        </small>
      </label>
      {hasPassword && (
        <>
          <label>
            Your password
            <input type="password" data-pw name="current_password" required maxLength={200} autoComplete="current-password" />
          </label>
          <ShowPassword />
        </>
      )}
      <button type="submit" disabled={pending}>
        {pending ? "Saving…" : "Save the email"}
      </button>
      {noteOf(state)}
      {email && !verified && (
        <p>
          <button type="submit" formAction={resend} className="linklike" disabled={sending} formNoValidate>
            {sending ? "Sending…" : "Send the link again"}
          </button>{" "}
          {resent?.message}
        </p>
      )}
    </form>
  );
}

// A new password. An account that signs in with Google alone has none yet, so it is not asked for the current one.
export function ChangePasswordForm({ hasPassword }: { hasPassword: boolean }) {
  const [state, action, pending] = useActionState<FormState, FormData>(accountChangePassword, null);

  return (
    <form action={action} className="auth-form">
      {hasPassword && (
        <label>
          Current password
          <input type="password" data-pw name="current_password" required maxLength={200} autoComplete="current-password" />
        </label>
      )}
      <label>
        {hasPassword ? "New password" : "Choose a password"}
        <input type="password" data-pw name="new_password" required minLength={10} maxLength={200} autoComplete="new-password" />
        <small>At least 10 characters. Your other devices are logged out.</small>
      </label>
      <label>
        Again
        <input type="password" data-pw name="new_password2" required minLength={10} maxLength={200} autoComplete="new-password" />
      </label>
      <ShowPassword />
      <button type="submit" disabled={pending}>
        {pending ? "Saving…" : hasPassword ? "Change the password" : "Set a password"}
      </button>
      {noteOf(state)}
    </form>
  );
}

// A new set of ten recovery codes; the old ones stop working. The codes are shown once.
export function NewCodesForm({ hasPassword }: { hasPassword: boolean }) {
  const router = useRouter();
  const [state, action, pending] = useActionState<CodesState, FormData>(accountNewCodes, null);

  if (state?.ok && state.codes) return <RecoveryCodes codes={state.codes} doneLabel="Done" onDone={() => router.refresh()} />;

  return (
    <form action={action} className="auth-form">
      {hasPassword && (
        <>
          <label>
            Your password
            <input type="password" data-pw name="current_password" required maxLength={200} autoComplete="current-password" />
          </label>
          <ShowPassword />
        </>
      )}
      <button type="submit" disabled={pending}>
        {pending ? "Making codes…" : "Make ten new codes"}
      </button>
      {noteOf(state)}
    </form>
  );
}

export function LogOutEverywhereButton() {
  const router = useRouter();
  const [busy, setBusy] = useState(false);

  return (
    <button
      type="button"
      className="auth-out"
      disabled={busy}
      onClick={() => {
        setBusy(true);
        void accountLogOutEverywhere().then(() => {
          forgetAccount();
          router.push("/");
          router.refresh();
        });
      }}
    >
      {busy ? "Logging out…" : "Log out everywhere"}
    </button>
  );
}

// Deleting the account asks for the password first and says what it does and does not remove.
export function DeleteAccountForm({ hasPassword }: { hasPassword: boolean }) {
  const router = useRouter();
  const [state, action, pending] = useActionState<FormState, FormData>(accountDeleteAccount, null);
  useEffect(() => {
    if (state?.ok) {
      forgetAccount();
      router.push("/");
      router.refresh();
    }
  }, [state, router]);

  return (
    <form
      action={action}
      className="auth-form"
      onSubmit={(e) => {
        if (!window.confirm("Delete your account for good? Your Pro membership is not deleted: you can link it to another account.")) e.preventDefault();
      }}
    >
      {hasPassword && (
        <>
          <label>
            Your password
            <input type="password" data-pw name="current_password" required maxLength={200} autoComplete="current-password" />
          </label>
          <ShowPassword />
        </>
      )}
      <button type="submit" className="auth-out" disabled={pending}>
        {pending ? "Deleting…" : "Delete my account"}
      </button>
      {noteOf(state)}
    </form>
  );
}

// "Continue with Google": a plain link, because the sign-in is a redirect (see app/auth/google). link=1 ties Google to
// the account that is logged in instead of logging in.
export function GoogleButton({ link }: { link?: boolean }) {
  return (
    <a className="btn google-btn" href={link ? "/auth/google?link=1" : "/auth/google"}>
      <svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true">
        <path fill="#4285F4" d="M23.5 12.27c0-.85-.08-1.67-.22-2.45H12v4.64h6.45a5.5 5.5 0 0 1-2.39 3.62v3h3.87c2.27-2.09 3.57-5.17 3.57-8.81Z" />
        <path fill="#34A853" d="M12 24c3.24 0 5.96-1.07 7.94-2.91l-3.87-3c-1.07.72-2.45 1.15-4.07 1.15-3.13 0-5.78-2.11-6.73-4.96H1.27v3.1A12 12 0 0 0 12 24Z" />
        <path fill="#FBBC05" d="M5.27 14.28a7.2 7.2 0 0 1 0-4.56v-3.1H1.27a12 12 0 0 0 0 10.76l4-3.1Z" />
        <path fill="#EA4335" d="M12 4.75c1.76 0 3.34.61 4.59 1.8l3.43-3.43A11.5 11.5 0 0 0 12 0 12 12 0 0 0 1.27 6.62l4 3.1C6.22 6.86 8.87 4.75 12 4.75Z" />
      </svg>
      {link ? "Link your Google account" : "Continue with Google"}
    </a>
  );
}
