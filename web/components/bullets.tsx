"use client";

import { useState } from "react";
import { shorten } from "@/lib/view";

function Bullet({ text }: { text: string }) {
  const [open, setOpen] = useState(false);
  const { short, cut } = shorten(text);
  if (!cut) return <li>{text}</li>;

  return (
    <li>
      {open ? text : short}{" "}
      <button type="button" className="more-inline" aria-expanded={open} onClick={() => setOpen(!open)}>
        {open ? "less" : "more"}
      </button>
    </li>
  );
}

// The gist bullets of a story. A long sentence is shown short first and opens to the whole sentence
// when the reader asks, so nothing is thrown away and nothing reads as a cut-off thought.
export default function Bullets({ bullets }: { bullets: string[] }) {
  return (
    <ul>
      {bullets.map((b, i) => (
        <Bullet key={i} text={b} />
      ))}
    </ul>
  );
}
