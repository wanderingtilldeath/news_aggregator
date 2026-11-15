"use server";

import { API_BASE } from "@/lib/api";

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
  });
  if (!res) return { ok: false, message: FAILED };
  if (res.status === 202) return { ok: true, message: "Check your inbox to confirm your address." };
  if (res.status === 400 || res.status === 429) return { ok: false, message: res.error ?? FAILED };
  return { ok: false, message: FAILED };
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
