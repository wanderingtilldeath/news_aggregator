"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useSyncExternalStore } from "react";
import { NOTEBOOK_KEY, parseNotes } from "@/lib/notebook";
import { Icon } from "./account-icons";
import { readLocal, subscribeLocal } from "@/lib/local";

// The notepad icon in the top bar, on every page, with the number of saved notes beside it (it changes the moment one
// is saved or removed). It reads this browser's storage, so it shows no number until the page is up.
export default function NotepadLink({ path = "" }: { path?: string }) {
  const raw = useSyncExternalStore(subscribeLocal, () => readLocal(NOTEBOOK_KEY), () => "");
  const count = parseNotes(raw).length;
  const label = count ? `Notepad, ${count} saved ${count === 1 ? "sentence" : "sentences"}` : "Notepad";

  return (
    <Link href="/notebook" className="nav-notepad" aria-label={label} title="Notepad" aria-current={path === "/notebook" ? "page" : undefined}>
      <Icon name="notepad" size={18} />
      {count > 0 && (
        <span className="nav-badge" aria-hidden="true">
          {count > 99 ? "99+" : count}
        </span>
      )}
    </Link>
  );
}

// The same link, told which page the reader is on. Reading the path makes it dynamic, so it sits in a <Suspense>
// boundary in the bar with the unmarked link as its fallback.
export function ActiveNotepadLink() {
  return <NotepadLink path={usePathname()} />;
}
