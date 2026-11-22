import type { Metadata } from "next";
import { connection } from "next/server";
import { Card, Meter } from "@/components/account-ui";
import { ChangePasswordForm, DeleteAccountForm, EmailForm, GoogleButton, LogOutEverywhereButton, NewCodesForm } from "@/components/account-forms";
import { getAccount } from "@/lib/account-data";

export const metadata: Metadata = { title: "Sign-in and security · Postpaper", robots: { index: false, follow: false } };

const GOOGLE_MESSAGES: Record<string, { ok: boolean; text: string }> = {
  linked: { ok: true, text: "Google is linked. You can now log in with it, and it is a way back in if you lose your password and recovery codes." },
  failed: { ok: false, text: "Google sign-in did not work. Please try again." },
  "login-first": { ok: false, text: "Log in to your account first." },
  taken: { ok: false, text: "That Google account is already linked to another Postpaper account." },
};

export default async function Page({ searchParams }: PageProps<"/account/security">) {
  await connection();
  const res = await getAccount();
  if (!res.ok) return null;
  const { email, email_verified, has_password, codes_left, identities } = res.data;
  const flag = (await searchParams).google;
  const google = typeof flag === "string" ? GOOGLE_MESSAGES[flag] : undefined;
  const googleOn = Boolean(process.env.GOOGLE_CLIENT_ID);
  const hasGoogle = identities.includes("google");

  return (
    <>
      <h1>Sign-in and security</h1>
      <p className="acct-lead">How you log in, and how you get back in if you forget.</p>

      {google && (
        <p className={google.ok ? "note ok" : "note err"} role="status">
          {google.text}
        </p>
      )}

      <Card title={has_password ? "Password" : "Set a password"} hint={has_password ? "Changing it logs out your other devices." : "You sign in with Google alone. Add a password as another way in."}>
        <ChangePasswordForm hasPassword={has_password} />
      </Card>

      <div id="codes" />
      <Card title="Recovery codes" hint="There is no email to send a reset to, so these ten one-time codes are the way back in. Each resets your password once.">
        <div className="acct-codes">
          <Meter left={codes_left} />
          <p className={codes_left <= 2 ? "acct-text warn" : "acct-text"}>
            <b>{codes_left}</b> of 10 left.{codes_left <= 2 && " Make a new set before you run out."}
          </p>
        </div>
        <NewCodesForm hasPassword={has_password} />
      </Card>

      <Card title="Email to log in with" hint="Optional. Once confirmed by a mailed link, you can type it instead of your username and use it to reset a forgotten password.">
        <EmailForm email={email} verified={email_verified} hasPassword={has_password} />
      </Card>

      {(googleOn || hasGoogle) && (
        <Card title="Google" hint={hasGoogle ? "Linked. Postpaper keeps only Google's id for you, never your email." : "Log in with one tap, and get back in if you lose your password and recovery codes. Postpaper keeps only Google's id, never your email."}>
          {hasGoogle ? (
            <p className="acct-text ok">Google is linked to this account.</p>
          ) : (
            <p className="auth-google">
              <GoogleButton link />
            </p>
          )}
        </Card>
      )}

      <Card title="Devices" hint="Log out of every browser and device where you are logged in, this one included.">
        <LogOutEverywhereButton />
      </Card>

      <Card title="Delete account" tone="danger" hint="This removes the account, its recovery codes and its Google link for good. A Pro membership is not deleted: you can link it to another account.">
        <DeleteAccountForm hasPassword={has_password} />
      </Card>
    </>
  );
}
