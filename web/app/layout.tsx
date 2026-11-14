import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Postpaper",
  description: "Articles from news, exam-prep and technology sites, gathered every morning.",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
