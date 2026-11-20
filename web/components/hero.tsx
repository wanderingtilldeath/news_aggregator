import SubscribeForm from "./subscribe-form";

// The signup, a dark band on the home page. The header's Subscribe button links here.
export default function Banner() {
  return (
    <section className="banner" id="subscribe" aria-labelledby="pitch-h">
      <div>
        <h2 id="pitch-h">All the news, none of the noise.</h2>
        <p>
          We read the day&rsquo;s news so you don&rsquo;t have to. You get the top stories, ranked, with a plain line on why each one matters. <b className="free">Free, no spam.</b>
        </p>
      </div>
      <SubscribeForm id="hero" />
    </section>
  );
}
