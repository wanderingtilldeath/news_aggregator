"use server";

import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { API_BASE, type Analytics } from "@/lib/api";

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

// Admin sign-in. An admin address gets a one-time link by email; opening it and pressing the button trades it for a
// session, which is kept in an httpOnly cookie, so the browser's scripts never see it and it is never in a URL.
const ADMIN_COOKIE = "pp-admin";

export async function requestAdminLink(_prev: FormState, form: FormData): Promise<FormState> {
  const res = await post("/admin/login", { email: String(form.get("email") ?? "").trim() });
  if (!res) return { ok: false, message: FAILED };
  // The same answer whether or not the address is an admin: nothing here says who is.
  if (res.status === 202) return { ok: true, message: "If that address may sign in, a link is on its way. It works once and expires in 15 minutes." };
  if (res.status === 404 || res.status === 405) return { ok: false, message: "Admin sign-in is not switched on: set ADMIN_EMAILS on the API." };
  if (res.status === 400) return { ok: false, message: res.error ?? FAILED };
  return { ok: false, message: FAILED };
}

export async function adminSignIn(_prev: FormState, form: FormData): Promise<FormState> {
  let session: { session: string; expires: string } | null = null;
  try {
    const res = await fetch(API_BASE + "/admin/session", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token: String(form.get("token") ?? "") }),
      cache: "no-store",
      signal: AbortSignal.timeout(10_000),
    });
    if (res.status === 401 || res.status === 400) return { ok: false, message: "This sign-in link is not valid or has expired. Ask for a new one." };
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
  redirect("/analytics");
}

export async function adminSignOut() {
  const jar = await cookies();
  const session = jar.get(ADMIN_COOKIE)?.value;
  if (session) {
    await fetch(API_BASE + "/admin/logout", { method: "POST", headers: { Authorization: `Bearer ${session}` }, cache: "no-store", signal: AbortSignal.timeout(10_000) }).catch(() => null);
    jar.delete(ADMIN_COOKIE);
  }
  redirect("/analytics");
}

// The analytics numbers, read with the signed-in admin's session. signedOut is true when there is none or the
// API refused it (expired, or the address was taken off the admin list).
export type AnalyticsResult = { ok: true; data: Analytics } | { ok: false; signedOut: boolean; message: string };

export async function loadAnalytics(days: number): Promise<AnalyticsResult> {
  const session = (await cookies()).get(ADMIN_COOKIE)?.value;
  if (!session) return { ok: false, signedOut: true, message: "" };
  try {
    const res = await fetch(`${API_BASE}/analytics?days=${days}`, { headers: { Authorization: `Bearer ${session}` }, cache: "no-store", signal: AbortSignal.timeout(15_000) });
    if (res.status === 401 || res.status === 403) return { ok: false, signedOut: true, message: "Your session has ended. Sign in again." };
    if (res.status === 404) return { ok: false, signedOut: true, message: "Analytics is not switched on: set ADMIN_EMAILS on the API." };
    if (!res.ok) return { ok: false, signedOut: false, message: FAILED };
    return { ok: true, data: (await res.json()) as Analytics };
  } catch {
    return { ok: false, signedOut: false, message: FAILED };
  }
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
