// Keeping the notebook in step with the server for a Pro member. The browser's copy (localStorage) is what the page
// shows; a Pro member's changes also go to the server, and opening the notebook pulls the server's list down, so
// the notebook follows the member between devices. A change that cannot be sent is marked, and the next sync
// uploads what the server is missing before it pulls.
import { proNoteDelete, proNoteSave, proNotesImport, proNotesLoad, type ProResult } from "@/app/actions";
import { readLocal, writeLocal } from "@/lib/local";
import { NOTEBOOK_KEY, PRO_DIRTY_KEY, PRO_SYNCED_KEY, parseNotes, type Note } from "@/lib/notebook";

// A sentence the member can read, for why something was not synced.
function reason(res: Extract<ProResult<unknown>, { ok: false }>): string {
  if (res.status === 403) return "your Pro period has ended";
  if (res.status === 401) return "your Pro link is no longer valid";
  if (res.status === 409 || res.status === 400) return res.message;
  return "the server could not be reached";
}

const marker = (token: string) => token.slice(0, 16);

// pushSave sends one new or edited note; it returns "" on success, else why not. A failure marks the notebook so the
// next sync uploads it.
export async function pushSave(token: string, note: Note): Promise<string> {
  const res = await proNoteSave(token, note);
  if (res.ok) return "";
  writeLocal(PRO_DIRTY_KEY, "1");
  return reason(res);
}

// pushDelete removes one note on the server (an empty id removes them all). A note already gone is fine.
export async function pushDelete(token: string, id: string): Promise<string> {
  const res = await proNoteDelete(token, id);
  if (res.ok || res.status === 404) return "";
  return reason(res);
}

// syncNotebook uploads what this browser holds and the server lacks (the first time under this token, or after a
// failed push), then replaces the browser's copy with the server's list. It returns "" on success, else why not.
export async function syncNotebook(token: string): Promise<string> {
  if (readLocal(PRO_SYNCED_KEY) !== marker(token) || readLocal(PRO_DIRTY_KEY) === "1") {
    const local = parseNotes(readLocal(NOTEBOOK_KEY));
    for (let i = 0; i < local.length; i += 400) {
      const res = await proNotesImport(token, local.slice(i, i + 400));
      if (!res.ok) return reason(res);
    }
  }
  const res = await proNotesLoad(token);
  if (!res.ok) return reason(res);
  writeLocal(NOTEBOOK_KEY, JSON.stringify(res.data.notes));
  writeLocal(PRO_SYNCED_KEY, marker(token));
  writeLocal(PRO_DIRTY_KEY, "");
  return "";
}

// pullNotebook is the read-only version for a member whose period has ended: it adds the server's notes to this
// browser's, the server's copy winning, and uploads nothing.
export async function pullNotebook(token: string): Promise<string> {
  const res = await proNotesLoad(token);
  if (!res.ok) return reason(res);
  const remote = new Set(res.data.notes.map((n) => n.id));
  const merged = [...res.data.notes, ...parseNotes(readLocal(NOTEBOOK_KEY)).filter((n) => !remote.has(n.id))];
  writeLocal(NOTEBOOK_KEY, JSON.stringify(merged.sort((a, b) => b.at - a.at)));
  return "";
}
