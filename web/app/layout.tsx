import type { Metadata } from "next";
import { Inter, Newsreader } from "next/font/google";
import { Masthead, SectionBar, SiteFooter } from "@/components/site";
import "./globals.css";

// Self-hosted at build time by next/font: no request to Google when a reader opens the site.
const sans = Inter({ subsets: ["latin"], variable: "--font-sans", display: "swap" });
const serif = Newsreader({ subsets: ["latin"], variable: "--font-serif", display: "swap", style: ["normal", "italic"] });

export const metadata: Metadata = {
  title: { default: "Postpaper", template: "%s" },
  description: "The finished daily newspaper for UPSC aspirants and engineers: ranked stories, a line on why each matters, a daily quiz.",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en" className={`${sans.variable} ${serif.variable}`}>
      <body>
        <a className="skip" href="#view">
          Skip to the content
        </a>
        <Masthead />
        <SectionBar />
        {children}
        <SiteFooter />
      </body>
    </html>
  );
}
