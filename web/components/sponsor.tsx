import { safeURL } from "@/lib/safe";

// One sponsor line, set with SPONSOR_TEXT and SPONSOR_URL (and SPONSOR_LABEL, "Sponsored" by default).
// It is always labelled and nothing is shown when the text or a safe link is missing.
export default function Sponsor() {
  const text = process.env.SPONSOR_TEXT?.trim();
  const href = safeURL(process.env.SPONSOR_URL);
  if (!text || !href) return null;

  return (
    <aside className="sponsor" aria-label="Sponsor">
      <span>{process.env.SPONSOR_LABEL?.trim() || "Sponsored"}</span>
      <a href={href} target="_blank" rel="sponsored noopener noreferrer">
        {text}
      </a>
    </aside>
  );
}
