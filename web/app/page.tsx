import { Suspense } from "react";
import Edition from "@/components/edition";
import SubscribeForm from "@/components/subscribe-form";

export default function Page({ searchParams }: PageProps<"/">) {
  return (
    <>
      <a className="skip" href="#view">
        Skip to the articles
      </a>
      <Suspense fallback={<div className="wrap"><div className="skeleton" /></div>}>
        <Edition searchParams={searchParams} />
      </Suspense>
      <SubscribeForm />
      <footer>
        <div className="wrap">
          <span>Every headline links to the original publisher. Summaries are generated from the article text.</span>
        </div>
      </footer>
    </>
  );
}
