"use client";

import Link from "next/link";
import { useEffect, useState, useSyncExternalStore } from "react";
import { loadAccount } from "@/app/actions";
import { ACCOUNT_KEY, MAX_NOTES, MAX_TEXT, NOTEBOOK_KEY, OWN_STORY, PRO_MAX_NOTES, SYNC_KEY, allTags, filterNotes, ownNote, parseNotes, parseTags, toMarkdown, type Note } from "@/lib/notebook";
import { pullNotebook, pushDelete, pushSave, syncNotebook } from "@/lib/notebook-sync";
import { readLocal, subscribeLocal, writeLocal } from "@/lib/local";

// Where the reader stands: not logged in, logged in without a Pro membership, Pro and paid up, Pro and lapsed, or unknown.
type ProState = { state: "anon" | "none" | "active" | "lapsed" | "error"; username?: string; until?: string; problem: string };

// The note a reader typed, stamped with the time and a random part for its id (kept out of the component, which must
// not call impure functions while it renders).
const typedNote = (text: string) => ownNote(text, Date.now(), Math.random().toString(36).slice(2, 10));

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

// The editor under one note. It changes the saved sentence itself (or, for a note the reader typed, that note), adds
// the reader's own comment under a sentence, and, for a Pro member, its tags. Each field is saved when it loses focus.
// The text is kept here while it is typed and follows the saved note when a sync changes it from outside. An emptied
// sentence is put back: a note is removed with Remove, not by deleting its words.
function NoteEditor({ n, pro, onSave, onDone }: { n: Note; pro: boolean; onSave: (patch: Partial<Note>) => void; onDone: () => void }) {
  const own = n.story === OWN_STORY;
  const incoming = { text: n.text, note: n.note ?? "", tags: (n.tags ?? []).join(", ") };
  const [saved, setSaved] = useState(incoming);
  const [text, setText] = useState(incoming.text);
  const [comment, setComment] = useState(incoming.note);
  const [tags, setTags] = useState(incoming.tags);
  if (incoming.text !== saved.text || incoming.note !== saved.note || incoming.tags !== saved.tags) {
    setSaved(incoming);
    setText(incoming.text);
    setComment(incoming.note);
    setTags(incoming.tags);
  }

  return (
    <div className="np-edit">
      <label htmlFor={`text-${n.id}`}>{own ? "Your note" : "The saved sentence"}</label>
      <textarea
        id={`text-${n.id}`}
        value={text}
        onChange={(e) => setText(e.target.value)}
        rows={own ? 4 : 3}
        maxLength={MAX_TEXT}
        autoFocus
        onBlur={() => {
          const t = text.trim();
          if (!t) setText(saved.text);
          else if (t !== saved.text) onSave({ text: t });
        }}
      />
      {!own && (
        <>
          <label htmlFor={`own-${n.id}`}>Your comment on it</label>
          <textarea
            id={`own-${n.id}`}
            value={comment}
            onChange={(e) => setComment(e.target.value)}
            rows={2}
            maxLength={5000}
            placeholder="Add your own comment"
            onBlur={() => comment.trim() !== saved.note && onSave({ note: comment.trim() })}
          />
        </>
      )}
      {pro && (
        <>
          <label htmlFor={`tags-${n.id}`}>Tags</label>
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
        </>
      )}
      <button type="button" className="np-done" onClick={onDone}>
        Done
      </button>
    </div>
  );
}

// The notepad page: the sentences a reader starred on story pages, as one card per story, plus the notes the reader
// typed, in a card of their own. Anyone can write a note and reword, comment on or remove any of them; the whole pad
// can be copied, exported as Markdown (Notion and most note apps import it), printed or cleared. It reads this
// browser's storage. A logged-in account that holds a Pro membership has its notepad also kept on the server (so it
// follows it to other devices, up to 10,000 notes), can tag notes, and can search the pad.
export default function Notebook() {
  const raw = useSyncExternalStore(subscribeLocal, () => readLocal(NOTEBOOK_KEY), () => "");
  const ready = useSyncExternalStore(() => () => {}, () => true, () => false);
  const notes = parseNotes(raw);

  const [pro, setPro] = useState<ProState | null>(null);
  const [query, setQuery] = useState("");
  const [tag, setTag] = useState("");
  const [problem, setProblem] = useState("");
  const [editing, setEditing] = useState("");
  const [draft, setDraft] = useState("");

  // Finds out who is logged in and what their membership allows, then syncs. It also keeps the two flags the rest
  // of the site reads: the username for the bar and whether a star is sent to the server.
  useEffect(() => {
    let live = true;
    void (async () => {
      const acc = await loadAccount();
      let next: ProState;
      if (!acc.ok) {
        next = { state: acc.signedOut ? "anon" : "error", problem: "" };
      } else if (acc.data.pro.active) {
        next = { state: "active", username: acc.data.username, until: acc.data.pro.until, problem: await syncNotebook(acc.data.username) };
      } else if (acc.data.pro.linked) {
        next = { state: "lapsed", username: acc.data.username, until: acc.data.pro.until, problem: await pullNotebook() };
      } else {
        next = { state: "none", username: acc.data.username, problem: "" };
      }
      if (next.state !== "error") writeLocal(ACCOUNT_KEY, next.username ?? "");
      if (next.state !== "error") writeLocal(SYNC_KEY, next.state === "active" ? "1" : "");
      if (live) setPro(next);
    })();
    return () => {
      live = false;
    };
  }, []);

  const active = pro?.state === "active";
  // Editing waits until the account is known and is off when it cannot be sent: a Pro member's change made while the
  // server is out of reach would be overwritten by the next sync, and a lapsed member's notes are read-only copies.
  const canEdit = pro !== null && (pro.state === "anon" || pro.state === "none" || pro.state === "active");
  // A lapsed member can still delete, so a delete is sent for them too.
  const sendsDeletes = active || pro?.state === "lapsed";

  const shown = active ? filterNotes(notes, query, tag) : notes;
  const grouped = new Map<string, { title: string; notes: Note[] }>();
  for (const n of shown) {
    if (!grouped.has(n.story)) grouped.set(n.story, { title: n.title, notes: [] });
    grouped.get(n.story)!.notes.push(n);
  }
  // The reader's own notes come first, then the stories, newest first.
  const groups = new Map([...grouped.entries()].sort(([a], [b]) => Number(b === OWN_STORY) - Number(a === OWN_STORY)));
  const ownCount = notes.filter((n) => n.story === OWN_STORY).length;
  const sentenceCount = notes.length - ownCount;
  const storyCount = new Set(notes.filter((n) => n.story !== OWN_STORY).map((n) => n.story)).size;

  const save = (list: Note[]) => writeLocal(NOTEBOOK_KEY, JSON.stringify(list));
  // A change is made here first, then sent to the server for a Pro account; a failure is shown, not hidden.
  const tell = (why: string) => setProblem(why ? `Saved on this device only: ${why}.` : "");
  const remove = (id: string) => {
    save(notes.filter((x) => x.id !== id));
    if (sendsDeletes) void pushDelete(id).then(tell);
  };
  const edit = (n: Note, patch: Partial<Note>) => {
    const next = { ...n, ...patch };
    save(notes.map((x) => (x.id === n.id ? next : x)));
    if (active) void pushSave(next).then(tell);
  };
  // A note the reader typed goes to the front of the pad, and to the server for a Pro account.
  const add = () => {
    const note = typedNote(draft);
    if (!note) return;
    const max = active ? PRO_MAX_NOTES : MAX_NOTES;
    if (notes.length >= max) {
      setProblem(`Your notepad is full (${max} notes). Remove some to add more.`);
      return;
    }
    save([note, ...notes]);
    setDraft("");
    setProblem("");
    if (active) void pushSave(note).then(tell);
  };
  const clear = () => {
    if (!window.confirm(sendsDeletes ? "Clear your whole notepad, here and on the server?" : "Clear your whole notepad?")) return;
    save([]);
    if (sendsDeletes) void pushDelete("").then(tell);
  };
  const copy = () => {
    const text = [...groups.values()].map((g) => `${g.title}\n${g.notes.map((n) => `- ${n.text}${n.note?.trim() ? `\n  My comment: ${n.note.trim()}` : ""}`).join("\n")}`).join("\n\n");
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

  if (!ready) {
    return (
      <header className="np-head">
        <h1>Notepad</h1>
      </header>
    );
  }

  // One slim line says where the notes live, and what a Pro account adds.
  const strip = (
    <>
      {pro?.state === "anon" && (
        <div className="np-strip">
          <span>
            <b>Saved on this device.</b> A Pro account keeps your notepad on every device and adds tags and search.
          </span>
          <Link href="/login">Log in</Link>
          <Link href="/signup">Sign up</Link>
        </div>
      )}
      {pro?.state === "none" && (
        <div className="np-strip">
          <span>
            <b>Logged in as {pro.username}.</b> Link your Pro membership to keep your notepad on every device and add tags and search.
          </span>
          <Link href="/account">Link Pro</Link>
          <Link href="/pro">About Pro</Link>
        </div>
      )}
      {pro?.state === "active" && (
        <div className="np-strip ok">
          <span>
            <b>Pro</b> · {pro.username} · synced across your devices · active until {pro.until}
          </span>
          <Link href="/account">Account</Link>
        </div>
      )}
      {pro?.state === "lapsed" && (
        <div className="np-strip">
          <span>
            <b>Your Pro period ended on {pro.until}.</b> Your notes are still yours, but they can only be read, removed or exported until you <Link href="/pro">renew</Link>.
          </span>
          <Link href="/account">Account</Link>
        </div>
      )}
      {pro?.state === "error" && <div className="np-strip">We could not reach the server to check your account, so this shows the notes on this device.</div>}
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
              {[sentenceCount > 0 && `${plural(sentenceCount, "saved sentence", "saved sentences")} from ${plural(storyCount, "story", "stories")}`, ownCount > 0 && plural(ownCount, "note of your own", "notes of your own")]
                .filter(Boolean)
                .join(" · ")}
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

      {canEdit && (
        <form
          className="np-compose"
          onSubmit={(e) => {
            e.preventDefault();
            add();
          }}
        >
          <label className="sr-only" htmlFor="np-new">
            Write a note
          </label>
          <textarea id="np-new" value={draft} onChange={(e) => setDraft(e.target.value)} rows={3} maxLength={MAX_TEXT} placeholder="Write a note of your own…" />
          <button type="submit" disabled={!draft.trim()}>
            Add note
          </button>
        </form>
      )}

      {notes.length === 0 ? (
        <div className="np-empty">
          <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <path d="M6 3h11a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6V3Z" />
            <path d="M6 3v18M10 8h5M10 12h5" />
          </svg>
          <h2>Nothing saved yet</h2>
          <p>Press the star beside a sentence on any story to keep it here, or write a note of your own above.</p>
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
                <h2>{story === OWN_STORY ? g.title : <Link href={`/story/${encodeURIComponent(story)}`}>{g.title}</Link>}</h2>
                <p className="np-meta">
                  {story === OWN_STORY ? plural(g.notes.length, "note", "notes") : plural(g.notes.length, "sentence", "sentences")} · {story === OWN_STORY ? "written" : "saved"}{" "}
                  {new Date(g.notes[0].at).toLocaleDateString("en-IN", { day: "numeric", month: "short" })}
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
                      {canEdit && editing === n.id && <NoteEditor n={n} pro={active} onSave={(patch) => edit(n, patch)} onDone={() => setEditing("")} />}
                      <div className="np-actions">
                        {canEdit && editing !== n.id && (
                          <button type="button" onClick={() => setEditing(n.id)}>
                            Edit
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
