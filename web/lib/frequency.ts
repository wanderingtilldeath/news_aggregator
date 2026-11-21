// How often the edition is mailed. The values are the ones the API accepts; weekly goes out on Sundays.
export const FREQUENCIES = [
  ["daily", "Every morning", "The day's top stories, in your inbox each morning."],
  ["weekly", "Once a week, on Sunday", "The week's top stories in one email."],
] as const;

export type Frequency = (typeof FREQUENCIES)[number][0];

export function asFrequency(v: unknown): Frequency | undefined {
  return v === "daily" || v === "weekly" ? v : undefined;
}
