import { Suspense } from "react";
import Edition from "@/components/edition";

// What shows while the edition loads: the shape of the page, not a spinner.
function Skeleton() {
  return (
    <main className="content" id="view" aria-busy="true" aria-label="Loading the edition">
      <div className="sk sk-title" style={{ width: "55%", height: 48 }} />
      <div className="sk sk-line" style={{ width: "40%" }} />
      <div className="sk-brief">
        <div className="sk-card">
          <div className="sk sk-img" />
          <div className="sk sk-title" />
          <div className="sk sk-line" />
          <div className="sk sk-line" style={{ width: "85%" }} />
        </div>
        <div className="sk-card">
          {[0, 1, 2, 3].map((i) => (
            <div key={i}>
              <div className="sk sk-line" />
              <div className="sk sk-line" style={{ width: "60%" }} />
            </div>
          ))}
        </div>
      </div>
    </main>
  );
}

export default function Page({ searchParams }: PageProps<"/">) {
  return (
    <Suspense fallback={<Skeleton />}>
      <Edition searchParams={searchParams} />
    </Suspense>
  );
}
