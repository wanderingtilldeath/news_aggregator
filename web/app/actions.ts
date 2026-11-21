"use server";

import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { API_BASE, type Analytics, type ContentData, type SubscriberList } from "@/lib/api";

export type FormState = { ok: boolean; message: string } | null;

const FAILED = "Something went wrong on our side. Please try again in a little while.";

// Server actions run on Vercel and call the API there, so the browser never talks to the
// API and no CORS is needed. API error text is shown only for 4xx; anything else is generic.
async function post(path: string, body: unknown): Promise<{ status: number; error?: string } | null> {
  try {
    const res = await fetch(API_BASE + path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
      cache: "no-store",
      signal: AbortSignal.timeout(10_000),
    });
    const json = await res.json().catch(() => ({}));
    return { status: res.status, error: typeof json.error === "string" ? json.error : undefined };
  } catch {
    return null;
  }
}

export async function subscribe(_prev: FormState, form: FormData): Promise<FormState> {
  // Hidden field a person never fills; a bot that does gets a fake success.
  if (String(form.get("website") ?? "") !== "") return { ok: true, message: "Check your inbox to confirm your address." };

  const res = await post("/subscribe", {
    email: String(form.get("email") ?? ""),
    // No box ticked means every edition; the API fills in the default.
    tracks: form.getAll("tracks").map(String),
    mode: String(form.get("mode") ?? ""),
    // Comma separated words to follow; the API cleans and caps them.
    topics: String(form.get("topics") ?? "").split(","),
    // The campaign tag of the link the reader arrived by (see components/source-field.tsx); the API cleans it.
    source: String(form.get("source") ?? ""),
  });
  if (!res) return { ok: false, message: FAILED };
  if (res.status === 202) return { ok: true, message: "Check your inbox to confirm your address." };
  if (res.status === 400 || res.status === 429) return { ok: false, message: res.error ?? FAILED };
  return { ok: false, message: FAILED };
}

// The manage page: change the reading length and the followed topics.
export async function saveSubscription(_prev: FormState, form: FormData): Promise<FormState> {
  const res = await post("/subscription", {
    token: String(form.get("token") ?? ""),
    mode: String(form.get("mode") ?? ""),
    topics: String(form.get("topics") ?? "").split(","),
  });
  if (!res) return { ok: false, message: FAILED };
  if (res.status === 200) return { ok: true, message: "Saved. The next briefing follows your new choices." };
  if (res.status === 404) return { ok: false, message: "This link is not valid." };
  if (res.status === 400 || res.status === 409) return { ok: false, message: res.error ?? FAILED };
  return { ok: false, message: FAILED };
}

// Admin sign-in. An admin gives an address from ADMIN_EMAILS and the shared ADMIN_PASSWORD; the API answers with a
// session, which is kept in an httpOnly cookie, so the browser's scripts never see it and it is never in a URL.
const ADMIN_COOKIE = "pp-admin";

export async function adminSignIn(_prev: FormState, form: FormData): Promise<FormState> {
  let session: { session: string; expires: string } | null = null;
  try {
    const res = await fetch(API_BASE + "/admin/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email: String(form.get("email") ?? "").trim(), password: String(form.get("password") ?? "") }),
      cache: "no-store",
      signal: AbortSignal.timeout(10_000),
    });
    if (res.status === 401) return { ok: false, message: "That email or password is not right." };
    if (res.status === 404 || res.status === 405) return { ok: false, message: "Admin sign-in is not switched on: set ADMIN_EMAILS and ADMIN_PASSWORD (12 characters or more) on the API." };
    if (res.status === 400 || res.status === 429) return { ok: false, message: ((await res.json().catch(() => ({}))) as { error?: string }).error ?? FAILED };
    if (!res.ok) return { ok: false, message: FAILED };
    session = await res.json();
  } catch {
    return { ok: false, message: FAILED };
  }
  if (!session?.session) return { ok: false, message: FAILED };

  (await cookies()).set(ADMIN_COOKIE, session.session, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path: "/",
    expires: new Date(session.expires),
  });
  redirect("/admin");
}

export async function adminSignOut() {
  const jar = await cookies();
  const session = jar.get(ADMIN_COOKIE)?.value;
  if (session) {
    await fetch(API_BASE + "/admin/logout", { method: "POST", headers: { Authorization: `Bearer ${session}` }, cache: "no-store", signal: AbortSignal.timeout(10_000) }).catch(() => null);
    jar.delete(ADMIN_COOKIE);
  }
  redirect("/admin");
}

// What an admin read returns. signedOut is true when there is no session or the API refused it (expired, or the
// address was taken off the admin list), so the page shows the sign-in form again.
export type AdminResult<T> = { ok: true; data: T } | { ok: false; signedOut: boolean; message: string };

async function adminFetch<T>(path: string, method = "GET"): Promise<AdminResult<T>> {
  const session = (await cookies()).get(ADMIN_COOKIE)?.value;
  if (!session) return { ok: false, signedOut: true, message: "" };
  try {
    const res = await fetch(API_BASE + path, { method, headers: { Authorization: `Bearer ${session}` }, cache: "no-store", signal: AbortSignal.timeout(15_000) });
    if (res.status === 401 || res.status === 403) return { ok: false, signedOut: true, message: "Your session has ended. Sign in again." };
    if (res.status === 404 && method === "GET") return { ok: false, signedOut: true, message: "The dashboard is not switched on: set ADMIN_EMAILS and ADMIN_PASSWORD on the API." };
    if (!res.ok) return { ok: false, signedOut: false, message: ((await res.json().catch(() => ({}))) as { error?: string }).error ?? FAILED };
    return { ok: true, data: (res.status === 204 ? null : await res.json()) as T };
  } catch {
    return { ok: false, signedOut: false, message: FAILED };
  }
}

export async function loadAnalytics(days: number) {
  return adminFetch<Analytics>(`/analytics?days=${days}`);
}

export async function loadContent(days: number) {
  return adminFetch<ContentData>(`/admin/content?days=${days}`);
}

export async function loadSubscribers(q: string, page: number) {
  return adminFetch<SubscriberList>(`/admin/subscribers?q=${encodeURIComponent(q)}&page=${page}`);
}

export async function removeSubscriber(id: number) {
  return adminFetch<null>(`/admin/subscribers/${id}`, "DELETE");
}

export async function confirm(_prev: FormState, form: FormData): Promise<FormState> {
  const res = await post("/confirm", { token: String(form.get("token") ?? "") });
  if (!res) return { ok: false, message: FAILED };
  if (res.status === 200) return { ok: true, message: "Confirmed. You will get Postpaper by email." };
  if (res.status === 404 || res.status === 400) return { ok: false, message: "This confirmation link is not valid. Sign up again to get a new one." };
  return { ok: false, message: FAILED };
}

export async function unsubscribe(_prev: FormState, form: FormData): Promise<FormState> {
  const res = await post("/unsubscribe", { token: String(form.get("token") ?? "") });
  if (!res) return { ok: false, message: FAILED };
  if (res.status === 200) return { ok: true, message: "You are unsubscribed. No more emails will be sent." };
  if (res.status === 404 || res.status === 400) return { ok: false, message: "This unsubscribe link is not valid." };
  return { ok: false, message: FAILED };
}

// Postpaper Teams: connect a Slack or Discord channel to a daily edition, or stop its posts.
export async function joinTeam(_prev: FormState, form: FormData): Promise<FormState> {
  const res = await post("/teams", { url: String(form.get("url") ?? "").trim(), track: String(form.get("track") ?? "") });
  if (!res) return { ok: false, message: FAILED };
  if (res.status === 202) return { ok: true, message: "Connected. A test message was posted to the channel, and the first edition arrives with tomorrow's morning run." };
  if (res.status === 404 || res.status === 405) return { ok: false, message: "Postpaper Teams is not switched on yet." };
  if (res.status === 400 || res.status === 429) return { ok: false, message: res.error ?? FAILED };
  return { ok: false, message: FAILED };
}

export async function stopTeam(_prev: FormState, form: FormData): Promise<FormState> {
  const res = await post("/teams/stop", { token: String(form.get("token") ?? "") });
  if (!res) return { ok: false, message: FAILED };
  if (res.status === 200) return { ok: true, message: "Stopped. This channel will get no more posts." };
  if (res.status === 404 || res.status === 400) return { ok: false, message: "This link is not valid, or the posts already stopped." };
  return { ok: false, message: FAILED };
}
