import type { Metadata } from "next";
import { SiteFooter, SiteHeader } from "@/components/site";
import "./globals.css";

export const metadata: Metadata = {
  title: { default: "Postpaper", template: "%s" },
  description: "The finished daily newspaper for UPSC aspirants and engineers: ranked stories, a line on why each matters, a daily quiz.",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en">
      <body>
        <a className="skip" href="#view">
          Skip to the content
        </a>
        <SiteHeader />
        {children}
        <SiteFooter />
      </body>
    </html>
  );
}
