// Run with: npm test (plain Node, no framework).
import assert from "node:assert/strict";
import { keyNumbers } from "./numbers.ts";

const figures = (text: string) => keyNumbers(text).map((k) => k.figure);

// Money in the forms the sources use.
assert.deepEqual(figures("Zeta raised Rs 1,905 Cr in Series C, and Botsync raised $11 Mn."), ["Rs 1,905 Cr", "$11 Mn"]);
assert.deepEqual(figures("The outlay is ₹2.5 lakh crore over five years."), ["₹2.5 lakh crore"]);
assert.deepEqual(figures("Prices of €40 million and £3 billion were quoted."), ["€40 million", "£3 billion"]);

// Percentages in the three spellings.
assert.deepEqual(figures("The repo rate stays at 5.5%, inflation fell to 4 per cent and growth to 6.8 percent."), ["5.5%", "4 per cent", "6.8 percent"]);

// Counted things carry their unit; a bare number or a year is not a figure.
assert.deepEqual(figures("About 12 lakh people were moved and 340 villages were hit in 2026, with 5 ministers present."), ["12 lakh people", "340 villages"]);
assert.deepEqual(figures("In 2026 the 3 judges met on 14 December."), []);

// A label is the few words after the figure, cut at punctuation, never ending on a filler word.
assert.deepEqual(keyNumbers("The repo rate stays at 5.5% against the Monetary Policy Committee's forecast, said the governor."), [{ figure: "5.5%", label: "against the Monetary Policy" }]);
assert.deepEqual(keyNumbers("Inflation fell to 4% in October.")[0], { figure: "4%", label: "in October" });
assert.deepEqual(keyNumbers("Inflation fell to 4% in the.")[0], { figure: "4%", label: "" }); // only filler words follow: nothing to show
assert.deepEqual(keyNumbers("Growth of 6.8% year on year, the best in a decade.")[0], { figure: "6.8%", label: "year on year" });
assert.equal(keyNumbers("A sum of Rs 50 Cr.")[0].label, "");
assert.equal(keyNumbers("It hit 12 lakh people in the north.")[0].label, ""); // a counted figure already holds its unit

// At most four, each once whatever the case, in order of appearance.
assert.deepEqual(figures("5% and 6% and 5% and 7% and 8% and 9%"), ["5%", "6%", "7%", "8%"]);
assert.deepEqual(figures("Rs 5 Cr then RS 5 CR again."), ["Rs 5 Cr"]);
assert.deepEqual(figures(""), []);

console.log("numbers: ok");
