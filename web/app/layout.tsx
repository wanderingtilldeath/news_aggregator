import type { Metadata, Viewport } from "next";
import Script from "next/script";
import { Suspense } from "react";
import { Inter, Montserrat, Newsreader } from "next/font/google";
import Sponsor from "@/components/sponsor";
import { CaptureSource } from "@/components/source-field";
import Toaster from "@/components/toaster";
import { SITE_URL } from "@/lib/site";
import FloatingActions from "@/components/floating-actions";
import ScrollTop from "@/components/scroll-top";
import { Masthead, SectionBar, SiteFooter } from "@/components/site";
import "./globals.css";

// Self-hosted at build time by next/font: no request to Google when a reader opens the site.
const sans = Inter({ subsets: ["latin"], variable: "--font-sans", display: "swap" });
const brand = Montserrat({ subsets: ["latin"], variable: "--font-brand", weight: ["400", "500"], display: "swap" });
const serif = Newsreader({ subsets: ["latin"], variable: "--font-serif", display: "swap", style: ["normal", "italic"] });

export const metadata: Metadata = {
  // Share-preview image addresses (the logo card) are made absolute from this.
  metadataBase: new URL(SITE_URL),
  title: { default: "Postpaper: all the news, none of the noise", template: "%s" },
  // Each page names itself as the canonical address without its query string, so the filtered views of the
  // front page and tracking tags (?utm_source=) count as one page.
  alternates: { canonical: "./", types: { "application/rss+xml": "/rss.xml" } },
  description: "Postpaper ranks the day's top stories by importance and adds a plain line on why each one matters. A free daily briefing you can read in five minutes, by email or on the site.",
  openGraph: { type: "website", siteName: "Postpaper", locale: "en_IN" },
  twitter: { card: "summary_large_image" },
};

// What search engines read about the site as a whole: who publishes it, and that it has a search box.
const SITE_LD = JSON.stringify({
  "@context": "https://schema.org",
  "@graph": [
    { "@type": "Organization", "@id": `${SITE_URL}/#org`, name: "Postpaper", url: SITE_URL, logo: `${SITE_URL}/icon.svg` },
    {
      "@type": "WebSite",
      "@id": `${SITE_URL}/#site`,
      url: SITE_URL,
      name: "Postpaper",
      publisher: { "@id": `${SITE_URL}/#org` },
      inLanguage: "en-IN",
      potentialAction: { "@type": "SearchAction", target: { "@type": "EntryPoint", urlTemplate: `${SITE_URL}/?q={search_term_string}` }, "query-input": "required name=search_term_string" },
    },
  ],
}).replace(/</g, "\\u003c");


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
        <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: SITE_LD }} />
      </head>
      <body>
        <a className="skip" href="#view">
          Skip to the content
        </a>
        <CaptureSource />
        <Suspense fallback={null}>
          <ScrollTop />
        </Suspense>
        <Masthead />
        <SectionBar />
        {children}
        <Sponsor />
        <SiteFooter />
        <Toaster />
        <FloatingActions />
        {/* Cloudflare Web Analytics: cookie-free page counts, loaded only when a token is set. */}
        {process.env.CF_ANALYTICS_TOKEN && (
          <Script src="https://static.cloudflareinsights.com/beacon.min.js" strategy="afterInteractive" data-cf-beacon={JSON.stringify({ token: process.env.CF_ANALYTICS_TOKEN })} />
        )}
      </body>
    </html>
  );
}
