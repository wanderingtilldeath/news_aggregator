require("dotenv").config();

const express = require("express");
const { fetchAllNews } = require("./rssService");
const { summarizeText } = require("./summaryService");

const axios = require("axios");
const cheerio = require("cheerio");
const cors = require("cors");

const app = express();
const PORT = process.env.PORT || 3000;

app.use(cors({ origin: "*" }));

/* ===========================================
   IMAGE EXTRACTION (FAST + 3 FALLBACKS)
   =========================================== */
app.get("/extract-image", async (req, res) => {
  const url = req.query.url;
  if (!url) return res.json({ image: null });

  // -------- First Try — Microlink --------
  try {
    const apiUrl = `https://api.microlink.io?url=${encodeURIComponent(url)}`;
    const response = await axios.get(apiUrl, { timeout: 2500 });

    const microlinkImg =
      response.data?.data?.image?.url ||
      response.data?.data?.logo?.url ||
      null;

    if (microlinkImg) return res.json({ image: microlinkImg });

  } catch (e) {}

  // -------- Second Try — Scrape OG/Twitter --------
  try {
    const html = await axios.get(url, { timeout: 2500 }).then(r => r.data);
    const $ = cheerio.load(html);

    const ogImg = $('meta[property="og:image"]').attr("content");
    const twImg = $('meta[name="twitter:image"]').attr("content");

    if (ogImg) return res.json({ image: ogImg });
    if (twImg) return res.json({ image: twImg });
  } catch (e) {}

  // -------- Third Try — Favicon fallback --------
  try {
    const origin = new URL(url).origin;
    return res.json({ image: `${origin}/favicon.ico` });
  } catch (e) {
    return res.json({ image: null });
  }
});

/* ===========================================
   QUICK SUMMARIZER (FAST)
   =========================================== */
app.get("/quick-summarize", async (req, res) => {
  const text = req.query.text || "";
  try {
    const summary = await summarizeText(text);
    res.send(summary);
  } catch {
    res.send("• Summary unavailable");
  }
});

/* ===========================================
   RAW NEWS
   =========================================== */
app.get("/news", async (req, res) => {
  const news = await fetchAllNews();
  res.json({ fresh: true, articles: news });
});

/* ===========================================
   SUMMARIZED NEWS (TOP 10 ONLY)
   =========================================== */
app.get("/news/summarized", async (req, res) => {
  const news = await fetchAllNews();

  const top10 = news.slice(0, 10);
  const rest30 = news.slice(10, 40);

  // FAST SUMMARY FOR FIRST 10
  const summaries = await Promise.all(
    top10.map(item => summarizeText(`${item.title}\n${item.description || ""}`))
  );

  const summarizedTop = top10.map((item, i) => ({
    ...item,
    summary: summaries[i]
  }));

  res.json({
    fresh: true,
    top_articles: summarizedTop,
    remaining_articles: rest30
  });
});

// Debug
app.get("/debug/news-count", async (req, res) => {
  const news = await fetchAllNews();
  res.send("Total articles fetched: " + news.length);
});

app.get("/", (req, res) => res.send("Backend running ✔"));

app.listen(PORT, () =>
  console.log(`Server running on http://localhost:${PORT}`)
);
