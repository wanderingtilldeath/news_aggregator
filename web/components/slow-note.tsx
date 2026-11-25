"use client";

import { useEffect, useState } from "react";

// Shown while a form waits on the API. A normal answer takes under a second, so say nothing at first; if the free
// API is waking from sleep, explain after 4 s and reassure after 15 s. It mounts when the wait starts, so its timers reset each time.
export default function SlowNote() {
  const [stage, setStage] = useState(0);
  useEffect(() => {
    const a = setTimeout(() => setStage(1), 4000);
    const b = setTimeout(() => setStage(2), 15000);
    return () => {
      clearTimeout(a);
      clearTimeout(b);
    };
  }, []);

  if (stage === 0) return null;
  return (
    <p className="note" role="status">
      {stage === 1 ? "Taking longer than expected… hang tight." : "Almost there. Please keep this page open."}
    </p>
  );
}
