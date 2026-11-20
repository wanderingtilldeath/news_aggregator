import type { Metadata, Viewport } from "next";
import Script from "next/script";
import { Inter, Montserrat, Newsreader } from "next/font/google";
import Sponsor from "@/components/sponsor";
import { SITE_URL } from "@/lib/site";
import { Masthead, SectionBar, SiteFooter } from "@/components/site";
import "./globals.css";

// Self-hosted at build time by next/font: no request to Google when a reader opens the site.
const sans = Inter({ subsets: ["latin"], variable: "--font-sans", display: "swap" });
const brand = Montserrat({ subsets: ["latin"], variable: "--font-brand", weight: ["400", "500"], display: "swap" });
const serif = Newsreader({ subsets: ["latin"], variable: "--font-serif", display: "swap", style: ["normal", "italic"] });

export const metadata: Metadata = {
  // Share-preview image addresses (the logo card) are made absolute from this.
  metadataBase: new URL(SITE_URL),
  title: { default: "Postpaper", template: "%s" },
  alternates: { types: { "application/rss+xml": "/rss.xml" } },
  description: "The finished daily newspaper: the day's stories ranked by importance, a line on why each one matters, and a short briefing you can read in five minutes. Free.",
};

// The browser's address bar follows the system theme until the reader picks one.
export const viewport: Viewport = {
  themeColor: "#121110",
};

// The page opens in dark mode (the <html> tag below says so, so it holds even without scripts). This
// runs before the first paint and only switches to light for a reader who chose it with the switch.
// It also puts back the reader's text size and column width (see components/reading-controls.tsx).
const THEME_SCRIPT = `try{var d=document.documentElement;if(localStorage.getItem("pp-theme")==="light")d.dataset.theme="light";var r=JSON.parse(localStorage.getItem("pp-read")||"{}");if(r.s)d.dataset.rsize=String(r.s);if(r.w)d.dataset.rwidth=String(r.w)}catch(e){}`;

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en" data-theme="dark" className={`${sans.variable} ${serif.variable} ${brand.variable}`} suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: THEME_SCRIPT }} />
      </head>
      <body>
        <a className="skip" href="#view">
          Skip to the content
        </a>
        <Masthead />
        <SectionBar />
        {children}
        <Sponsor />
        <SiteFooter />
        {/* Cloudflare Web Analytics: cookie-free page counts, loaded only when a token is set. */}
        {process.env.CF_ANALYTICS_TOKEN && (
          <Script src="https://static.cloudflareinsights.com/beacon.min.js" strategy="afterInteractive" data-cf-beacon={JSON.stringify({ token: process.env.CF_ANALYTICS_TOKEN })} />
        )}
      </body>
    </html>
  );
}
