import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import { GoogleButton, LogInForm } from "@/components/account-forms";
import ManageLinkForm from "@/components/manage-link-form";

export const metadata: Metadata = { title: "Log in · Postpaper", description: "Log in to your Postpaper account, or manage your newsletter subscription.", robots: { index: false } };

const GOOGLE_MESSAGES: Record<string, string> = {
  failed: "Google sign-in did not work. Please try again.",
  "login-first": "Log in to your account first, then link Google from your account page.",
  taken: "That Google account is already linked to another Postpaper account.",
};

// The query string only carries the result of a Google sign-in, so it is read inside Suspense and the form stays static.
async function GoogleMessage({ searchParams }: { searchParams: PageProps<"/login">["searchParams"] }) {
  const flag = (await searchParams).google;
  const message = typeof flag === "string" ? GOOGLE_MESSAGES[flag] : undefined;
  return (
    message && (
      <p className="note err" role="status">
        {message}
      </p>
    )
  );
}

export default function Page({ searchParams }: PageProps<"/login">) {
  return (
    <main className="narrow login" id="view">
      <h1>Log in</h1>
      <Suspense>
        <GoogleMessage searchParams={searchParams} />
      </Suspense>
      <LogInForm />
      {process.env.GOOGLE_CLIENT_ID && (
        <p className="auth-google">
          <GoogleButton />
        </p>
      )}
      <p className="auth-links">
        <Link href="/signup">Sign up</Link>
        <Link href="/forgot">Forgot your password?</Link>
        <Link href="/recover">Use a recovery code</Link>
      </p>

      <h2 className="sec-title">Newsletter subscribers</h2>
      <p>The daily email has no account. Enter the address you subscribed with and we will email you a link to change how long your briefing is, pick topics, or unsubscribe.</p>
      <ManageLinkForm />

      <div className="login-foot">
        <span>
          New here? <Link href="/signup">Create an account</Link> to keep a Pro notepad on every device.
        </span>
        <Link href="/admin">Admin sign-in</Link>
      </div>
    </main>
  );
}
