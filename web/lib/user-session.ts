// The reader's account session cookie: the name and the options both the server actions and the Google route handlers
// use, so they cannot drift apart. httpOnly, so a script on the page cannot read it, and never in a URL.
export const USER_COOKIE = "pp-user";

export const userCookie = (expires: Date) =>
  ({
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax" as const,
    path: "/",
    expires,
  }) as const;

// Recovery codes made by a Google sign-in cannot be shown on the redirect that follows it, so they wait in this
// cookie (an hour at most) until the account page has shown them and the reader has said they are saved.
export const FLASH_CODES_COOKIE = "pp-flash-codes";
