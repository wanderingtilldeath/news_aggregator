const Groq = require("groq-sdk");

const groq = new Groq({
  apiKey: process.env.GROQ_API_KEY
});

// ✨ Reusable prompt
const SUMMARY_PROMPT = `
You will ALWAYS output EXACTLY 3 bullet points.
Rules:
- No apologies.
- No filler like “unfortunately”.
- If text is incomplete, summarize what is available.
- Bullets must start with "•".

Summarize this:
`;

async function runSummary(text) {
  return groq.chat.completions.create({
    model: process.env.SUMMARY_MODEL || "llama-3.1-8b-instant",
    messages: [
      { role: "user", content: SUMMARY_PROMPT + text }
    ],
    max_tokens: 90,
    temperature: 0.2
  });
}

// ✨ Parallel-safe summarizer with retry
async function summarizeText(text) {
  try {
    const result = await runSummary(text);
    return result.choices[0].message.content.trim();
  } catch (err) {
    console.log("Groq summary failed once → retrying…", err.message);
    try {
      const retry = await runSummary(text);
      return retry.choices[0].message.content.trim();
    } catch (e2) {
      console.log("Groq summary failed twice → fallback");
      return `• Summary unavailable\n• Try again later\n• Source temporarily down`;
    }
  }
}

module.exports = { summarizeText };
