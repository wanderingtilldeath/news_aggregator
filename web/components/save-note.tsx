"use client";

import { useSyncExternalStore } from "react";
import { MAX_NOTES, NOTEBOOK_KEY, PRO_MAX_NOTES, PRO_TOKEN_KEY, noteId, parseNotes, toggled, type Note } from "@/lib/notebook";
import { pushDelete, pushSave } from "@/lib/notebook-sync";
import { readLocal, subscribeLocal, writeLocal } from "@/lib/local";
import { showToast } from "@/lib/toast";

// A star beside a sentence on a story page: one press saves it to the reader's notebook, another
// takes it out. The notebook lives in this browser; a Pro member's changes are also sent to the server.
export default function SaveNote({ story, title, text }: { story: string; title: string; text: string }) {
  const raw = useSyncExternalStore(subscribeLocal, () => readLocal(NOTEBOOK_KEY), () => "");
  const id = noteId(story, text);
  const saved = parseNotes(raw).some((n) => n.id === id);

  // Sends a Pro member's change to the server and says so if it could not be sent.
  async function sync(token: string, send: () => Promise<string>) {
    const why = await send();
    if (why) showToast({ message: `Saved on this device only: ${why}.` });
  }

  function press() {
    const token = readLocal(PRO_TOKEN_KEY);
    const note: Note = { id, text, story, title, at: Date.now() };
    const before = parseNotes(readLocal(NOTEBOOK_KEY));
    const max = token ? PRO_MAX_NOTES : MAX_NOTES;
    const wasSaved = before.some((n) => n.id === id);
    writeLocal(NOTEBOOK_KEY, JSON.stringify(toggled(before, note, max)));

    // Undo puts the notebook back as it was, here and on the server.
    const undo = () => {
      writeLocal(NOTEBOOK_KEY, JSON.stringify(before));
      if (token) void sync(token, () => (wasSaved ? pushSave(token, before.find((n) => n.id === id) ?? note) : pushDelete(token, id)));
    };

    // Say what happened, after checking it did: the browser may be blocking or have filled its storage.
    const now = parseNotes(readLocal(NOTEBOOK_KEY));
    const isSaved = now.some((n) => n.id === id);
    if (isSaved === wasSaved) {
      showToast({ message: "Could not save: this browser is blocking storage for the site." });
      return;
    }
    if (isSaved) {
      const full = before.length >= max;
      showToast({
        message: full ? `Saved to your notebook. It holds ${max} notes, so the oldest was dropped.` : token ? "Saved to your notebook and synced" : "Saved to your notebook",
        href: "/notebook",
        label: "View notebook",
        undo,
      });
      if (token) void sync(token, () => pushSave(token, note));
    } else {
      showToast({ message: "Removed from your notebook", undo });
      if (token) void sync(token, () => pushDelete(token, id));
    }
  }

  return (
    <button type="button" className={saved ? "note-btn saved" : "note-btn"} aria-pressed={saved} onClick={press} title={saved ? "Remove from your notebook" : "Save to your notebook"}>
      <span aria-hidden="true">{saved ? "★" : "☆"}</span>
      <span className="sr-only">{saved ? "Remove from your notebook" : "Save to your notebook"}</span>
    </button>
  );
}
