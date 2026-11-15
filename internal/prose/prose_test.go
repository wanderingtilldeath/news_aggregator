package prose

import (
	"strings"
	"testing"
)

func TestLeadTakesWholeOpeningSentencesAndSkipsChrome(t *testing.T) {
	content := "Subscribe to our newsletter for more. LOGIN Bangladesh has completed the legal procedures required to secure the return of ousted Prime Minister Sheikh Hasina from India, the foreign ministry said on Sunday. " +
		"“We have completed everything required from our side as part of the legal process,” Prothom Alo quoted him as saying. " +
		"Short one. " +
		"Hasina was sentenced to death in absentia by a tribunal in Dhaka last year over a crackdown on student protests that left hundreds dead. " +
		"A fourth sentence that must not appear because the lead is capped at three sentences and seventy words."

	got := Lead(content, 3, 70)
	if !strings.HasPrefix(got, "Bangladesh has completed the legal procedures") || strings.Contains(got, "Subscribe") || strings.Contains(got, "Short one") {
		t.Fatalf("Lead = %q, want the article's own opening without chrome or short fragments", got)
	}
	if !strings.HasSuffix(got, "student protests that left hundreds dead.") || strings.Contains(got, "fourth") {
		t.Fatalf("Lead = %q, want three whole sentences ending at a full stop", got)
	}
	if n := len(strings.Fields(got)); n > 70 {
		t.Fatalf("%d words, want at most 70", n)
	}
}

func TestLeadKeepsAFirstSentenceWholeEvenWhenLong(t *testing.T) {
	first := "Long " + strings.Repeat("word ", 90) + "end."
	got := Lead(first+" Another sentence that is long enough to count as one.", 3, 70)
	if got != first {
		t.Fatalf("Lead = %q, want the long first sentence whole and nothing after it", got)
	}
	if Lead("", 3, 70) != "" || Lead("Too short.", 3, 70) != "" {
		t.Fatal("no usable sentence must give an empty lead")
	}
}

func TestSentencesAndFragments(t *testing.T) {
	got := Sentences("The leaders of the U.S. — and India — met Dr. Rao on Nov. 12. It went well. Yes.")
	want := []string{"The leaders of the U.S. — and India — met Dr. Rao on Nov. 12.", "It went well.", "Yes."}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("Sentences = %q, want %q", got, want)
	}
	if !Fragment("— are expected to meet.") || Fragment("The summit is in Manila.") {
		t.Fatal("Fragment must flag a dash-led tail and nothing else")
	}
}
