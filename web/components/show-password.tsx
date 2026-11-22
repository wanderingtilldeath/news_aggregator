"use client";

// A "Show password" checkbox for a form. It turns every password box of the same form (the ones marked data-pw) into
// plain text and back, so a typo can be seen before it is sent. Nothing is stored; the boxes stay uncontrolled, so what
// was typed is kept when the type changes.
export default function ShowPassword() {
  return (
    <label className="auth-check">
      <input
        type="checkbox"
        onChange={(e) => {
          const form = e.currentTarget.form;
          form?.querySelectorAll<HTMLInputElement>("input[data-pw]").forEach((box) => {
            box.type = e.currentTarget.checked ? "text" : "password";
          });
        }}
      />{" "}
      Show password
    </label>
  );
}
