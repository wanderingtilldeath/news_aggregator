import SubscribeForm from "./subscribe-form";

// The signup, a dark band on the home page. The header's Subscribe button links here.
export default function Banner() {
  return (
    <section className="banner" id="subscribe" aria-labelledby="pitch-h">
      <div>
        <h2 id="pitch-h">One edition a day. Then you are done.</h2>
        <p>UPSC current affairs and engineering news, ranked, with a line on why each story matters. Free, in your inbox.</p>
      </div>
      <SubscribeForm id="hero" />
    </section>
  );
}
