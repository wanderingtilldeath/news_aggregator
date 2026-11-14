"use client";

// Opens the browser's print dialog, where "Save as PDF" makes the weekly revision a file.
export default function PrintButton() {
  return (
    <div className="printbar">
      <button type="button" onClick={() => window.print()}>
        Print or save as PDF
      </button>
    </div>
  );
}
