import type { Metadata } from "next";
import Link from "next/link";
import { connection } from "next/server";
import { Suspense } from "react";
import { loadAccount, readFlashCodes } from "@/app/actions";
import { ChangePasswordForm, DeleteAccountForm, FlashCodes, GoogleButton, LinkProForm, LogOutButton, LogOutEverywhereButton, NewCodesForm, RememberAccount } from "@/components/account-forms";
import Loading from "@/components/loading";

export const metadata: Metadata = { title: "Your account · Postpaper", robots: { index: false, follow: false } };

const GOOGLE_MESSAGES: Record<string, { ok: boolean; text: string }> = {
  linked: { ok: true, text: "Google is linked. You can now log in with it, and it is a way back in if you lose your password and recovery codes." },
  failed: { ok: false, text: "Google sign-in did not work. Please try again." },
  "login-first": { ok: false, text: "Log in to your account first." },
  taken: { ok: false, text: "That Google account is already linked to another Postpaper account." },
};

async function Body({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  // The session cookie makes this a per-visitor page: never prerendered or cached.
  await connection();
  const res = await loadAccount();
  if (!res.ok) {
    return res.signedOut ? (
      <p>
        You are not logged in. <Link href="/login">Log in</Link> or <Link href="/signup">sign up</Link>.
      </p>
    ) : (
      <p className="note err">{res.message}</p>
    );
  }

  const { username, pro, has_password, codes_left, identities, roles } = res.data;
  const flash = await readFlashCodes();
  const flag = (await searchParams).google;
  const google = typeof flag === "string" ? GOOGLE_MESSAGES[flag] : undefined;
  const googleOn = Boolean(process.env.GOOGLE_CLIENT_ID);
  const hasGoogle = identities.includes("google");

  return (
    <>
      <RememberAccount username={username} />
      <p>
        Logged in as <b>{username}</b>
        {roles.includes("pro") && <span className="role-chip">Pro</span>}
        {roles.includes("admin") && <span className="role-chip">Admin</span>}.
      </p>
      {google && (
        <p className={google.ok ? "note ok" : "note err"} role="status">
          {google.text}
        </p>
      )}

      {flash.length > 0 && (
        <>
          <h2 className="sec-title">Save your recovery codes</h2>
          <FlashCodes codes={flash} username={username} />
        </>
      )}

      <h2 className="sec-title">Pro membership</h2>
      {pro.active && (
        <p>
          Your Pro membership is active until <b>{pro.until}</b>. Your <Link href="/notebook">Notepad</Link> syncs on every device you log in on.
        </p>
      )}
      {pro.linked && !pro.active && (
        <p>
          Your Pro membership ended on <b>{pro.until}</b>. Your notes are still yours. <Link href="/pro">Renew</Link> to sync and add notes again.
        </p>
      )}
      {!pro.linked && (
        <>
          <p>
            If you are a Pro member, link your membership once with the link from your Pro email, or with the payment id from your receipt if the email never came. Not a member? <Link href="/pro">See Postpaper Pro</Link>.
          </p>
          <LinkProForm />
        </>
      )}

      <h2 className="sec-title">Signing in and getting back in</h2>
      <p>
        <b>{codes_left}</b> of your 10 recovery codes {codes_left === 1 ? "is" : "are"} left.{" "}
        {codes_left <= 2 && <b>Make a new set before you run out. </b>}
        Each code resets your password once. They are the way back in: there is no email to send a reset to.
      </p>
      <NewCodesForm hasPassword={has_password} />

      <h3 className="auth-sub">{has_password ? "Change your password" : "Set a password"}</h3>
      <ChangePasswordForm hasPassword={has_password} />

      {googleOn && (
        <>
          <h3 className="auth-sub">Google</h3>
          {hasGoogle ? (
            <p>Google is linked to this account. It is a second way in, and a way back if you lose your password and recovery codes.</p>
          ) : (
            <>
              <p>Link a Google account to log in with one tap, and to get back in if you lose your password and recovery codes. Postpaper keeps only Google&rsquo;s id for you, never your email.</p>
              <p className="auth-google">
                <GoogleButton link />
              </p>
            </>
          )}
        </>
      )}

      {roles.includes("admin") && (
        <>
          <h2 className="sec-title">Admin</h2>
          <p>
            This account can open the <Link href="/admin">admin dashboard</Link> with the admin password.
          </p>
        </>
      )}

      <h2 className="sec-title">Logging out and leaving</h2>
      <div className="login-foot">
        <LogOutButton />
        <LogOutEverywhereButton />
      </div>
      <h3 className="auth-sub">Delete the account</h3>
      <p>This removes the account, its recovery codes and its Google link for good. A Pro membership is not deleted: you can link it to another account.</p>
      <DeleteAccountForm hasPassword={has_password} />
    </>
  );
}

export default function Page({ searchParams }: PageProps<"/account">) {
  return (
    <main className="narrow login" id="view">
      <h1>Your account</h1>
      <Suspense fallback={<Loading />}>
        <Body searchParams={searchParams} />
      </Suspense>
    </main>
  );
}
