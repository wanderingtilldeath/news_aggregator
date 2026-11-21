import type { Metadata } from "next";
import Link from "next/link";
import ManageLinkForm from "@/components/manage-link-form";

export const metadata: Metadata = { title: "Log in · Postpaper", description: "Manage your Postpaper subscription or open your notepad.", robots: { index: false } };

export default function Page() {
  return (
    <main className="narrow login" id="view">
      <h1>Log in</h1>
      <p>Postpaper has no passwords. Here is how to get back to what is yours.</p>

      <h2 className="sec-title">Subscribers</h2>
      <p>Enter the address you subscribed with and we will email you a link. From it you can change how long your briefing is, pick topics, or unsubscribe.</p>
      <ManageLinkForm />

      <h2 className="sec-title">Pro members</h2>
      <p>Your notepad follows you to every device once you connect your Pro link.</p>
      <p className="login-links">
        <Link href="/notebook" className="btn">
          Open the Notepad
        </Link>
        <Link href="/pro">About Pro</Link>
      </p>

      <div className="login-foot">
        <span>
          New here? <Link href="/#subscribe">Sign up free</Link>
        </span>
        <Link href="/admin">Admin sign-in</Link>
      </div>
    </main>
  );
}
