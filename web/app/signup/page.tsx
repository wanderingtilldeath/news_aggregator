import type { Metadata } from "next";
import Link from "next/link";
import { GoogleButton, SignUpForm } from "@/components/account-forms";

export const metadata: Metadata = { title: "Sign up · Postpaper", description: "Make a free Postpaper account with a username and a password. No email needed.", robots: { index: false } };

export default function Page() {
  return (
    <main className="narrow login" id="view">
      <h1>Sign up</h1>
      <p>A username and a password. No email is needed, and nothing is sent to you. You get ten recovery codes to keep.</p>
      <SignUpForm />
      {process.env.GOOGLE_CLIENT_ID && (
        <p className="auth-google">
          <GoogleButton />
        </p>
      )}
      <div className="login-foot">
        <span>
          Have an account? <Link href="/login">Log in</Link>
        </span>
      </div>
    </main>
  );
}
