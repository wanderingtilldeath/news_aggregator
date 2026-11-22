import Loading from "@/components/loading";

// The placeholder for any page that has to fetch before it can draw, shown at once when a link to it is pressed, so
// the page never looks frozen. Pages with their own loading.tsx (the account area) use that one instead.
export default function PageLoading() {
  return (
    <main className="content" id="view">
      <Loading />
    </main>
  );
}
