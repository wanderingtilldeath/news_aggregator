"use client";

import { useEffect, useRef } from "react";

const KEY = "pp-src";

// Remembers where a visitor came from: the campaign tag of the link they arrived by (?utm_source=linkedin,
// or ?ref=). It is kept for the tab only, so a visitor who lands on a story and signs up from the home page
// still counts for the link. Rendered once in the layout.
export function CaptureSource() {
  useEffect(() => {
    try {
      const q = new URLSearchParams(window.location.search);
      const tag = q.get("utm_source") ?? q.get("ref");
      if (tag) sessionStorage.setItem(KEY, tag.slice(0, 40));
    } catch {
      /* storage blocked: the signup is simply counted as direct */
    }
  }, []);
  return null;
}

// A hidden field of the signup form that carries the remembered tag to the API, which cleans it.
export function SourceField() {
  const box = useRef<HTMLInputElement>(null);
  useEffect(() => {
    try {
      if (box.current) box.current.value = sessionStorage.getItem(KEY) ?? "";
    } catch {
      /* blocked: left empty */
    }
  }, []);
  return <input ref={box} type="hidden" name="source" defaultValue="" />;
}
