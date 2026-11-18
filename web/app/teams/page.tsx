import type { Metadata } from "next";
import Link from "next/link";
import TeamForm from "@/components/team-form";

export const metadata: Metadata = {
  title: "Postpaper for Teams · Postpaper",
  description: "The daily edition posted to your team's Slack or Discord channel, free.",
};

export default function Page() {
  return (
    <main className="page" id="view">
      <h1>Postpaper for Teams</h1>
      <p className="lead-in">The daily edition, posted to your team&rsquo;s Slack or Discord channel once a morning: five stories, each with a line on why it matters, and a link to the quiz. Free.</p>

      <h2>Set it up</h2>
      <ol className="plain steps">
        <li>
          <b>Slack:</b> create an app at <a href="https://api.slack.com/apps" target="_blank" rel="noopener noreferrer">api.slack.com/apps</a>, turn on <i>Incoming Webhooks</i> and add one to your channel.
          <br />
          <b>Discord:</b> open the channel&rsquo;s settings, choose <i>Integrations</i>, then <i>Webhooks</i>, and create one.
        </li>
        <li>Copy the webhook address and paste it below, then choose the edition.</li>
        <li>We post a test message at once. If it arrives, you are set; tomorrow&rsquo;s edition follows.</li>
      </ol>
      <TeamForm />

      <h2>What we keep</h2>
      <ul className="plain">
        <li>Only the webhook address and the edition you chose. The address is a secret: we never show it again, and only Slack and Discord webhook addresses are accepted.</li>
        <li>Every post ends with a link that stops them. Deleting the webhook in Slack or Discord stops them too.</li>
      </ul>
      <p>
        <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}
