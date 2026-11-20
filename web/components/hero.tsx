import SubscribeForm from "./subscribe-form";

// The signup, a dark band on the home page. The header's Subscribe button links here.
export default function Banner() {
  return (
    <section className="banner" id="subscribe" aria-labelledby="pitch-h">
      <div>
        <h2 id="pitch-h">The whole day&rsquo;s news, in five minutes.</h2>
        <p>One short briefing in your inbox every morning: the stories that matter, ranked, each with a line on why it matters. Then you are done. Free, no spam.</p>
      </div>
      <SubscribeForm id="hero" />
    </section>
  );
}
