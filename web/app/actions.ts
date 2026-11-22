"use server";

import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { API_BASE, type Analytics, type ContentData, type ProList, type SubscriberList } from "@/lib/api";
import { tokenFromInput, type Note } from "@/lib/notebook";
import { FLASH_CODES_COOKIE, USER_COOKIE, userCookie } from "@/lib/user-session";

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
    // "daily" or "weekly"; empty means daily.
    frequency: String(form.get("frequency") ?? ""),
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

// The Log in page: mail an active subscriber the link to manage their subscription. The answer is the same
// whether or not the address is subscribed.
export async function requestManageLink(_prev: FormState, form: FormData): Promise<FormState> {
  const res = await post("/subscription/link", { email: String(form.get("email") ?? "").trim() });
  if (!res) return { ok: false, message: FAILED };
  if (res.status === 202) return { ok: true, message: "If that address is subscribed, a link is on its way. It can take a minute to arrive." };
  if (res.status === 400 || res.status === 429) return { ok: false, message: res.error ?? FAILED };
  return { ok: false, message: FAILED };
}

// The manage page: change the reading length, how often the edition comes, and the followed topics.
export async function saveSubscription(_prev: FormState, form: FormData): Promise<FormState> {
  const res = await post("/subscription", {
    token: String(form.get("token") ?? ""),
    mode: String(form.get("mode") ?? ""),
    frequency: String(form.get("frequency") ?? ""),
    topics: String(form.get("topics") ?? "").split(","),
  });
  if (!res) return { ok: false, message: FAILED };
  if (res.status === 200) return { ok: true, message: "Saved. The next briefing follows your new choices." };
  if (res.status === 404) return { ok: false, message: "This link is not valid." };
  if (res.status === 400 || res.status === 409) return { ok: false, message: res.error ?? FAILED };
  return { ok: false, message: FAILED };
}

// The manage page: erase the subscriber, the address and everything kept with it. The link's token is the proof.
export async function deleteSubscription(_prev: FormState, form: FormData): Promise<FormState> {
  const res = await post("/subscription/delete", { token: String(form.get("token") ?? "") });
  if (!res) return { ok: false, message: FAILED };
  if (res.status === 200) return { ok: true, message: "Deleted. Your address and choices are gone, and no more email will be sent." };
  if (res.status === 404) return { ok: true, message: "Already deleted, or this link is not valid." };
  return { ok: false, message: res.error ?? FAILED };
}

// Admin sign-in. An admin first logs in to their account (ADMIN_USERNAMES lists the accounts that may be admin), then
// enters the shared ADMIN_PASSWORD; the account's session proves the first step and the API answers with an admin
// session, which is kept in an httpOnly cookie, so the browser's scripts never see it and it is never in a URL.
const ADMIN_COOKIE = "pp-admin";

export async function adminSignIn(_prev: FormState, form: FormData): Promise<FormState> {
  const account = (await cookies()).get(USER_COOKIE)?.value;
  if (!account) return { ok: false, message: "Log in to your account first." };
  let session: { session: string; expires: string } | null = null;
  try {
    const res = await fetch(API_BASE + "/admin/login", {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${account}` },
      body: JSON.stringify({ password: String(form.get("password") ?? "") }),
      cache: "no-store",
      signal: AbortSignal.timeout(10_000),
    });
    if (res.status === 401) return { ok: false, message: "That password is not right, or your account session has ended." };
    if (res.status === 404 || res.status === 405) return { ok: false, message: "Admin sign-in is not switched on: set ADMIN_USERNAMES and ADMIN_PASSWORD (12 characters or more) on the API." };
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

async function adminFetch<T>(path: string, method = "GET", body?: unknown): Promise<AdminResult<T>> {
  const session = (await cookies()).get(ADMIN_COOKIE)?.value;
  if (!session) return { ok: false, signedOut: true, message: "" };
  try {
    const headers: Record<string, string> = { Authorization: `Bearer ${session}` };
    if (body !== undefined) headers["Content-Type"] = "application/json";
    const res = await fetch(API_BASE + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body), cache: "no-store", signal: AbortSignal.timeout(15_000) });
    if (res.status === 401 || res.status === 403) return { ok: false, signedOut: true, message: "Your session has ended. Sign in again." };
    if (res.status === 404 && method === "GET") return { ok: false, signedOut: true, message: "The dashboard is not switched on: set ADMIN_USERNAMES and ADMIN_PASSWORD on the API." };
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

export async function loadPro(q: string, page: number) {
  return adminFetch<ProList>(`/admin/pro?q=${encodeURIComponent(q)}&page=${page}`);
}

// Gives an address Pro for days; paymentId (from the unmatched list) settles that payment, empty is a manual grant.
export async function grantPro(email: string, days: number, paymentId: string) {
  return adminFetch<{ granted: boolean; until: string }>("/admin/pro", "POST", { email, days, payment_id: paymentId });
}

export async function revokePro(email: string) {
  return adminFetch<null>("/admin/pro/revoke", "POST", { email });
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

// Reader accounts: a username and password, and a recovery code shown once. The session the API returns is kept in an
// httpOnly cookie, so the browser's scripts never see it and it is never in a URL.
// On a failure it hands back the username and recovery code that were typed, so the form can show them again (a form
// action clears every field when it ends). codes are the recovery codes just made (signup), shown once; left is how
// many an account has.
export type AccountState = { ok: boolean; message: string; username?: string; codes?: string[]; left?: number; code?: string } | null;

async function userSession() {
  return (await cookies()).get(USER_COOKIE)?.value;
}

type SessionBody = { session?: string; username?: string; expires?: string; recovery_codes?: string[]; codes_left?: number; error?: string };

// Sends the form's fields to an account endpoint and, when it answers with a session, keeps it in the cookie.
async function accountCall(path: string, payload: Record<string, string>): Promise<AccountState> {
  let res: Response;
  try {
    res = await fetch(API_BASE + path, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(payload), cache: "no-store", signal: AbortSignal.timeout(15_000) });
  } catch {
    return { ok: false, message: FAILED, username: payload.username, code: payload.recovery_code };
  }
  const body = (await res.json().catch(() => ({}))) as SessionBody;
  if (!res.ok) return { ok: false, message: res.status >= 500 || !body.error ? FAILED : body.error, username: payload.username, code: payload.recovery_code };
  if (!body.session || !body.username) return { ok: false, message: FAILED, username: payload.username, code: payload.recovery_code };

  (await cookies()).set(USER_COOKIE, body.session, userCookie(new Date(body.expires ?? Date.now() + 30 * 864e5)));

  return { ok: true, message: "", username: body.username, codes: body.recovery_codes, left: body.codes_left };
}

export async function accountSignUp(_prev: AccountState, form: FormData): Promise<AccountState> {
  const password = String(form.get("password") ?? "");
  if (password !== String(form.get("password2") ?? "")) return { ok: false, message: "The two passwords are not the same.", username: String(form.get("username") ?? "") };
  return accountCall("/account/signup", { username: String(form.get("username") ?? ""), email: String(form.get("email") ?? ""), password });
}

export async function accountLogIn(_prev: AccountState, form: FormData): Promise<AccountState> {
  return accountCall("/account/login", { username: String(form.get("username") ?? ""), password: String(form.get("password") ?? "") });
}

export async function accountRecover(_prev: AccountState, form: FormData): Promise<AccountState> {
  const password = String(form.get("new_password") ?? "");
  if (password !== String(form.get("new_password2") ?? "")) return { ok: false, message: "The two passwords are not the same.", username: String(form.get("username") ?? ""), code: String(form.get("recovery_code") ?? "") };
  return accountCall("/account/recover", { username: String(form.get("username") ?? ""), recovery_code: String(form.get("recovery_code") ?? ""), new_password: password });
}

// Asks for a password-reset link by email. The answer never says whether the address belongs to an account.
export async function accountRequestReset(_prev: FormState, form: FormData): Promise<FormState> {
  try {
    const res = await fetch(API_BASE + "/account/reset-request", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email: String(form.get("email") ?? "").trim() }),
      cache: "no-store",
      signal: AbortSignal.timeout(15_000),
    });
    const body = (await res.json().catch(() => ({}))) as { error?: string };
    if (res.status === 202) return { ok: true, message: "If that address belongs to an account and was confirmed, a link is on its way. It works once, for an hour." };
    return { ok: false, message: res.status >= 500 || !body.error ? FAILED : body.error };
  } catch {
    return { ok: false, message: FAILED };
  }
}

// Sets the new password from the link in the reset mail; the API logs the reader in.
export async function accountReset(_prev: AccountState, form: FormData): Promise<AccountState> {
  const password = String(form.get("new_password") ?? "");
  if (password !== String(form.get("new_password2") ?? "")) return { ok: false, message: "The two passwords are not the same." };
  return accountCall("/account/reset", { token: String(form.get("token") ?? ""), new_password: password });
}

// Spends the link in the confirmation mail. The mail only links to a page with a button, so a mail scanner that opens
// the link does not use it up.
export async function accountVerifyEmailLink(token: string): Promise<FormState> {
  try {
    const res = await fetch(API_BASE + "/account/email/verify", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ token }), cache: "no-store", signal: AbortSignal.timeout(15_000) });
    if (res.status === 204) return { ok: true, message: "Your email is confirmed." };
    const body = (await res.json().catch(() => ({}))) as { error?: string };
    return { ok: false, message: res.status >= 500 || !body.error ? FAILED : body.error };
  } catch {
    return { ok: false, message: FAILED };
  }
}

// Mails a fresh confirmation link to the account's unconfirmed address.
export async function accountResendVerify(): Promise<FormState> {
  const r = await accountPost("/account/email/resend", {});
  if (!r) return { ok: false, message: (await userSession()) ? FAILED : "Log in first." };
  if (r.status !== 204) return { ok: false, message: said(r) };
  return { ok: true, message: "Sent. Check your inbox." };
}

// Ends the session on the server and forgets the cookie.
export async function accountLogOut() {
  const jar = await cookies();
  const session = jar.get(USER_COOKIE)?.value;
  if (session) {
    await fetch(API_BASE + "/account/logout", { method: "POST", headers: { Authorization: `Bearer ${session}` }, cache: "no-store", signal: AbortSignal.timeout(10_000) }).catch(() => null);
    jar.delete(USER_COOKIE);
  }
}

export type AccountInfo = {
  username: string;
  email?: string;
  email_verified: boolean;
  pro: { linked: boolean; active: boolean; until?: string; days_left: number; email?: string; edition?: string };
  has_password: boolean;
  codes_left: number;
  identities: string[];
  roles: string[];
};

// Who is logged in, and whether their Pro membership is linked and paid up. signedOut is true when there is no
// session or the API refused it (expired, or ended by a password reset).
export async function loadAccount(): Promise<{ ok: true; data: AccountInfo } | { ok: false; signedOut: boolean; message: string }> {
  const session = await userSession();
  if (!session) return { ok: false, signedOut: true, message: "" };
  try {
    const res = await fetch(API_BASE + "/account/me", { headers: { Authorization: `Bearer ${session}` }, cache: "no-store", signal: AbortSignal.timeout(15_000) });
    if (res.status === 401) return { ok: false, signedOut: true, message: "" };
    if (!res.ok) return { ok: false, signedOut: false, message: FAILED };
    return { ok: true, data: (await res.json()) as AccountInfo };
  } catch {
    return { ok: false, signedOut: false, message: FAILED };
  }
}

// Calls an account endpoint with the logged-in session. null when nobody is logged in or the API is unreachable.
async function accountPost(path: string, payload: Record<string, unknown>): Promise<{ status: number; body: Record<string, unknown> } | null> {
  const session = await userSession();
  if (!session) return null;
  try {
    const res = await fetch(API_BASE + path, {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${session}` },
      body: JSON.stringify(payload),
      cache: "no-store",
      signal: AbortSignal.timeout(15_000),
    });
    return { status: res.status, body: res.status === 204 ? {} : ((await res.json().catch(() => ({}))) as Record<string, unknown>) };
  } catch {
    return null;
  }
}

// The text of an API answer for the reader: the API's own words for a refusal, a plain failure for anything else.
const said = (r: { status: number; body: Record<string, unknown> }) => (r.status >= 500 || typeof r.body.error !== "string" ? FAILED : (r.body.error as string));

// Ties a Pro membership to the logged-in account, on proof the reader holds it: the Pro link from the Pro email, or the
// payment id from the Razorpay receipt (for a member whose Pro mail never arrived).
export async function linkProMembership(_prev: FormState, form: FormData): Promise<FormState> {
  const text = String(form.get("link") ?? "").trim();
  const token = tokenFromInput(text);
  const payload = /^pay_[A-Za-z0-9]{6,40}$/.test(text) ? { payment_id: text } : token ? { token } : null;
  if (!payload) return { ok: false, message: "That is not a Pro link or a payment id. Paste the link from your Pro email, or the payment id (it starts with pay_) from your receipt." };
  const r = await accountPost("/account/pro", payload);
  if (!r) return { ok: false, message: (await userSession()) ? FAILED : "Log in first." };
  if (r.status === 200) return { ok: true, message: "Linked. Your notepad now syncs on every device you log in on." };
  return { ok: false, message: said(r) };
}

// A new password, which ends every session; the API answers with a new one, which replaces the cookie.
export async function accountChangePassword(_prev: FormState, form: FormData): Promise<FormState> {
  const next = String(form.get("new_password") ?? "");
  if (next !== String(form.get("new_password2") ?? "")) return { ok: false, message: "The two new passwords are not the same." };
  const r = await accountPost("/account/password", { current_password: String(form.get("current_password") ?? ""), new_password: next });
  if (!r) return { ok: false, message: (await userSession()) ? FAILED : "Log in first." };
  if (r.status !== 200 || typeof r.body.session !== "string") return { ok: false, message: said(r) };
  (await cookies()).set(USER_COOKIE, r.body.session, userCookie(new Date(String(r.body.expires ?? Date.now() + 30 * 864e5))));
  return { ok: true, message: "Password changed. Your other devices are logged out." };
}

// Sets, changes or clears (empty) the email that can be used to log in; it asks for the password.
export async function accountSetEmail(_prev: FormState, form: FormData): Promise<FormState> {
  const email = String(form.get("email") ?? "").trim();
  const r = await accountPost("/account/email", { email, current_password: String(form.get("current_password") ?? "") });
  if (!r) return { ok: false, message: (await userSession()) ? FAILED : "Log in first." };
  if (r.status !== 204) return { ok: false, message: said(r) };
  return { ok: true, message: email ? "Saved. We sent a link to this address: open it to confirm, and you can then log in with it and reset your password by email." : "Email removed." };
}

export type CodesState = { ok: boolean; message: string; codes?: string[] } | null;

// A new set of ten recovery codes, shown once; the old ones stop working.
export async function accountNewCodes(_prev: CodesState, form: FormData): Promise<CodesState> {
  const r = await accountPost("/account/codes", { current_password: String(form.get("current_password") ?? "") });
  if (!r) return { ok: false, message: (await userSession()) ? FAILED : "Log in first." };
  if (r.status !== 200 || !Array.isArray(r.body.recovery_codes)) return { ok: false, message: said(r) };
  return { ok: true, message: "", codes: r.body.recovery_codes as string[] };
}

// Ends every session of the account, this one included.
export async function accountLogOutEverywhere() {
  await accountPost("/account/logout-all", {});
  (await cookies()).delete(USER_COOKIE);
}

// Deletes the account for good (after the password is asked for); a Pro membership can be linked again elsewhere.
export async function accountDeleteAccount(_prev: FormState, form: FormData): Promise<FormState> {
  const r = await accountPost("/account/delete", { current_password: String(form.get("current_password") ?? "") });
  if (!r) return { ok: false, message: (await userSession()) ? FAILED : "Log in first." };
  if (r.status !== 204) return { ok: false, message: said(r) };
  (await cookies()).delete(USER_COOKIE);
  return { ok: true, message: "Your account is deleted." };
}

// The recovery codes a Google sign-in made, waiting to be shown (see lib/user-session.ts).
export async function readFlashCodes(): Promise<string[]> {
  try {
    const parsed: unknown = JSON.parse((await cookies()).get(FLASH_CODES_COOKIE)?.value ?? "[]");
    return Array.isArray(parsed) ? parsed.filter((c): c is string => typeof c === "string").slice(0, 20) : [];
  } catch {
    return [];
  }
}

export async function acknowledgeCodes() {
  (await cookies()).delete(FLASH_CODES_COOKIE);
}

// The Pro notepad, read and written with the logged-in account's session (its Pro membership gives it the notes).
// status is the HTTP status, 0 when the API could not be reached.
export type ProResult<T> = { ok: true; data: T } | { ok: false; status: number; message: string };

async function proFetch<T>(path: string, method: string, body?: unknown): Promise<ProResult<T>> {
  const session = await userSession();
  if (!session) return { ok: false, status: 401, message: "Log in first." };
  try {
    const res = await fetch(API_BASE + path, {
      method,
      headers: { Authorization: `Bearer ${session}`, ...(body === undefined ? {} : { "Content-Type": "application/json" }) },
      body: body === undefined ? undefined : JSON.stringify(body),
      cache: "no-store",
      signal: AbortSignal.timeout(15_000),
    });
    const json = res.status === 204 ? null : await res.json().catch(() => null);
    if (!res.ok) return { ok: false, status: res.status, message: typeof json?.error === "string" ? json.error : FAILED };
    return { ok: true, data: json as T };
  } catch {
    return { ok: false, status: 0, message: FAILED };
  }
}

const wire = (n: Note) => ({ id: n.id, story: n.story, title: n.title.slice(0, 300), text: n.text.slice(0, 2000), note: n.note ?? "", tags: n.tags ?? [], at: n.at });

// What a free account carries between devices (bookmarks, flashcard progress, streak, missed quiz questions).
export type SyncState = { bookmarks: unknown[]; cards: Record<string, unknown>; streak: string; missed: unknown[] };

export async function accountSyncLoad() {
  return proFetch<SyncState>("/account/sync", "GET");
}

// mode "merge" unions the kinds sent with the account's; "replace" makes them the account's copy.
export async function accountSyncSave(body: { mode: "merge" | "replace"; bookmarks?: unknown; cards?: unknown; streak?: unknown; missed?: unknown }) {
  return proFetch<SyncState>("/account/sync", "PUT", body);
}

export async function proNotesLoad() {
  return proFetch<{ notes: Note[]; limit: number }>("/pro/notes", "GET");
}

export async function proNoteSave(note: Note) {
  return proFetch<Note>(`/pro/notes/${encodeURIComponent(note.id)}`, "PUT", wire(note));
}

// An empty id deletes every note.
export async function proNoteDelete(id: string) {
  return proFetch<null>(id ? `/pro/notes/${encodeURIComponent(id)}` : "/pro/notes", "DELETE");
}

export async function proNotesImport(notes: Note[]) {
  return proFetch<{ added: number }>("/pro/notes/import", "POST", { notes: notes.map(wire) });
}
