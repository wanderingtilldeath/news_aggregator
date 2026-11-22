import type { Metadata } from "next";
import Link from "next/link";
import { connection } from "next/server";
import { readFlashCodes } from "@/app/actions";
import { Icon, type IconName } from "@/components/account-icons";
import { Card, Meter, Stat } from "@/components/account-ui";
import { FlashCodes } from "@/components/account-forms";
import { getAccount } from "@/lib/account-data";

export const metadata: Metadata = { title: "Your account · Postpaper", robots: { index: false, follow: false } };

const GOOGLE_MESSAGES: Record<string, string> = {
  failed: "Google sign-in did not work. Please try again.",
  "login-first": "Log in to your account first.",
};

const TILES: { href: string; icon: IconName; title: string; text: string }[] = [
  { href: "/notebook", icon: "notepad", title: "Notepad", text: "The sentences and notes you saved." },
  { href: "/bookmarks", icon: "bookmark", title: "Saved stories", text: "Stories you set aside to read later." },
  { href: "/quiz", icon: "quiz", title: "Daily quiz", text: "Five questions from today's news." },
  { href: "/flashcards", icon: "cards", title: "Flashcards", text: "Revise what you have read." },
  { href: "/epaper", icon: "paper", title: "E-paper", text: "Today's edition as a printable page." },
];

export default async function Page({ searchParams }: PageProps<"/account">) {
  await connection();
  const res = await getAccount();
  if (!res.ok) return null; // the layout already shows why

  const { username, email, pro, has_password, codes_left, identities, roles } = res.data;
  const flash = await readFlashCodes();
  const flag = (await searchParams).google;
  const message = typeof flag === "string" ? GOOGLE_MESSAGES[flag] : undefined;
  const admin = roles.includes("admin");
  const hasGoogle = identities.includes("google");
  const methods = [has_password && "Password", hasGoogle && "Google", email && "Email"].filter(Boolean).join(", ");

  return (
    <>
      <h1 className="acct-title">Hello, {username}</h1>

      {message && (
        <p className="note err" role="status">
          {message}
        </p>
      )}

      {flash.length > 0 && (
        <Card title="Save your recovery codes" hint="They are shown once. Without them, a lost password cannot be reset.">
          <FlashCodes codes={flash} username={username} />
        </Card>
      )}

      {admin && (
        <section className="acct-admin">
          <div>
            <h2>Admin dashboard</h2>
            <p>Subscribers, readers, content health and email sends. You are asked for the admin password once.</p>
          </div>
          <Link className="btn primary" href="/admin">
            Open the dashboard <Icon name="arrow" />
          </Link>
        </section>
      )}

      <div className="acct-stats">
        <Stat label="Membership" value={pro.active ? "Pro" : pro.linked ? "Ended" : "Free"} tone={pro.active ? "ok" : undefined} detail={pro.active ? `Active until ${pro.until}` : pro.linked ? `Ended on ${pro.until}` : "Reading is free. Pro adds a synced notepad."}>
          <Link href="/account/pro">{pro.linked ? "Manage membership" : "Link or get Pro"}</Link>
        </Stat>
        <Stat label="Recovery codes" value={`${codes_left} of 10`} tone={codes_left <= 2 ? "warn" : undefined} detail={codes_left <= 2 ? "Make a new set before you run out." : "Each one resets your password once."}>
          <Meter left={codes_left} />
          <Link href="/account/security#codes">{codes_left <= 2 ? "Make new codes" : "View settings"}</Link>
        </Stat>
        <Stat label="Ways to sign in" value={methods || "None"} detail={hasGoogle ? "Google is a way back in if you lose your password." : "Link Google as a second way back in."}>
          <Link href="/account/security">Sign-in settings</Link>
        </Stat>
      </div>

      <h2 className="acct-h2">Jump back in</h2>
      <div className="acct-tiles">
        {TILES.map((t) => (
          <Link key={t.href} href={t.href} className="acct-tile">
            <Icon name={t.icon} size={22} />
            <b>{t.title}</b>
            <span>{t.text}</span>
          </Link>
        ))}
      </div>
    </>
  );
}
