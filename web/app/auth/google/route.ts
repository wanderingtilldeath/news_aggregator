import { createHash, randomBytes } from "node:crypto";
import { cookies } from "next/headers";
import { connection, NextResponse } from "next/server";
import { SITE_URL } from "@/lib/site";

// Sign in with Google, step one: send the reader to Google with a one-time state and a PKCE challenge. The state and the
// verifier wait in short-lived httpOnly cookies (limited to /auth/google) for the callback. ?link=1 means the reader
// is logged in already and is tying Google to that account instead of logging in. Without GOOGLE_CLIENT_ID this goes
// back to the login page.
export async function GET(req: Request) {
  // Per-visitor and env-dependent: never prerendered at build time.
  await connection();
  const clientId = process.env.GOOGLE_CLIENT_ID;
  if (!clientId) return NextResponse.redirect(`${SITE_URL}/login`);

  const link = new URL(req.url).searchParams.get("link") === "1";
  const state = randomBytes(16).toString("base64url");
  const verifier = randomBytes(32).toString("base64url");
  const challenge = createHash("sha256").update(verifier).digest("base64url");

  const jar = await cookies();
  const options = { httpOnly: true, secure: process.env.NODE_ENV === "production", sameSite: "lax" as const, path: "/auth/google", maxAge: 600 };
  jar.set("pp-g-state", state, options);
  jar.set("pp-g-verifier", verifier, options);
  jar.set("pp-g-link", link ? "1" : "0", options);

  const url = new URL("https://accounts.google.com/o/oauth2/v2/auth");
  url.search = new URLSearchParams({
    client_id: clientId,
    redirect_uri: `${SITE_URL}/auth/google/callback`,
    response_type: "code",
    scope: "openid",
    state,
    code_challenge: challenge,
    code_challenge_method: "S256",
    prompt: "select_account",
  }).toString();

  return NextResponse.redirect(url);
}
