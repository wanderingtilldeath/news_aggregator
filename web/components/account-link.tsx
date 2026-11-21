"use client";

import Link from "next/link";
import { useSyncExternalStore } from "react";
import { ACCOUNT_KEY, ADMIN_HINT_KEY } from "@/lib/notebook";
import { readLocal, subscribeLocal } from "@/lib/local";

// "Log in" in the top bar, or the username once logged in; both lead to the account. A browser that last saw an admin
// account also shows an Admin link (a hint only: /admin asks for the real credentials). The username is remembered in
// this browser when the reader logs in (it is not a secret), so the bar needs no request to know it.
export default function AccountLink() {
  const name = useSyncExternalStore(subscribeLocal, () => readLocal(ACCOUNT_KEY), () => "");
  const admin = useSyncExternalStore(subscribeLocal, () => readLocal(ADMIN_HINT_KEY) === "1", () => false);

  return (
    <>
      {name && admin && (
        <Link href="/admin" className="nav-admin" title="Admin dashboard">
          Admin
        </Link>
      )}
      <Link href={name ? "/account" : "/login"} className="nav-account" title={name ? `Your account, ${name}` : "Log in"}>
        {name || "Log in"}
      </Link>
    </>
  );
}
