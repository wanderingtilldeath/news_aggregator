"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Icon, type IconName } from "./account-icons";

type Item = { href: string; label: string; icon: IconName };

const ACCOUNT: Item[] = [
  { href: "/account", label: "Overview", icon: "home" },
  { href: "/account/pro", label: "Membership", icon: "pro" },
  { href: "/account/newsletter", label: "Newsletter", icon: "mail" },
  { href: "/account/security", label: "Sign-in and security", icon: "shield" },
];

const SHORTCUTS: Item[] = [
  { href: "/notebook", label: "Notepad", icon: "notepad" },
  { href: "/bookmarks", label: "Saved stories", icon: "bookmark" },
  { href: "/quiz", label: "Daily quiz", icon: "quiz" },
];

// The sidebar of the account area (a scrolling row of tabs on a phone). An admin account also gets the dashboard.
export default function AccountNav({ admin }: { admin: boolean }) {
  const path = usePathname();
  const link = (it: Item, extra?: string) => (
    <Link key={it.href} href={it.href} aria-current={path === it.href ? "page" : undefined} className={extra}>
      <Icon name={it.icon} />
      {it.label}
    </Link>
  );

  return (
    <nav className="acct-nav" aria-label="Your account">
      {ACCOUNT.map((it) => link(it))}
      {admin && link({ href: "/admin", label: "Admin dashboard", icon: "admin" }, "admin")}
      <span className="acct-nav-label">Shortcuts</span>
      {SHORTCUTS.map((it) => link(it))}
    </nav>
  );
}
