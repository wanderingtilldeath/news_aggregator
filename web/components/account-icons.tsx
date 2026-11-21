// The few line icons the account area uses, drawn at 24 by 24 in the current text colour.
const PATHS = {
  home: "M3 11 12 3l9 8M5 9.5V20h14V9.5",
  pro: "M3 7l4.5 4L12 4l4.5 7L21 7l-2 12H5L3 7z",
  shield: "M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6l8-3z",
  notepad: "M6 3h12v18H6zM9 8h6M9 12h6M9 16h4",
  bookmark: "M7 3h10v18l-5-4-5 4z",
  admin: "M4 4h7v7H4zM13 4h7v7h-7zM4 13h7v7H4zM13 13h7v7h-7z",
  logout: "M10 4H5v16h5M15 8l4 4-4 4M19 12H9",
  quiz: "M9.5 9a2.5 2.5 0 1 1 3.5 2.3c-.7.4-1 1-1 1.7M12 17h.01M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z",
  cards: "M12 3 3 8l9 5 9-5-9-5zM3 13l9 5 9-5",
  paper: "M5 4h11l3 3v13H5zM8 10h8M8 14h8",
  arrow: "M5 12h14M13 6l6 6-6 6",
  check: "M5 12.5l4.5 4.5L19 7.5",
  alert: "M12 4 2.5 20h19L12 4zM12 10v4.5M12 17.5h.01",
} as const;

export type IconName = keyof typeof PATHS;

export function Icon({ name, size = 18 }: { name: IconName; size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d={PATHS[name]} />
    </svg>
  );
}
