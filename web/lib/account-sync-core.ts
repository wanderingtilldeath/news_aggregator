// The pure part of keeping a free account's bookmarks, flashcard progress, streak, missed quiz questions and map marks in step
// across devices (see account-sync.ts for the part that talks to the server). No imports, so Node tests can run it.

export type Kind = "bookmarks" | "cards" | "streak" | "missed" | "marks";

// Each kind: the localStorage key it lives in, how a stored string becomes the value sent, and how a value from the
// server becomes the string stored. The keys are the ones the pages already use.
export const KINDS: { kind: Kind; key: string }[] = [
  { kind: "bookmarks", key: "pp-bookmarks" },
  { kind: "cards", key: "pp-cards" },
  { kind: "streak", key: "pp-streak" },
  { kind: "missed", key: "pp-missed" },
  { kind: "marks", key: "pp-maps-marks" },
];

export const SYNC_USER_KEY = "pp-sync-user";
export const SYNC_BASE_KEY = "pp-sync-base";

// A short fingerprint of a stored string, to tell whether it changed since it was last in step with the server.
export function hash(s: string): string {
  let h = 5381;
  for (let i = 0; i < s.length; i++) h = ((h << 5) + h + s.charCodeAt(i)) | 0;
  return (h >>> 0).toString(36) + ":" + s.length;
}

export type Base = Partial<Record<Kind, string>>;

export function parseBase(raw: string): Base {
  try {
    const v = JSON.parse(raw || "{}");
    return v && typeof v === "object" && !Array.isArray(v) ? v : {};
  } catch {
    return {};
  }
}

// What to do for the logged-in user. A device that has not met this account yet merges everything it holds into the
// account (so nothing saved before logging in is lost). After that, the kinds changed on this device since the last
// sync replace the account's copy, and when nothing changed the device just reads the account's copy.
export function plan(locals: Record<Kind, string>, base: Base, user: string, syncedUser: string): { mode: "merge" | "replace" | "get"; kinds: Kind[] } {
  if (syncedUser !== user) return { mode: "merge", kinds: KINDS.map((k) => k.kind).filter((k) => locals[k] !== "") };
  const dirty = changed(locals, base);
  return dirty.length ? { mode: "replace", kinds: dirty } : { mode: "get", kinds: [] };
}

// The kinds whose stored string differs from what it was at the last sync.
export function changed(locals: Record<Kind, string>, base: Base): Kind[] {
  return KINDS.map((k) => k.kind).filter((k) => hash(locals[k]) !== (base[k] ?? hash("")));
}

// The value sent for a kind, from its stored string. Unreadable storage counts as empty.
export function toWire(kind: Kind, raw: string): unknown {
  if (kind === "streak") return raw;
  try {
    const v = JSON.parse(raw || (kind === "cards" ? "{}" : "[]"));
    if (kind === "cards") return v && typeof v === "object" && !Array.isArray(v) ? v : {};
    return Array.isArray(v) ? v : [];
  } catch {
    return kind === "cards" ? {} : [];
  }
}

// The string stored for a kind, from the server's value. Empty values store as "" so an untouched device stays clean.
export function toStored(kind: Kind, value: unknown): string {
  if (kind === "streak") return typeof value === "string" ? value : "";
  if (kind === "cards") return value && typeof value === "object" && Object.keys(value).length ? JSON.stringify(value) : "";
  return Array.isArray(value) && value.length ? JSON.stringify(value) : "";
}
