require("dotenv").config();

const express = require("express");
const { fetchAllNews } = require("./rssService");
const { summarizeText } = require("./summaryService");

const axios = require("axios");
const cors = require("cors");

const app = express();
app.use(cors());
const PORT = process.env.PORT || 3000;


/**
 * IMAGE EXTRACTION
 */
app.get("/extract-image", async (req, res) => {
  const articleUrl = req.query.url;
  if (!articleUrl) return res.status(400).json({ error: "Missing URL" });

  try {
    const apiUrl = `https://api.microlink.io?url=${encodeURIComponent(articleUrl)}`;

    const response = await axios.get(apiUrl, { timeout: 1000 }); // ⚡ 1 second timeout

    const image =
      response.data?.data?.image?.url ||
      response.data?.data?.logo?.url ||
      null;

    res.json({ image });
  } catch (error) {
    console.log("Microlink slow — using fallback");
    return res.json({ image: null });
  }
});


/**
 * RAW NEWS — always fresh
 */
app.get("/news", async (req, res) => {
  console.log("Fetching FRESH RAW news...");
  const news = await fetchAllNews();

  res.json({
    fresh: true,
    articles: news
  });
});

/**
 * SUMMARIZED NEWS — always fresh
 */
app.get("/news/summarized", async (req, res) => {
  console.log("Fetching fresh summarized news...");

  const news = await fetchAllNews();
  const limited = news.slice(0, 40);

  const summaries = await Promise.all(
    limited.map(item =>
      summarizeText(`${item.title}\n${item.description || ""}`)
    )
  );

  const summarizedArticles = limited.map((item, i) => ({
    ...item,
    summary: summaries[i]
  }));

  res.json({
    fresh: true,
    all_articles: summarizedArticles   // IMPORTANT FIX
  });
});




app.listen(PORT, () => {
  console.log(`Server running on http://localhost:${PORT}`);
});
