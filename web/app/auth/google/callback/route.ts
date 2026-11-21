import { timingSafeEqual } from "node:crypto";
import { cookies } from "next/headers";
import { NextResponse } from "next/server";
import { API_BASE } from "@/lib/api";
import { SITE_URL } from "@/lib/site";
import { FLASH_CODES_COOKIE, USER_COOKIE, userCookie } from "@/lib/user-session";

const same = (a: string, b: string) => a.length === b.length && timingSafeEqual(Buffer.from(a), Buffer.from(b));

// Sign in with Google, step two: Google sends the reader back with a code. After checking the state against the cookie
// from step one, the code goes to the API, which trades it for the reader's identity. For a login or a new account the
// answer is a session (kept in the account cookie); recovery codes made for a new account wait in a short-lived cookie
// for the account page to show once. For ?link=1 the reader's own session goes with the code and Google is tied to
// that account. Any failure goes back to the login page (or the account page, when linking) with a short flag.
export async function GET(req: Request) {
  const params = new URL(req.url).searchParams;
  const jar = await cookies();
  const state = jar.get("pp-g-state")?.value ?? "";
  const verifier = jar.get("pp-g-verifier")?.value ?? "";
  const link = jar.get("pp-g-link")?.value === "1";
  for (const name of ["pp-g-state", "pp-g-verifier", "pp-g-link"]) jar.delete(name);

  const back = (flag: string) => NextResponse.redirect(`${SITE_URL}${link ? "/account" : "/login"}?google=${flag}`);
  const code = params.get("code") ?? "";
  if (params.get("error") || !code || !state || !same(params.get("state") ?? "", state)) return back("failed");

  const session = jar.get(USER_COOKIE)?.value;
  if (link && !session) return back("login-first");

  let res: Response;
  try {
    res = await fetch(`${API_BASE}/account/google`, {
      method: "POST",
      headers: { "Content-Type": "application/json", ...(link && session ? { Authorization: `Bearer ${session}` } : {}) },
      body: JSON.stringify({ code, redirect_uri: `${SITE_URL}/auth/google/callback`, code_verifier: verifier }),
      cache: "no-store",
      signal: AbortSignal.timeout(15_000),
    });
  } catch {
    return back("failed");
  }
  if (res.status === 409) return back("taken");
  if (!res.ok) return back("failed");

  if (link) return NextResponse.redirect(`${SITE_URL}/account?google=linked`);

  const body = (await res.json().catch(() => ({}))) as { session?: string; expires?: string; recovery_codes?: string[] };
  if (!body.session) return back("failed");
  jar.set(USER_COOKIE, body.session, userCookie(new Date(body.expires ?? Date.now() + 30 * 864e5)));
  if (body.recovery_codes?.length) {
    jar.set(FLASH_CODES_COOKIE, JSON.stringify(body.recovery_codes), { httpOnly: true, secure: process.env.NODE_ENV === "production", sameSite: "lax", path: "/", maxAge: 3600 });
  }

  return NextResponse.redirect(`${SITE_URL}/account`);
}
