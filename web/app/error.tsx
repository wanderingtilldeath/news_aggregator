"use client";

export default function Error({ error, reset }: { error: Error; reset: () => void }) {
  return (
    <main className="empty" id="view">
      <p>Could not load the news: {error.message}</p>
      <button type="button" onClick={reset}>
        Try again
      </button>
    </main>
  );
}
