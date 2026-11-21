// A short message at the bottom of the screen ("Saved to your notepad"). Any client code can call showToast; the
// Toaster in the layout listens for it. It has no server code, so client components can import it.

export type ToastDetail = {
  message: string;
  // An optional link ("View notepad") and an optional undo for what just happened.
  href?: string;
  label?: string;
  undo?: () => void;
};

export const TOAST_EVENT = "pp-toast";

export function showToast(detail: ToastDetail) {
  window.dispatchEvent(new CustomEvent<ToastDetail>(TOAST_EVENT, { detail }));
}
