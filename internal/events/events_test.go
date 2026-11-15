package events

import (
	"strings"
	"testing"
	"time"
)

var pub = time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)

func days(found []Found) []string {
	var out []string
	for _, f := range found {
		out = append(out, f.Day.Format("2006-01-02"))
	}

	return out
}

func TestExtractFindsUpcomingDatesInEveryCommonForm(t *testing.T) {
	text := "The Monetary Policy Committee will announce its decision on 4 December after a three-day meeting. " +
		"The climate summit is scheduled to close on November 20, 2026 with a final text. " +
		"Applications for the exam open from 1st Jan and close on 15 Jan. " +
		"Nothing else happens in this plain sentence with no dates in it at all."
	got := Extract(text, pub, 90)
	want := []string{"2026-12-04", "2026-11-20", "2027-01-01"}
	if strings.Join(days(got), ",") != strings.Join(want, ",") {
		t.Fatalf("days = %v, want %v (three per article at most)", days(got), want)
	}
	if !strings.HasPrefix(got[0].Text, "The Monetary Policy Committee will announce") || !strings.HasSuffix(got[0].Text, "meeting.") {
		t.Fatalf("text = %q, want the whole sentence", got[0].Text)
	}
}

func TestExtractLeavesOutPastDatesNamesAndWordsThatLookLikeMonths(t *testing.T) {
	text := "The attack happened on 12 November and the trial began on 3 March last year, officials said in a statement. " +
		"The leader, who was born on 20 December, is known for speeches that may 10 times exceed an hour in length. " +
		"The police said the march 5 km long would be held on 31 February according to the notice." +
		"The report was filed on 14 June 2025 by the committee after months of hearings and visits."
	if got := Extract(text, pub, 90); len(got) != 0 {
		t.Fatalf("Extract = %+v, want nothing: past dates, a birthday, 'may 10', 'march 5', an impossible day", got)
	}
}

func TestExtractResolvesTheYearAndKeepsTheHorizon(t *testing.T) {
	late := time.Date(2026, 12, 20, 9, 0, 0, 0, time.UTC)
	got := Extract("The new rules take effect on 5 January for every account holder in the country.", late, 90)
	if len(got) != 1 || got[0].Day.Format("2006-01-02") != "2027-01-05" {
		t.Fatalf("Extract = %+v, want 5 January 2027 (the next one after 20 December)", got)
	}
	if got := Extract("The deadline for the scheme is 30 June for all applicants who have registered.", pub, 90); len(got) != 0 {
		t.Fatalf("a date 7 months out must be outside a 90 day horizon: %+v", got)
	}
	// The day of publication counts.
	if got := Extract("The vote takes place on 15 November in the lower house of parliament today.", pub, 90); len(got) != 1 {
		t.Fatalf("today's date was dropped: %+v", got)
	}
}

func TestExtractCutsALongSentenceAtAWord(t *testing.T) {
	long := "The committee, which has met repeatedly over many months to consider a very long list of proposals from states and ministries and industry bodies, will report on 4 December " + strings.Repeat("with many details of the work behind it ", 8) + "in full."
	got := Extract(long, pub, 90)
	if len(got) != 1 || len(got[0].Text) > maxText+4 || !strings.HasSuffix(got[0].Text, "…") {
		t.Fatalf("Extract = %+v, want one event cut at a word with an ellipsis", got)
	}
}

func TestExtractSkipsARunOfNumberedListItems(t *testing.T) {
	list := "01 The Jadeja connection: How Jamnagar shaped Mohit Ulva, India U-19's unlikely hero 02 Step inside Varun Dhawan's 4BHK Juhu home 03 Mohit Ulva will play again on 20 November."
	if got := Extract(list, pub, 90); len(got) != 0 {
		t.Fatalf("a page of list items gave %v, want none", got)
	}
	real := "The committee will meet on 4 December and again on 20 December to decide on the rate."
	if !Plausible(real) || len(Extract(real, pub, 90)) == 0 {
		t.Fatal("a sentence with two dates was dropped as a list")
	}
}

func TestExtractSkipsAnAuthorLine(t *testing.T) {
	by := "By Ana Carmo 15 November 2026 Peace and Security Nobel Peace Prize laureate Nadia Murad has spent years telling the world about the atrocities."
	if got := Extract(by, pub, 90); len(got) != 0 {
		t.Fatalf("a byline gave %v, want none", got)
	}
}
