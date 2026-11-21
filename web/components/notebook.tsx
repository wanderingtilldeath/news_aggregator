"use client";

import Link from "next/link";
import { useEffect, useState, useSyncExternalStore } from "react";
import { proNotebookStatus } from "@/app/actions";
import { NOTEBOOK_KEY, PRO_SYNCED_KEY, PRO_TOKEN_KEY, allTags, filterNotes, parseNotes, parseTags, toMarkdown, tokenFromInput, type Note } from "@/lib/notebook";
import { pullNotebook, pushDelete, pushSave, syncNotebook } from "@/lib/notebook-sync";
import { readLocal, subscribeLocal, writeLocal } from "@/lib/local";

type ProState = { token: string; state: "active" | "lapsed" | "invalid" | "error"; until?: string; problem: string };

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

// The Pro member's own note and tags under one saved sentence. Each is saved when the field loses focus. The text
// is kept here while it is typed and follows the saved note when a sync changes it from outside.
function NoteEditor({ n, onSave, onDone }: { n: Note; onSave: (patch: Partial<Note>) => void; onDone: () => void }) {
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
    <div className="np-edit">
      <label className="sr-only" htmlFor={`own-${n.id}`}>
        Your note on this sentence
      </label>
      <textarea
        id={`own-${n.id}`}
        value={own}
        onChange={(e) => setOwn(e.target.value)}
        rows={2}
        maxLength={5000}
        placeholder="Your note"
        autoFocus
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
      <button type="button" className="np-done" onClick={onDone}>
        Done
      </button>
    </div>
  );
}

// The notepad page: the sentences a reader starred on story pages, as one card per story, newest first, each
// sentence with a way to remove it; the whole pad can be copied, exported as Markdown (Notion and most note apps
// import it), printed or cleared. It reads this browser's storage. A Pro member can connect their Pro link: the
// notepad is then also kept on the server (so it follows them to other devices, up to 10,000 notes), each sentence
// can carry the member's own note and tags, and the pad can be searched.
export default function Notebook() {
  const raw = useSyncExternalStore(subscribeLocal, () => readLocal(NOTEBOOK_KEY), () => "");
  const token = useSyncExternalStore(subscribeLocal, () => readLocal(PRO_TOKEN_KEY), () => "");
  const ready = useSyncExternalStore(() => () => {}, () => true, () => false);
  const notes = parseNotes(raw);

  const [checked, setChecked] = useState<ProState | null>(null);
  const [query, setQuery] = useState("");
  const [tag, setTag] = useState("");
  const [pasted, setPasted] = useState("");
  const [showConnect, setShowConnect] = useState(false);
  const [connectError, setConnectError] = useState("");
  const [problem, setProblem] = useState("");
  const [editing, setEditing] = useState("");

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

  const shown = active ? filterNotes(notes, query, tag) : notes;
  const groups = new Map<string, { title: string; notes: Note[] }>();
  for (const n of shown) {
    if (!groups.has(n.story)) groups.set(n.story, { title: n.title, notes: [] });
    groups.get(n.story)!.notes.push(n);
  }
  const storyCount = new Set(notes.map((n) => n.story)).size;

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
    if (!window.confirm(token ? "Clear your whole notepad, here and on the server?" : "Clear your whole notepad?")) return;
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
    a.download = "postpaper-notepad.md";
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
    setShowConnect(false);
    writeLocal(PRO_TOKEN_KEY, t);
  }

  const disconnect = () => {
    if (!window.confirm("Disconnect Pro on this browser? Your notes stay here, and stay on the server.")) return;
    writeLocal(PRO_TOKEN_KEY, "");
    writeLocal(PRO_SYNCED_KEY, "");
    setProblem("");
  };

  if (!ready) {
    return (
      <header className="np-head">
        <h1>Notepad</h1>
      </header>
    );
  }

  // One slim line says where the notes live, and what Pro adds.
  const strip = (
    <>
      {!token && (
        <div className="np-strip">
          <span>
            <b>Saved on this device.</b> Pro keeps your notepad on every device and adds your own notes, tags and search.
          </span>
          <button type="button" className="np-link" aria-expanded={showConnect} onClick={() => setShowConnect(!showConnect)}>
            Connect Pro
          </button>
          <Link href="/pro">About Pro</Link>
        </div>
      )}
      {!token && showConnect && (
        <form onSubmit={connect} className="np-connect">
          <label className="sr-only" htmlFor="pro-link">
            Your Pro link
          </label>
          <input id="pro-link" type="text" value={pasted} onChange={(e) => setPasted(e.target.value)} placeholder="Paste your Pro link" autoComplete="off" spellCheck={false} autoFocus />
          <button type="submit" disabled={!pasted.trim()}>
            Connect
          </button>
        </form>
      )}
      {!token && connectError && (
        <p className="note err" role="status">
          {connectError}
        </p>
      )}
      {token && !pro && <div className="np-strip">Checking your Pro membership…</div>}
      {pro?.state === "active" && (
        <div className="np-strip ok">
          <span>
            <b>Pro</b> · synced across your devices · active until {pro.until}
          </span>
          <button type="button" className="np-link" onClick={disconnect}>
            Disconnect
          </button>
        </div>
      )}
      {pro?.state === "lapsed" && (
        <div className="np-strip">
          <span>
            <b>Your Pro period ended on {pro.until}.</b> Your notes are still yours. <Link href="/pro">Renew</Link> to sync and add notes again.
          </span>
          <button type="button" className="np-link" onClick={disconnect}>
            Disconnect
          </button>
        </div>
      )}
      {pro?.state === "invalid" && (
        <div className="np-strip">
          <span>This Pro link is no longer valid.</span>
          <button type="button" className="np-link" onClick={disconnect}>
            Disconnect
          </button>
        </div>
      )}
      {pro?.state === "error" && <div className="np-strip">We could not reach the server to check your membership, so this shows the notes on this device.</div>}
      {(pro?.problem || problem) && (
        <p className="note err" role="status">
          {problem || `Not synced: ${pro?.problem}.`}
        </p>
      )}
    </>
  );

  const tags = active ? allTags(notes) : [];

  return (
    <>
      <header className="np-head">
        <div>
          <h1>Notepad</h1>
          {notes.length > 0 && (
            <p className="np-count">
              {plural(notes.length, "saved sentence", "saved sentences")} from {plural(storyCount, "story", "stories")}
            </p>
          )}
        </div>
        {notes.length > 0 && (
          <div className="np-tools" role="group" aria-label="Notepad actions">
            <button type="button" onClick={copy}>
              Copy
            </button>
            <button type="button" onClick={download} title="A Markdown file that Notion and most note apps import">
              Export .md
            </button>
            <button type="button" onClick={() => window.print()} title="Print or save as PDF">
              Print
            </button>
            <button type="button" className="danger" onClick={clear}>
              Clear
            </button>
          </div>
        )}
      </header>

      {strip}

      {notes.length === 0 ? (
        <div className="np-empty">
          <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <path d="M6 3h11a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6V3Z" />
            <path d="M6 3v18M10 8h5M10 12h5" />
          </svg>
          <h2>Nothing saved yet</h2>
          <p>Press the star beside a sentence on any story to keep it here.</p>
          <Link href="/" className="btn primary">
            Read today&rsquo;s stories
          </Link>
        </div>
      ) : (
        <>
          {active && (
            <div className="np-filter">
              <label className="sr-only" htmlFor="np-q">
                Search your notepad
              </label>
              <input id="np-q" type="search" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search your notepad" autoComplete="off" />
              {tags.length > 0 && (
                <div className="np-tagbar" role="group" aria-label="Filter by tag">
                  {tags.map((t) => (
                    <button key={t} type="button" aria-pressed={tag === t} onClick={() => setTag(tag === t ? "" : t)}>
                      #{t}
                    </button>
                  ))}
                </div>
              )}
            </div>
          )}
          {shown.length === 0 && <p className="empty">Nothing matches.</p>}
          <div className="np-grid">
            {[...groups.entries()].map(([story, g]) => (
              <article key={story} className="np-card">
                <h2>
                  <Link href={`/story/${encodeURIComponent(story)}`}>{g.title}</Link>
                </h2>
                <p className="np-meta">
                  {plural(g.notes.length, "sentence", "sentences")} · saved {new Date(g.notes[0].at).toLocaleDateString("en-IN", { day: "numeric", month: "short" })}
                </p>
                <ul>
                  {g.notes.map((n) => (
                    <li key={n.id} className="np-item">
                      <p className="np-quote">{n.text}</p>
                      {n.tags?.length ? (
                        <p className="np-chips">
                          {n.tags.map((t) => (
                            <span key={t}>#{t}</span>
                          ))}
                        </p>
                      ) : null}
                      {n.note?.trim() && (
                        <p className="np-own">
                          <span>Your note</span>
                          {n.note}
                        </p>
                      )}
                      {active && editing === n.id && <NoteEditor n={n} onSave={(patch) => edit(n, patch)} onDone={() => setEditing("")} />}
                      <div className="np-actions">
                        {active && editing !== n.id && (
                          <button type="button" onClick={() => setEditing(n.id)}>
                            {n.note?.trim() || n.tags?.length ? "Edit note" : "Add note"}
                          </button>
                        )}
                        <button type="button" onClick={() => remove(n.id)} aria-label={`Remove: ${n.text.slice(0, 60)}`}>
                          Remove
                        </button>
                      </div>
                    </li>
                  ))}
                </ul>
              </article>
            ))}
          </div>
        </>
      )}
    </>
  );
}
