import type { ReactNode } from "react";

// A settings card: a title, a line saying what it does, and the controls. tone "danger" is for deleting the account.
export function Card({ title, hint, tone, children }: { title: string; hint?: ReactNode; tone?: "danger"; children?: ReactNode }) {
  return (
    <section className={tone ? `acct-card ${tone}` : "acct-card"}>
      <header>
        <h2>{title}</h2>
        {hint && <p>{hint}</p>}
      </header>
      {children}
    </section>
  );
}

// A status tile on the overview: a label, the answer, a line of detail and one link to the page that changes it.
// tone "warn" is for something the reader should act on.
export function Stat({ label, value, detail, tone, children }: { label: string; value: ReactNode; detail?: ReactNode; tone?: "ok" | "warn"; children?: ReactNode }) {
  return (
    <div className={tone ? `acct-stat ${tone}` : "acct-stat"}>
      <span className="acct-stat-label">{label}</span>
      <b>{value}</b>
      {detail && <p>{detail}</p>}
      {children}
    </div>
  );
}

// Ten dots for the ten recovery codes: the unused ones are filled.
export function Meter({ left, of = 10 }: { left: number; of?: number }) {
  return (
    <div className="acct-meter" role="img" aria-label={`${left} of ${of} recovery codes left`}>
      {Array.from({ length: of }, (_, i) => (
        <span key={i} className={i < left ? "on" : undefined} />
      ))}
    </div>
  );
}
