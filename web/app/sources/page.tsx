import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import { getPublicSources } from "@/lib/api";
import { CONTACT_EMAIL } from "@/lib/site";
import { catName, hostName } from "@/lib/view";

export const metadata: Metadata = {
  title: "Sources and takedown · Postpaper",
  description: "Which sites Postpaper reads, what each site's robots.txt says about AI use, and how a publisher can ask for removal.",
};

async function List() {
  const sources = await getPublicSources();
  if (!sources.length) return <p className="empty">The list of sources is not available right now.</p>;

  return (
    <table className="src-table">
      <caption className="sr-only">Sites Postpaper reads</caption>
      <thead>
        <tr>
          <th scope="col">Site</th>
          <th scope="col">Sections</th>
          <th scope="col">Edition</th>
          <th scope="col">AI use, per its robots.txt</th>
        </tr>
      </thead>
      <tbody>
        {sources.map((s) => (
          <tr key={s.host}>
            <th scope="row">
              <a href={`https://${s.host}/`} target="_blank" rel="noopener noreferrer">
                {hostName(s.host)}
              </a>
            </th>
            <td>{s.names.join(", ")}</td>
            <td>{s.categories.map(catName).join(", ")}</td>
            <td>
              <span className={`policy ${s.policy}`}>{s.policy === "restricts" ? "Asks AI users to keep out" : "No restriction found"}</span>
              {s.note && <small>{s.note}</small>}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

export default function Page() {
  return (
    <main className="page">
      <h1>Sources and takedown</h1>
      <p className="lead-in">Postpaper is a finished daily edition built from other publishers&rsquo; reporting. Here is what we take, what we do not, and how to ask us to stop.</p>

      <h2>What we do with a story</h2>
      <ul className="plain">
        <li>We read each site politely: we follow its robots.txt, wait between requests and identify ourselves.</li>
        <li>We show a headline, a short summary and a line on why it matters, written by an AI model from the article. We never show the publisher&rsquo;s full text.</li>
        <li>Every story links to the original article, and the reader goes there to read the whole piece.</li>
        <li>The summaries can be wrong. Check the source before relying on a fact or an exam note.</li>
      </ul>

      <h2>The sites we read</h2>
      <p>
        The last column is what each site&rsquo;s robots.txt said about AI use when we added it. &ldquo;Asks AI users to keep out&rdquo; means the site disallows some AI crawlers. We
        fetch those sites as an ordinary reader would and summarise for readers; if you run one and you object, ask us to remove it below.
      </p>
      <Suspense fallback={null}>
        <List />
      </Suspense>

      <h2 id="takedown">Ask us to remove a site or fix a story</h2>
      <p>
        If you publish one of these sites and want us to stop, email <a href={`mailto:${CONTACT_EMAIL}?subject=Postpaper%20takedown`}>{CONTACT_EMAIL}</a> from an address on your domain, or from one
        we can check is yours. We stop crawling the site and delete its stories from Postpaper, usually within a day, and we do not need a reason.
      </p>
      <p>
        If a summary is wrong, email the same address with the story&rsquo;s link and what is wrong, and we will correct or remove it.
      </p>

      <p>
        <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}
