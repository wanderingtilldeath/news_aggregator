import { Suspense } from "react";
import Edition from "@/components/edition";

export default function Page({ searchParams }: PageProps<"/">) {
  return (
    <Suspense
      fallback={
        <main className="content" id="view">
          <div className="skeleton" />
        </main>
      }
    >
      <Edition searchParams={searchParams} />
    </Suspense>
  );
}
