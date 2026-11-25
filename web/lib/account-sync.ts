// Keeping a free account's bookmarks, flashcard progress, streak and missed quiz questions in step across devices.
// The browser's copy (localStorage) is what the pages read and write. For a logged-in account this file sends what
// changed on this device and brings back the account's copy; see account-sync-core.ts for the rules. The session is a
// cookie the server actions read, so nothing here handles a secret. A failed request is simply tried again at the
// next change or when the tab regains focus.
import { accountSyncLoad, accountSyncSave } from "@/app/actions";
import { readLocal, writeLocal } from "@/lib/local";
import { KINDS, SYNC_BASE_KEY, SYNC_USER_KEY, changed, hash, parseBase, plan, toStored, toWire, type Kind } from "@/lib/account-sync-core";

function snapshot(): Record<Kind, string> {
  return Object.fromEntries(KINDS.map((k) => [k.kind, readLocal(k.key)])) as Record<Kind, string>;
}

// Whether this device holds something the account does not (so a sync is worth starting).
export function needsSync(user: string): boolean {
  return !user ? false : readLocal(SYNC_USER_KEY) !== user || changed(snapshot(), parseBase(readLocal(SYNC_BASE_KEY))).length > 0;
}

let running = false;
let again = false;

// syncAccount runs one round (or more, when a change arrived meanwhile). Calls made while one is running are folded
// into it.
export async function syncAccount(user: string): Promise<void> {
  if (!user) return;
  if (running) {
    again = true;
    return;
  }
  running = true;
  try {
    do {
      again = false;
      await round(user);
    } while (again);
  } finally {
    running = false;
  }
}

async function round(user: string) {
  const locals = snapshot();
  const base = parseBase(readLocal(SYNC_BASE_KEY));
  const todo = plan(locals, base, user, readLocal(SYNC_USER_KEY));

  const body: Record<string, unknown> = { mode: todo.mode };
  for (const k of todo.kinds) body[k] = toWire(k, locals[k]);
  const res = todo.mode === "get" ? await accountSyncLoad() : await accountSyncSave(body as Parameters<typeof accountSyncSave>[0]);
  if (!res.ok) return;

  const next = { ...base };
  for (const { kind, key } of KINDS) {
    // An API that predates a kind does not send it: leave what this device holds.
    if (!(kind in res.data)) continue;
    const stored = toStored(kind, res.data[kind]);
    const nowRaw = readLocal(key);
    // Changed while the request was out: leave it, the next round sends it.
    if (hash(nowRaw) !== hash(locals[kind])) continue;
    if (stored !== nowRaw) {
      writeLocal(key, stored);
    }
    next[kind] = hash(stored);
  }
  try {
    localStorage.setItem(SYNC_BASE_KEY, JSON.stringify(next));
    localStorage.setItem(SYNC_USER_KEY, user);
  } catch {
    // storage blocked: the next round merges again, which is safe
  }
}

// forgetSynced clears this browser's copy of the synced items and the bookkeeping, for logging out or deleting the
// account: the items live on the account, and a shared device must not keep them.
export function forgetSynced() {
  for (const { key } of KINDS) writeLocal(key, "");
  try {
    localStorage.removeItem(SYNC_BASE_KEY);
    localStorage.removeItem(SYNC_USER_KEY);
  } catch {
    // nothing to clear
  }
}
