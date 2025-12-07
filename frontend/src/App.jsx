import { useState } from "react";
import "./styles.css";

function App() {
  const [articles, setArticles] = useState([]);
  const [loading, setLoading] = useState(false);

  function convertToBullets(text) {
    if (!text) return [];

    return text
      .split(/[\n\r]*[*•][\s]*/g)
      .map((t) => t.trim())
      .filter(
        (t) =>
          t.length > 3 &&
          !t.toLowerCase().startsWith("here are") &&
          !t.toLowerCase().startsWith("summary") &&
          !t.toLowerCase().startsWith("the news")
      );
  }

  async function fetchFreshSummaries() {
    setLoading(true);

    try {
      const res = await fetch("https://news-aggregator-backend-p5ft.onrender.com/news/summarized");
      const data = await res.json();

      const list = data.summarized_articles || [];

      const withImages = await Promise.all(
        list.map(async (item) => {
          try {
            const metaRes = await fetch(
              `https://news-aggregator-backend-p5ft.onrender.com/extract-image?url=${encodeURIComponent(
                item.link
              )}`
            );
            const meta = await metaRes.json();

            return {
              ...item,
              image:
                meta.image || "https://placehold.co/800x500?text=No+Image",
            };
          } catch (e) {
            return {
              ...item,
              image: "https://placehold.co/800x500?text=No+Image",
            };
          }
        })
      );

      setArticles(withImages);
    } catch (err) {
      console.error("Error fetching news:", err);
    }

    setLoading(false);
  }

  return (
    <div>
      <h1>AI News Aggregator</h1>

      {/* ONE BUTTON ONLY */}
      <button onClick={fetchFreshSummaries}>
        {loading ? "Loading Fresh AI Summaries..." : "Load Fresh AI News"}
      </button>

      {loading && (
        <>
          <div className="shimmer"></div>
          <div className="shimmer"></div>
          <div className="shimmer"></div>
        </>
      )}

      <div className="news-container">
        {articles.map((item, i) => (
          <div className="news-card" key={i}>
            <img src={item.image} className="thumb" />

            <div className="content">
              <div className="news-title">{item.title}</div>
              <div className="news-source">Source: {item.source}</div>

              {/* AI Summary */}
              <div className="summary-box">
                <div className="summary-title">AI Summary</div>
                <ul className="summary-list">
                  {convertToBullets(item.summary).map((point, index) => (
                    <li key={index}>{point}</li>
                  ))}
                </ul>
              </div>

              <br />
              <a href={item.link} target="_blank">
                Read full article →
              </a>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

export default App;
