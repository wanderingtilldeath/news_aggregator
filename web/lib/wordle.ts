// The daily word game. Everything here is plain functions with no imports, so it runs under plain Node
// for its tests (wordle.test.ts). The answer for a day comes from ANSWERS by date, the same for every reader.
export type Mark = "correct" | "present" | "absent";

export const WORD_LENGTH = 5;
export const MAX_GUESSES = 6;

// News words, so each answer is something a reader of the site has met. Order is mixed on purpose.
// shortcut: a guess is only checked for length and letters, not against a dictionary, and the answers sit in
// the page bundle where a curious reader can find them; upgrade when the game has real players.
export const ANSWERS = [
  "voter", "cloud", "taxes", "storm", "court", "robot", "price", "union", "judge", "chips",
  "trade", "flood", "audit", "drone", "party", "ocean", "loans", "media", "rally", "solar",
  "bonds", "virus", "mayor", "model", "stock", "crops", "trial", "token", "power", "water",
  "banks", "civic", "cyber", "fuel", "green", "labor", "order", "peace", "polls", "press",
  "rates", "seats", "study", "truce", "urban", "rural", "wages", "world", "brief", "chief",
  "claim", "daily", "focus", "funds", "goods", "issue", "local", "story", "think", "video",
  "email", "phone", "pixel", "apps", "quake", "river", "coast", "earth", "nurse", "drugs",
  "state", "bills", "laws", "print", "input", "super", "inbox", "stage", "round", "equal",
  "grant", "board", "boost", "break", "cabin", "carry", "cause", "check", "civil", "clash",
  "clean", "clear", "coach", "count", "cover", "crash", "crime", "crowd", "debut", "delay",
  "depth", "drive", "early", "elect", "enact", "enter", "entry", "error", "event", "exact",
  "exist", "extra", "faith", "fault", "field", "final", "fraud", "front", "guard", "guide",
  "happy", "heavy", "honor", "human", "hotel", "image", "index", "joint", "major", "march",
  "mount", "movie", "nation", "noise", "novel", "offer", "panel", "paper", "plant", "point",
  "proof", "queen", "raise", "reach", "reply", "rider", "rival", "route", "scale", "scene",
  "score", "sense", "serve", "share", "shift", "shock", "short", "sight", "skill", "smart",
  "solve", "sound", "space", "speed", "spend", "split", "staff", "start", "steel", "still",
  "style", "sugar", "table", "taste", "thank", "theme", "tight", "title", "total", "touch",
  "tower", "track", "train", "trend", "trust", "truth", "unity", "upper", "usage", "value",
  "visit", "vital", "voice", "waste", "watch", "wheat", "while", "whole", "youth", "zones",
].filter((w) => w.length === WORD_LENGTH);

const pad = (n: number) => String(n).padStart(2, "0");

// The day as UTC "YYYY-MM-DD", so every reader on Earth plays the same word on the same date.
export const dayKey = (d: Date) => `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())}`;

// Days since 2026-01-01 (puzzle number).
export function dayNumber(key: string): number {
  const [y, m, d] = key.split("-").map(Number);
  return Math.floor((Date.UTC(y, m - 1, d) - Date.UTC(2026, 0, 1)) / 86_400_000);
}

export const wordOfDay = (key: string) => ANSWERS[((dayNumber(key) % ANSWERS.length) + ANSWERS.length) % ANSWERS.length];

// Marks each letter of a guess. Two passes so a repeated letter is marked "present" only as many times as the
// answer has it: exact matches first, then the rest from what is left.
export function score(guess: string, answer: string): Mark[] {
  const marks: Mark[] = Array(guess.length).fill("absent");
  const left: Record<string, number> = {};
  for (let i = 0; i < guess.length; i++) {
    if (guess[i] === answer[i]) marks[i] = "correct";
    else left[answer[i]] = (left[answer[i]] ?? 0) + 1;
  }
  for (let i = 0; i < guess.length; i++) {
    if (marks[i] === "correct" || !left[guess[i]]) continue;
    marks[i] = "present";
    left[guess[i]]--;
  }
  return marks;
}

const EMOJI: Record<Mark, string> = { correct: "🟩", present: "🟨", absent: "⬛" };

// The result a reader can paste without giving the word away.
export function shareText(guesses: string[], answer: string, key: string, name = "Wordle"): string {
  const won = guesses[guesses.length - 1] === answer;
  const grid = guesses.map((g) => score(g, answer).map((m) => EMOJI[m]).join("")).join("\n");
  return `Postpaper ${name} ${dayNumber(key)} ${won ? guesses.length : "X"}/${MAX_GUESSES}\n${grid}`;
}

// Stats kept in the browser as "lastWinDay|played|won|streak". A win on the day after the last win extends the
// streak; any other win starts it at 1; a loss resets it to 0.
export function nextStats(raw: string, won: boolean, key: string): string {
  const [lastWin = 0, played = 0, wins = 0, streak = 0] = raw.split("|").map((v) => parseInt(v, 10) || 0);
  const n = dayNumber(key);
  const run = won ? (lastWin === n - 1 && lastWin !== 0 ? streak + 1 : 1) : 0;
  return `${won ? n : lastWin}|${played + 1}|${wins + (won ? 1 : 0)}|${run}`;
}

export const readStats = (raw: string) => {
  const [, played = 0, won = 0, streak = 0] = raw.split("|").map((v) => parseInt(v, 10) || 0);
  return { played, won, streak };
};
