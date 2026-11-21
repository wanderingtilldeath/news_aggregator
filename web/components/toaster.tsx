"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { TOAST_EVENT, type ToastDetail } from "@/lib/toast";

// Shows the message of the latest showToast call for a few seconds (longer when it offers an undo). It is a
// polite live region, so a screen reader announces it without taking focus.
export default function Toaster() {
  const [toast, setToast] = useState<ToastDetail | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);

  useEffect(() => {
    const show = (e: Event) => {
      const detail = (e as CustomEvent<ToastDetail>).detail;
      clearTimeout(timer.current);
      setToast(detail);
      timer.current = setTimeout(() => setToast(null), detail.undo ? 7000 : 4500);
    };
    window.addEventListener(TOAST_EVENT, show);
    return () => {
      window.removeEventListener(TOAST_EVENT, show);
      clearTimeout(timer.current);
    };
  }, []);

  return (
    <div className="toast-region" role="status" aria-live="polite">
      {toast && (
        <div className="toast">
          <span>{toast.message}</span>
          {toast.href && (
            <Link href={toast.href} onClick={() => setToast(null)}>
              {toast.label ?? "View"}
            </Link>
          )}
          {toast.undo && (
            <button
              type="button"
              onClick={() => {
                toast.undo?.();
                setToast(null);
              }}
            >
              Undo
            </button>
          )}
        </div>
      )}
    </div>
  );
}
