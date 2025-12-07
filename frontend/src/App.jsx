import { useState } from "react";
import "./styles.css";

const API = "https://news-aggregator-backend-p5ft.onrender.com";

function App() {
  const [articles, setArticles] = useState([]);
  const [allArticles, setAllArticles] = useState([]);
  const [visibleCount, setVisibleCount] = useState(10);

  const [loading, setLoading] = useState(false);
  const [showTimer, setShowTimer] = useState(false);
  const [counter, setCounter] = useState(15);

  // NEW FLAG — tracks background loading
  const [isBackgroundLoading, setIsBackgroundLoading] = useState(false);

  function convertToBullets(text) {
    if (!text) return [];
    return text
      .split(/[\n\r]*[*•][\s]*/g)
      .map(t => t.trim())
      .filter(t => t.length > 3);
  }

  async function fetchFreshSummaries() {
    setLoading(true);
    setShowTimer(true);
    setCounter(20);

    let timer = setInterval(() => {
      setCounter((prev) => {
        if (prev <= 1) {
          clearInterval(timer);
          return 0;
        }
        return prev - 1;
      });
    }, 1000);

    try {
      const res = await fetch(`${API}/news/summarized?t=${Date.now()}`);
      const data = await res.json();

      const top = data.top_articles || [];
      const rest = data.remaining_articles || [];

      // 1) GET IMAGES FOR TOP 10
      const topWithImages = await Promise.all(
        top.map(async (item) => {
          const metaRes = await fetch(
            `${API}/extract-image?url=${encodeURIComponent(item.link)}`
          );
          const meta = await metaRes.json();

          return {
            ...item,
            image: meta.image || "https://placehold.co/600x400?text=No+Image",
          };
        })
      );

      // 2) SHOW FIRST 10
      setArticles(topWithImages);
      setAllArticles(topWithImages);

      // 3) START BACKGROUND PROCESS
      setIsBackgroundLoading(true);   // ← IMPORTANT
      backgroundProcessRemaining(rest);

    } catch (e) {
      console.error(e);
    }

    setShowTimer(false);
    setLoading(false);
  }

 async function backgroundProcessRemaining(rest) {
  setIsBackgroundLoading(true);

  const BATCH_SIZE = 5; // PROCESS 5 ARTICLES IN PARALLEL

  for (let i = 0; i < rest.length; i += BATCH_SIZE) {
    const batch = rest.slice(i, i + BATCH_SIZE);

    // PROCESS 5 ARTICLES TOGETHER (PARALLEL)
    const results = await Promise.all(
      batch.map(async (item) => {
        let summary = "• Summary unavailable";
        let image = "https://placehold.co/600x400?text=No+Image";

        try {
          summary = await fetch(
            `${API}/quick-summarize?text=` +
              encodeURIComponent(item.title + "\n" + (item.description || ""))
          ).then((res) => res.text());
        } catch {}

        try {
          const imgRes = await fetch(
            `${API}/extract-image?url=${encodeURIComponent(item.link)}`
          );
          const meta = await imgRes.json();
          if (meta.image) image = meta.image;
        } catch {}

        return { ...item, summary, image };
      })
    );

    // ADD THE 5 PROCESSED ARTICLES TO UI
    setAllArticles((prev) => [...prev, ...results]);
  }

  setIsBackgroundLoading(false);
}



  return (
    <div>
      <h1>AI News Aggregator</h1>

      <button onClick={fetchFreshSummaries}>
        {loading ? "Loading..." : "Load Fresh News"}
      </button>

      {showTimer && (
        <div className="waiting-box">
          <p>Please wait… Fetching fresh news</p>
          <p className="timer">{counter}s</p>
        </div>
      )}

      <div className="news-container">
        {articles.map((item, i) => (
          <div className="news-card" key={i}>
            <img src={item.image} className="thumb" />
            <div className="content">
              <div className="news-title">{item.title}</div>
              <div className="news-source">Source: {item.source}</div>

              <div className="summary-box">
                <div className="summary-title">AI Summary</div>
                <ul className="summary-list">
                  {convertToBullets(item.summary).map((p, idx) => (
                    <li key={idx}>{p}</li>
                  ))}
                </ul>
              </div>

              <a href={item.link} target="_blank">
                Read full article →
              </a>
            </div>
          </div>
        ))}
      </div>

      {articles.length > 0 && (
        <button
          onClick={() => {
            const next = visibleCount + 10;

            if (allArticles.length < next && isBackgroundLoading) {
              // New timer for background wait
              setShowTimer(true);
              setCounter(6);

              let timer = setInterval(() => {
                setCounter((prev) => {
                  if (prev <= 1) {
                    clearInterval(timer);
                    setShowTimer(false);
                    return 0;
                  }
                  return prev - 1;
                });
              }, 1000);

              return;
            }

            // Load next available set
            setVisibleCount(next);
            setArticles(allArticles.slice(0, next));
          }}
        >
          {isBackgroundLoading ? "Fetching…" : "Load More"}
        </button>
      )}

    </div>
  );
}

export default App;
