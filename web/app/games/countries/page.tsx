import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import { connection } from "next/server";
import CountryGuess from "@/components/country-guess";
import Loading from "@/components/loading";
import countries from "@/lib/data/countries.json";
import { countryOfDay } from "@/lib/games";
import { dayKey } from "@/lib/wordle";

export const metadata: Metadata = { title: "Country guess · Postpaper", description: "Name the country from its outline. Each guess shows how far away and in which direction." };

async function Body() {
  await connection();
  const day = dayKey(new Date());
  const target = countryOfDay(countries, day);
  return <CountryGuess day={day} target={target} list={countries.map(({ name, lat, lon }) => ({ name, lat, lon }))} />;
}

export default function Page() {
  return (
    <main className="page" id="view">
      <h1>Country guess</h1>
      <p className="lead-in">Name the country from its outline in six tries. After each guess you see how far away it was and which way to go.</p>
      <Suspense fallback={<Loading />}>
        <Body />
      </Suspense>
      <p className="fine-print">India, Pakistan and China are never the answer. Their borders are disputed, so we do not draw them.</p>
      <p>
        <Link href="/games">All games</Link>
      </p>
    </main>
  );
}
