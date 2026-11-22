"use client";

import { useEffect, useSyncExternalStore } from "react";
import { needsSync, syncAccount } from "@/lib/account-sync";
import { readLocal, subscribeLocal } from "@/lib/local";
import { ACCOUNT_KEY } from "@/lib/notebook";

// Renders nothing. For a logged-in reader it syncs bookmarks, flashcard progress, the streak and missed quiz
// questions with the account: on load, a couple of seconds after any of them changes, and when the tab regains focus.
export default function SyncAccount() {
  const user = useSyncExternalStore(subscribeLocal, () => readLocal(ACCOUNT_KEY), () => "");

  useEffect(() => {
    if (!user) return;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const later = () => {
      clearTimeout(timer);
      if (needsSync(user)) timer = setTimeout(() => void syncAccount(user), 2000);
    };
    const now = () => void syncAccount(user);

    now();
    const off = subscribeLocal(later);
    window.addEventListener("focus", now);
    return () => {
      clearTimeout(timer);
      off();
      window.removeEventListener("focus", now);
    };
  }, [user]);

  return null;
}
