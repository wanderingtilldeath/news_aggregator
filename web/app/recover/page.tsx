import type { Metadata } from "next";
import Link from "next/link";
import { RecoverForm } from "@/components/account-forms";

export const metadata: Metadata = { title: "Reset your password · Postpaper", robots: { index: false } };

export default function Page() {
  return (
    <main className="narrow login" id="view">
      <h1>Reset your password</h1>
      <p>Enter your username, one of the recovery codes you were shown when you signed up, and a new password. The code is used up, every other device is logged out, and your remaining codes still work.</p>
      <RecoverForm />
      <div className="login-foot">
        <span>
          Remembered it? <Link href="/login">Log in</Link>
        </span>
      </div>
    </main>
  );
}
