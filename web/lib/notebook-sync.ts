// Keeping the notepad in step with the server for a logged-in account that holds a Pro membership. The browser's
// copy (localStorage) is what the page shows; such an account's changes also go to the server, and opening the
// notepad pulls the server's list down, so the notepad follows the reader between devices. The session is a cookie
// the server actions read, so nothing here handles a secret. A change that cannot be sent is marked, and the next
// sync uploads what the server is missing before it pulls.
import { proNoteDelete, proNoteSave, proNotesImport, proNotesLoad, type ProResult } from "@/app/actions";
import { readLocal, writeLocal } from "@/lib/local";
import { NOTEBOOK_KEY, PRO_DIRTY_KEY, PRO_SYNCED_KEY, parseNotes, type Note } from "@/lib/notebook";

// A sentence the reader can read, for why something was not synced.
function reason(res: Extract<ProResult<unknown>, { ok: false }>): string {
  if (res.status === 401) return "you are logged out";
  if (res.status === 403 || res.status === 409 || res.status === 400) return res.message;
  return "the server could not be reached";
}

// pushSave sends one new or edited note; it returns "" on success, else why not. A failure marks the notepad so the
// next sync uploads it.
export async function pushSave(note: Note): Promise<string> {
  const res = await proNoteSave(note);
  if (res.ok) return "";
  writeLocal(PRO_DIRTY_KEY, "1");
  return reason(res);
}

// pushDelete removes one note on the server (an empty id removes them all). A note already gone is fine.
export async function pushDelete(id: string): Promise<string> {
  const res = await proNoteDelete(id);
  if (res.ok || res.status === 404) return "";
  return reason(res);
}

// syncNotebook uploads what this browser holds and the server lacks (the first time under this account, or after a
// failed push), then replaces the browser's copy with the server's list. It returns "" on success, else why not.
export async function syncNotebook(account: string): Promise<string> {
  if (readLocal(PRO_SYNCED_KEY) !== account || readLocal(PRO_DIRTY_KEY) === "1") {
    const local = parseNotes(readLocal(NOTEBOOK_KEY));
    for (let i = 0; i < local.length; i += 400) {
      const res = await proNotesImport(local.slice(i, i + 400));
      if (!res.ok) return reason(res);
    }
  }
  const res = await proNotesLoad();
  if (!res.ok) return reason(res);
  writeLocal(NOTEBOOK_KEY, JSON.stringify(res.data.notes));
  writeLocal(PRO_SYNCED_KEY, account);
  writeLocal(PRO_DIRTY_KEY, "");
  return "";
}

// pullNotebook is the read-only version for an account whose Pro period has ended: it adds the server's notes to this
// browser's, the server's copy winning, and uploads nothing.
export async function pullNotebook(): Promise<string> {
  const res = await proNotesLoad();
  if (!res.ok) return reason(res);
  const remote = new Set(res.data.notes.map((n) => n.id));
  const merged = [...res.data.notes, ...parseNotes(readLocal(NOTEBOOK_KEY)).filter((n) => !remote.has(n.id))];
  writeLocal(NOTEBOOK_KEY, JSON.stringify(merged.sort((a, b) => b.at - a.at)));
  return "";
}
