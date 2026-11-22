import type { Metadata } from "next";
import Link from "next/link";
import { ForgotForm } from "@/components/account-forms";

export const metadata: Metadata = { title: "Forgot your password · Postpaper", robots: { index: false } };

export default function Page() {
  return (
    <main className="narrow login" id="view">
      <h1>Forgot your password?</h1>
      <p>Enter the email address of your account and we will mail you a link to choose a new password. It works for accounts whose email was confirmed.</p>
      <ForgotForm />
      <div className="login-foot">
        <span>
          No email on the account? <Link href="/recover">Use a recovery code</Link> instead.
        </span>
        <Link href="/login">Log in</Link>
      </div>
    </main>
  );
}
