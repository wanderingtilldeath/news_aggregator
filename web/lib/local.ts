// A few things the reader keeps in their own browser (streak, missed quiz questions). Nothing here
// is sent anywhere. Storage can be blocked or full, so every call is allowed to fail.
const listeners = new Set<() => void>();

export const subscribeLocal = (fn: () => void) => {
  listeners.add(fn);
  return () => void listeners.delete(fn);
};

export function readLocal(key: string): string {
  try {
    return localStorage.getItem(key) ?? "";
  } catch {
    return "";
  }
}

export function writeLocal(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    // the feature just does not remember
  }
  listeners.forEach((fn) => fn());
}
