"use client";

import { forwardRef, useEffect, useId, useRef, useState } from "react";
import { useDismiss } from "./more-menu";

type Props = Omit<React.InputHTMLAttributes<HTMLInputElement>, "list" | "role" | "autoComplete"> & {
  // A fixed list to filter in the browser; without it the suggestions come from /api/suggest.
  options?: string[];
};

// A search field that offers suggestions as you type. It stays a plain input inside its form, so without
// script the form still submits as before; choosing a suggestion fills the field and submits the form.
const SuggestInput = forwardRef<HTMLInputElement, Props>(function SuggestInput({ options, onChange, onKeyDown, ...rest }, ref) {
  const id = useId();
  const box = useRef<HTMLSpanElement>(null);
  const input = useRef<HTMLInputElement | null>(null);
  const [items, setItems] = useState<string[]>([]);
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(-1);

  useDismiss(open, setOpen, box);

  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const pending = useRef<AbortController>(undefined);
  const show = (list: string[]) => {
    setItems(list);
    setActive(-1);
    setOpen(list.length > 0);
  };

  // Looks up suggestions 150 ms after the last keystroke and drops an answer that arrives late.
  const look = (text: string) => {
    clearTimeout(timer.current);
    const q = text.trim().toLowerCase();
    if (q.length < 2) return show([]);
    if (options) {
      const hit = options.filter((o) => o.toLowerCase().includes(q));
      return show([...hit.filter((o) => o.toLowerCase().startsWith(q)), ...hit.filter((o) => !o.toLowerCase().startsWith(q))].slice(0, 8));
    }
    timer.current = setTimeout(async () => {
      pending.current?.abort();
      const ctl = (pending.current = new AbortController());
      try {
        const res = await fetch(`/api/suggest?q=${encodeURIComponent(q)}`, { signal: ctl.signal });
        if (!res.ok) return;
        const body = (await res.json()) as { suggestions?: string[] };
        show(body.suggestions ?? []);
      } catch {
        // A failed or aborted lookup just shows no suggestions.
      }
    }, 150);
  };
  useEffect(() => () => {
    clearTimeout(timer.current);
    pending.current?.abort();
  }, []);

  const pick = (text: string) => {
    const el = input.current;
    if (!el) return;
    el.value = text;
    setOpen(false);
    el.form?.requestSubmit();
  };

  return (
    <span className="suggest" ref={box}>
      <input
        {...rest}
        ref={(el) => {
          input.current = el;
          if (typeof ref === "function") ref(el);
          else if (ref) ref.current = el;
        }}
        role="combobox"
        autoComplete="off"
        aria-autocomplete="list"
        aria-expanded={open}
        aria-controls={`${id}-list`}
        aria-activedescendant={active >= 0 ? `${id}-${active}` : undefined}
        onChange={(e) => {
          onChange?.(e);
          look(e.target.value);
        }}
        onKeyDown={(e) => {
          onKeyDown?.(e);
          if (!open || items.length === 0) return;
          if (e.key === "ArrowDown" || e.key === "ArrowUp") {
            e.preventDefault();
            setActive((a) => (e.key === "ArrowDown" ? (a + 1) % items.length : (a - 1 + items.length) % items.length));
          } else if (e.key === "Enter" && active >= 0) {
            e.preventDefault();
            pick(items[active]);
          }
        }}
      />
      {open && (
        <ul id={`${id}-list`} role="listbox" className="suggest-list">
          {items.map((s, i) => (
            <li key={s} id={`${id}-${i}`} role="option" aria-selected={i === active} onMouseDown={(e) => { e.preventDefault(); pick(s); }}>
              {s}
            </li>
          ))}
        </ul>
      )}
    </span>
  );
});

export default SuggestInput;
