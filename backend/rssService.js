const RSSParser = require("rss-parser");
const parser = new RSSParser();

const SOURCES = [
  { name: "CNN", url: "http://rss.cnn.com/rss/edition.rss" },
  { name: "TechCrunch", url: "http://feeds.feedburner.com/TechCrunch/" },
  { name: "TOI", url: "https://timesofindia.indiatimes.com/rssfeeds/-2128936835.cms" },
  { name: "Hindustan Times", url: "https://www.hindustantimes.com/feeds/rss/topnews/rssfeed.xml" }
];

function shuffle(arr) {
  return arr.sort(() => Math.random() - 0.5);
}

async function fetchAllNews() {
  // Fetch all RSS feeds concurrently
  const fetchJobs = SOURCES.map(async src => {
    try {
      const feed = await parser.parseURL(src.url);
      return feed.items.slice(0, 10).map(item => ({
        source: src.name,
        title: item.title,
        link: item.link,
        description: item.contentSnippet || item.summary || "",
        published: item.pubDate
      }));
    } catch (err) {
      console.log(`RSS error [${src.name}]:`, err.message);
      return [];  // fallback — avoid breaking rest
    }
  });

  // Wait for all feeds to complete
  const results = await Promise.all(fetchJobs);

  // Flatten all article arrays
  const allNews = results.flat();

  return shuffle(allNews).slice(0, 40);
}

module.exports = { fetchAllNews };
