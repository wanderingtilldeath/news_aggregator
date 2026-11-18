import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import Loading from "@/components/loading";
import { connection } from "next/server";
import { getConcepts } from "@/lib/api";
import { conceptId } from "@/lib/concepts";

export const metadata: Metadata = {
  title: "Concepts · Postpaper",
  description: "Plain-language explainers for the ideas behind the news, written once and shown on every story that needs them.",
};

async function Glossary() {
  await connection();
  const concepts = await getConcepts();
  if (!concepts.length) return <p className="empty">The glossary fills up as stories come in. Check back after the next edition.</p>;

  const letters = [...new Set(concepts.map((c) => c.term.charAt(0).toUpperCase()))];

  return (
    <>
      <nav className="gl-letters" aria-label="Jump to a letter">
        {letters.map((l) => (
          <a key={l} href={`#letter-${l}`}>
            {l}
          </a>
        ))}
      </nav>
      {letters.map((l) => (
        <section key={l} id={`letter-${l}`}>
          <h2 className="gl-letter">{l}</h2>
          <dl className="glossary">
            {concepts
              .filter((c) => c.term.charAt(0).toUpperCase() === l)
              .map((c) => (
                <div key={c.term} id={conceptId(c.term)}>
                  <dt>{c.term}</dt>
                  <dd>{c.body}</dd>
                </div>
              ))}
          </dl>
        </section>
      ))}
    </>
  );
}

export default function Page() {
  return (
    <main className="page" id="view">
      <h1>Concepts</h1>
      <p className="lead-in">The ideas behind the news, explained in a line or two. Each is written once and appears on every story that needs it.</p>
      <p className="fine-print">Explainers are written by an AI model; check one against a reference before you rely on it for the exam.</p>
      <Suspense fallback={<Loading />}>
        <Glossary />
      </Suspense>
      <p>
        <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}
