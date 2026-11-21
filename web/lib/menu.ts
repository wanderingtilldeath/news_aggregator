// The links to the site's tools, in one place: the "More" menu in the sections bar and the footer
// both read them, so a new page is added once.
export type MenuLink = { href: string; label: string };
export type MenuGroup = { title: string; links: MenuLink[] };

// The "More" menu: everything the bar has no room for, in three groups.
export const MORE: MenuGroup[] = [
  {
    title: "Study",
    links: [
      { href: "/quiz", label: "Quiz" },
      { href: "/flashcards", label: "Flashcards" },
      { href: "/concepts", label: "Concepts" },
      { href: "/bookmarks", label: "Bookmarks" },
    ],
  },
  {
    title: "Read",
    links: [
      { href: "/archive", label: "Archive" },
      { href: "/week", label: "Weekly revision" },
      { href: "/month", label: "Monthly compilations" },
      { href: "/editorials", label: "Editorials" },
      { href: "/epaper", label: "E-paper" },
      { href: "/underreported", label: "Underreported" },
    ],
  },
  {
    title: "Explore",
    links: [
      { href: "/graph", label: "Who is in the news" },
      { href: "/teams", label: "Slack and Discord" },
    ],
  },
];

// The footer: every page, in three columns.
export const FOOTER: MenuGroup[] = [
  {
    title: "Read",
    links: [
      { href: "/?track=upsc", label: "UPSC edition" },
      { href: "/?track=engineering", label: "Engineering edition" },
      { href: "/archive", label: "Archive" },
      { href: "/week", label: "Weekly revision" },
      { href: "/month", label: "Monthly compilations" },
      { href: "/editorials", label: "Editorials" },
      { href: "/epaper", label: "E-paper" },
      { href: "/underreported", label: "Underreported" },
      { href: "/funding", label: "Startup funding" },
    ],
  },
  {
    title: "Study",
    links: [
      { href: "/quiz", label: "Quiz" },
      { href: "/flashcards", label: "Flashcards" },
      { href: "/games", label: "Games" },
      { href: "/bookmarks", label: "Bookmarks" },
      { href: "/notebook", label: "Notepad" },
      { href: "/concepts", label: "Concepts" },
      { href: "/graph", label: "Who is in the news" },
    ],
  },
  {
    title: "About",
    links: [
      { href: "/#subscribe", label: "Get it by email" },
      { href: "/pro", label: "Pro" },
      { href: "/teams", label: "Slack and Discord" },
      { href: "/sources", label: "Sources and takedown" },
    ],
  },
];
