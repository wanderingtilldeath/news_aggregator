"use client";

import { useEffect, useState, useSyncExternalStore } from "react";

const noop = () => () => {};

// Reads text aloud with the browser's own speech engine: nothing is sent to a server and it costs
// nothing. The button is not rendered where the browser has no speech support.
export default function ReadAloud({ text, label = "Listen" }: { text: string; label?: string }) {
  const supported = useSyncExternalStore(
    noop,
    () => "speechSynthesis" in window,
    () => false,
  );
  const [speaking, setSpeaking] = useState(false);

  // Stop when the reader leaves the page.
  useEffect(() => () => window.speechSynthesis?.cancel(), []);

  if (!supported) return null;

  function toggle() {
    const synth = window.speechSynthesis;
    if (speaking) {
      synth.cancel();
      setSpeaking(false);
      return;
    }
    const say = new SpeechSynthesisUtterance(text);
    say.onend = say.onerror = () => setSpeaking(false);
    synth.cancel();
    synth.speak(say);
    setSpeaking(true);
  }

  return (
    <button type="button" className="listen" onClick={toggle} aria-pressed={speaking}>
      <span aria-hidden="true">{speaking ? "■" : "▶"}</span> {speaking ? "Stop" : label}
    </button>
  );
}
