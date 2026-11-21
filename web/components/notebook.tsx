"use client";

import Link from "next/link";
import { useEffect, useState, useSyncExternalStore } from "react";
import { proNotebookStatus } from "@/app/actions";
import { NOTEBOOK_KEY, PRO_SYNCED_KEY, PRO_TOKEN_KEY, allTags, filterNotes, parseNotes, parseTags, toMarkdown, tokenFromInput, type Note } from "@/lib/notebook";
import { pullNotebook, pushDelete, pushSave, syncNotebook } from "@/lib/notebook-sync";
import { readLocal, subscribeLocal, writeLocal } from "@/lib/local";

// The Pro member's own note and tags under one saved sentence. Each is saved when the field loses focus. The text
// is kept here while it is typed and follows the saved note when a sync changes it from outside.
function NoteEditor({ n, onSave }: { n: Note; onSave: (patch: Partial<Note>) => void }) {
  const incoming = { note: n.note ?? "", tags: (n.tags ?? []).join(", ") };
  const [saved, setSaved] = useState(incoming);
  const [own, setOwn] = useState(incoming.note);
  const [tags, setTags] = useState(incoming.tags);
  if (incoming.note !== saved.note || incoming.tags !== saved.tags) {
    setSaved(incoming);
    setOwn(incoming.note);
    setTags(incoming.tags);
  }

  return (
    <div className="nb-edit">
      <label className="sr-only" htmlFor={`own-${n.id}`}>
        Your note on this sentence
      </label>
      <textarea
        id={`own-${n.id}`}
        value={own}
        onChange={(e) => setOwn(e.target.value)}
        rows={2}
        maxLength={5000}
        placeholder="Add your note"
        onBlur={() => own.trim() !== saved.note && onSave({ note: own.trim() })}
      />
      <label className="sr-only" htmlFor={`tags-${n.id}`}>
        Tags for this sentence
      </label>
      <input
        id={`tags-${n.id}`}
        type="text"
        value={tags}
        onChange={(e) => setTags(e.target.value)}
        placeholder="Tags, separated by commas"
        maxLength={200}
        autoComplete="off"
        onBlur={() => {
          const next = parseTags(tags);
          if (next.join(", ") !== saved.tags) onSave({ tags: next });
        }}
      />
    </div>
  );
}

type ProState = { token: string; state: "active" | "lapsed" | "invalid" | "error"; until?: string; problem: string };

// The notebook page: the saved sentences grouped by story, newest story first, with a way to take one out, copy
// everything as text, download it as a Markdown file (Notion and most note apps import it), print or save as PDF,
// or clear it. It reads this browser's storage. A Pro member can connect their Pro link: the notebook is then also
// kept on the server (so it follows them to other devices, up to 10,000 notes), and each saved sentence can carry
// the member's own note and tags, and the notebook can be searched.
export default function Notebook() {
  const raw = useSyncExternalStore(subscribeLocal, () => readLocal(NOTEBOOK_KEY), () => "");
  const token = useSyncExternalStore(subscribeLocal, () => readLocal(PRO_TOKEN_KEY), () => "");
  const notes = parseNotes(raw);

  const [checked, setChecked] = useState<ProState | null>(null);
  const [query, setQuery] = useState("");
  const [tag, setTag] = useState("");
  const [pasted, setPasted] = useState("");
  const [connectError, setConnectError] = useState("");
  const [problem, setProblem] = useState("");

  // Checks the membership and syncs, whenever a Pro link is connected.
  useEffect(() => {
    if (!token) return;
    let live = true;
    void (async () => {
      const st = await proNotebookStatus(token);
      let why = "";
      if (st.state === "active") why = await syncNotebook(token);
      else if (st.state === "lapsed") why = await pullNotebook(token);
      if (live) setChecked({ token, state: st.state, until: st.until, problem: why });
    })();
    return () => {
      live = false;
    };
  }, [token]);

  const pro = checked && checked.token === token ? checked : null;
  const active = pro?.state === "active";

  const groups = new Map<string, { title: string; notes: Note[] }>();
  const shown = active ? filterNotes(notes, query, tag) : notes;
  for (const n of shown) {
    if (!groups.has(n.story)) groups.set(n.story, { title: n.title, notes: [] });
    groups.get(n.story)!.notes.push(n);
  }

  const save = (list: Note[]) => writeLocal(NOTEBOOK_KEY, JSON.stringify(list));
  // A change is made here first, then sent to the server for a connected member; a failure is shown, not hidden.
  const tell = (why: string) => setProblem(why ? `Saved on this device only: ${why}.` : "");
  const remove = (id: string) => {
    save(notes.filter((x) => x.id !== id));
    if (token) void pushDelete(token, id).then(tell);
  };
  const edit = (n: Note, patch: Partial<Note>) => {
    const next = { ...n, ...patch };
    save(notes.map((x) => (x.id === n.id ? next : x)));
    if (token) void pushSave(token, next).then(tell);
  };
  const clear = () => {
    if (!window.confirm(token ? "Clear your whole notebook, here and on the server?" : "Clear your whole notebook?")) return;
    save([]);
    if (token) void pushDelete(token, "").then(tell);
  };
  const copy = () => {
    const text = [...groups.values()].map((g) => `${g.title}\n${g.notes.map((n) => `- ${n.text}`).join("\n")}`).join("\n\n");
    void navigator.clipboard?.writeText(text);
  };

  // A Markdown file made in the browser: nothing is uploaded.
  const download = () => {
    const url = URL.createObjectURL(new Blob([toMarkdown(notes, window.location.origin)], { type: "text/markdown;charset=utf-8" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = "postpaper-notebook.md";
    a.click();
    URL.revokeObjectURL(url);
  };

  async function connect(e: React.FormEvent) {
    e.preventDefault();
    const t = tokenFromInput(pasted);
    if (!t) return setConnectError("That does not look like a Pro link. Paste the link from your Pro email.");
    const st = await proNotebookStatus(t);
    if (st.state === "invalid") return setConnectError("That Pro link is not valid.");
    if (st.state === "error") return setConnectError("We could not reach the server just now. Try again in a little while.");
    setConnectError("");
    setPasted("");
    writeLocal(PRO_TOKEN_KEY, t);
  }

  const disconnect = () => {
    if (!window.confirm("Disconnect Pro on this browser? Your notes stay here, and stay on the server.")) return;
    writeLocal(PRO_TOKEN_KEY, "");
    writeLocal(PRO_SYNCED_KEY, "");
    setProblem("");
  };

  const panel = (
    <section className="nb-pro" aria-label="Pro notebook">
      {!token && (
        <>
          <h2>Postpaper Pro notebook</h2>
          <p>Keep your notebook on every device, hold up to 10,000 notes, add your own note under each saved sentence, tag them and search. Already a member? Paste your Pro link.</p>
          <form onSubmit={connect} className="nb-connect">
            <label className="sr-only" htmlFor="pro-link">
              Your Pro link
            </label>
            <input id="pro-link" type="text" value={pasted} onChange={(e) => setPasted(e.target.value)} placeholder="Paste your Pro link" autoComplete="off" spellCheck={false} />
            <button type="submit" disabled={!pasted.trim()}>
              Connect
            </button>
            <Link href="/pro">About Pro</Link>
          </form>
          {connectError && (
            <p className="note err" role="status">
              {connectError}
            </p>
          )}
        </>
      )}
      {token && !pro && <p>Checking your membership…</p>}
      {pro?.state === "active" && (
        <p>
          <b>Pro notebook.</b> Synced across your devices. Active until {pro.until}.{" "}
          <button type="button" className="nb-link" onClick={disconnect}>
            Disconnect
          </button>
        </p>
      )}
      {pro?.state === "lapsed" && (
        <p>
          <b>Your Pro period ended on {pro.until}.</b> Your notes are still yours and still here. <Link href="/pro">Renew</Link> to sync and add notes again.{" "}
          <button type="button" className="nb-link" onClick={disconnect}>
            Disconnect
          </button>
        </p>
      )}
      {pro?.state === "invalid" && (
        <p>
          This Pro link is no longer valid.{" "}
          <button type="button" className="nb-link" onClick={disconnect}>
            Disconnect
          </button>
        </p>
      )}
      {pro?.state === "error" && <p>We could not reach the server to check your membership, so this shows the notes on this device.</p>}
      {(pro?.problem || problem) && (
        <p className="note err" role="status">
          {problem || `Not synced: ${pro?.problem}.`}
        </p>
      )}
    </section>
  );

  if (!notes.length) {
    return (
      <>
        {panel}
        <p className="empty">Your notebook is empty. Press the star beside a sentence on a story page to save it here.</p>
      </>
    );
  }

  const tags = active ? allTags(notes) : [];

  return (
    <>
      {panel}
      <div className="printbar notebook-actions">
        <button type="button" onClick={copy}>
          Copy as text
        </button>
        <button type="button" onClick={download}>
          Download for Notion (.md)
        </button>
        <button type="button" onClick={() => window.print()}>
          Print or save as PDF
        </button>
        <button type="button" onClick={clear}>
          Clear
        </button>
      </div>
      {active && (
        <div className="nb-filter">
          <label className="sr-only" htmlFor="nb-q">
            Search your notes
          </label>
          <input id="nb-q" type="search" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search your notes" autoComplete="off" />
          {tags.length > 0 && (
            <div className="nb-tags" role="group" aria-label="Filter by tag">
              {tags.map((t) => (
                <button key={t} type="button" aria-pressed={tag === t} onClick={() => setTag(tag === t ? "" : t)}>
                  #{t}
                </button>
              ))}
            </div>
          )}
        </div>
      )}
      {shown.length === 0 && <p className="empty">No notes match.</p>}
      {[...groups.entries()].map(([story, g]) => (
        <section key={story} className="nb-story">
          <h2>
            <Link href={`/story/${encodeURIComponent(story)}`}>{g.title}</Link>
          </h2>
          <ul>
            {g.notes.map((n) => (
              <li key={n.id}>
                {n.text}{" "}
                <button type="button" className="nb-remove" onClick={() => remove(n.id)} aria-label="Remove this note">
                  ✕
                </button>
                {n.tags?.length ? <span className="nb-chips">{n.tags.map((t) => `#${t}`).join(" ")}</span> : null}
                {n.note?.trim() && <p className="nb-own print-only">My note: {n.note}</p>}
                {active && <NoteEditor n={n} onSave={(patch) => edit(n, patch)} />}
              </li>
            ))}
          </ul>
        </section>
      ))}
    </>
  );
}
