import type { Metadata } from "next";
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
  description: "The finished daily newspaper for UPSC aspirants and engineers: ranked stories, a line on why each matters, a daily quiz.",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en" className={`${sans.variable} ${serif.variable} ${brand.variable}`}>
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
