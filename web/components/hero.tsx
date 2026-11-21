import SubscribeForm from "./subscribe-form";

// The signup, a dark band on the home page. The header's Subscribe button links here.
export default function Banner() {
  return (
    <section className="banner" id="subscribe" aria-labelledby="pitch-h">
      <div>
        <h2 id="pitch-h">All the news, none of the noise.</h2>
        <p>
          One email each morning: the top stories, ranked, with a plain line on why each one matters. Read it in a minute, or take the longer version. UPSC and engineering editions too. <b className="free">Free, no spam.</b>
        </p>
      </div>
      <SubscribeForm id="hero" />
    </section>
  );
}
