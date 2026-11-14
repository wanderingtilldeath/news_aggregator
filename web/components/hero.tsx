import SubscribeForm from "./subscribe-form";

// The pitch above the news on the home page: who Postpaper is for, what a reader
// gets, and the signup form.
export default function Hero() {
  return (
    <section className="pitch" aria-labelledby="pitch-h">
      <div>
        <h2 id="pitch-h">The finished daily newspaper for UPSC aspirants and engineers.</h2>
        <p>
          Postpaper reads the sources worth reading, ranks the day&rsquo;s stories by importance, and gives each one a line on why it matters. Read it in one minute
          or seven, test yourself with a daily quiz, and be done. There is no endless feed.
        </p>
        <ul>
          <li>
            <b>UPSC edition:</b> India and world stories tagged by syllabus area (GS1 to GS4).
          </li>
          <li>
            <b>Engineering edition:</b> engineering blogs and AI news, summarised.
          </li>
          <li>
            <b>Every story links to its publisher.</b> Postpaper summarises and points you there.
          </li>
        </ul>
      </div>
      <SubscribeForm id="hero" />
    </section>
  );
}
