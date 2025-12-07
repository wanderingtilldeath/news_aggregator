const RSSParser = require("rss-parser");
const parser = new RSSParser();

const SOURCES = [
  // World News
  { name: "CNN", url: "http://rss.cnn.com/rss/edition.rss" },

  { name: "TechCrunch", url: "http://feeds.feedburner.com/TechCrunch/" },

  { name: "Times of India", url: "https://timesofindia.indiatimes.com/rssfeeds/-2128936835.cms" },
  { name: "Hindustan Times", url: "https://www.hindustantimes.com/feeds/rss/topnews/rssfeed.xml" },
];
function shuffle(array) {
  return array.sort(() => Math.random() - 0.5);
}


async function fetchAllNews() {
  const allNews = [];

  for (const source of SOURCES) {
    try {
      const feed = await parser.parseURL(source.url);
      const items = feed.items.slice(0, 3); // only 2 per source (fast)
      
      items.forEach((item) => {
        allNews.push({
          source: source.name,
          title: item.title,
          link: item.link,
          description: item.contentSnippet || item.summary || "",
          published: item.pubDate
        });
      });

    } catch (err) {
      console.error(`Error fetching ${source.name}:`, err.message);
    }
  }

  return shuffle(allNews).slice(0, 40);

}

module.exports = { fetchAllNews };
