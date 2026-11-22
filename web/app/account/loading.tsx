// Shown in the account area's content column the moment a link inside it is pressed (Your account, Membership,
// Sign-in and security), while the next page fetches the account. The side menu stays; only the page is a placeholder.
export default function Loading() {
  return (
    <div className="acct-skel" role="status" aria-live="polite">
      <span className="sr-only">Loading…</span>
      <div className="sk sk-title" />
      <div className="acct-skel-grid">
        <div className="sk acct-skel-card" />
        <div className="sk acct-skel-card" />
        <div className="sk acct-skel-card" />
      </div>
      <div className="sk sk-line" />
      <div className="sk sk-line" />
    </div>
  );
}
