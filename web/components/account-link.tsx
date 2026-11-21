"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useRef, useState, useSyncExternalStore } from "react";
import { accountLogOut } from "@/app/actions";
import { ACCOUNT_KEY, ADMIN_HINT_KEY } from "@/lib/notebook";
import { readLocal, subscribeLocal } from "@/lib/local";
import { forgetAccount } from "./account-forms";
import { Icon, type IconName } from "./account-icons";
import { useDismiss } from "./more-menu";

const ITEMS: { href: string; label: string; icon: IconName }[] = [
  { href: "/account", label: "Your account", icon: "home" },
  { href: "/account/pro", label: "Membership", icon: "pro" },
  { href: "/account/security", label: "Sign-in and security", icon: "shield" },
];

// "Log in" in the top bar, or, once logged in, the username as a menu: the account pages, the admin dashboard for an
// admin account, and Log out. The username is remembered in this browser when the reader logs in (it is not a
// secret), so the bar needs no request to know it. The admin item shows when this browser last saw an admin account;
// that is a hint only, /admin asks for the real credentials.
export default function AccountLink() {
  const name = useSyncExternalStore(subscribeLocal, () => readLocal(ACCOUNT_KEY), () => "");
  const admin = useSyncExternalStore(subscribeLocal, () => readLocal(ADMIN_HINT_KEY) === "1", () => false);
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  useDismiss(open, setOpen, box);

  if (!name) {
    return (
      <Link href="/login" className="nav-account" title="Log in">
        Log in
      </Link>
    );
  }

  function logOut() {
    setBusy(true);
    void accountLogOut().then(() => {
      forgetAccount();
      setOpen(false);
      setBusy(false);
      router.push("/");
      router.refresh();
    });
  }

  return (
    <div className="user-menu" ref={box}>
      <button type="button" className="user-btn" aria-expanded={open} aria-controls="user-panel" aria-haspopup="menu" onClick={() => setOpen(!open)}>
        <span className="user-avatar" aria-hidden="true">
          {name.slice(0, 1)}
        </span>
        <span className="user-name">{name}</span>
        <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true">
          <path d="m1.5 3.5 3.5 3.5 3.5-3.5" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </button>
      {open && (
        <div id="user-panel" className="user-panel" role="menu">
          <p className="user-head">
            Logged in as <b>{name}</b>
          </p>
          {ITEMS.map((it) => (
            <Link key={it.href} href={it.href} role="menuitem" onClick={() => setOpen(false)}>
              <Icon name={it.icon} />
              {it.label}
            </Link>
          ))}
          {admin && (
            <Link href="/admin" role="menuitem" className="admin" onClick={() => setOpen(false)}>
              <Icon name="admin" />
              Admin dashboard
            </Link>
          )}
          <button type="button" role="menuitem" className="out" disabled={busy} onClick={logOut}>
            <Icon name="logout" />
            {busy ? "Logging out…" : "Log out"}
          </button>
        </div>
      )}
    </div>
  );
}
