import { SITE_URL } from "@/lib/site";

// Share links are plain anchors to each app's share page: no script and no tracking from us.
export default function Share({ id, title }: { id: string; title: string }) {
  const link = encodeURIComponent(`${SITE_URL}/story/${id}`);
  const text = encodeURIComponent(title);
  const targets = [
    ["WhatsApp", `https://wa.me/?text=${text}%20${link}`],
    ["Telegram", `https://t.me/share/url?url=${link}&text=${text}`],
    ["X", `https://twitter.com/intent/tweet?url=${link}&text=${text}`],
  ];

  return (
    <div className="share" aria-label="Share this story">
      <span>Share</span>
      {targets.map(([name, href]) => (
        <a key={name} href={href} target="_blank" rel="noopener noreferrer">
          {name}
        </a>
      ))}
    </div>
  );
}
