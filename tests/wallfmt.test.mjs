// Pure text helpers of the clock tile. Run: node tests/wallfmt.test.mjs
import { readFileSync } from "node:fs";
import assert from "node:assert/strict";

const src = readFileSync(new URL("../web/static/wallfmt.js", import.meta.url), "utf8");
const { rainOutlook, wasteSentence, inkOn, kidColor } = new Function(`${src}; return { rainOutlook, wasteSentence, inkOn, kidColor };`)();

const H = (probs) => probs.map((p, i) => ({ time: new Date(2026, 8, 30, i).toISOString(), precipProb: p }));
const now = new Date(2026, 8, 30, 13, 18);
const flat = (n, v) => Array(n).fill(v);
const withRain = (from, to, p, base = 10) => H(Array.from({ length: 24 }, (_, h) => (h >= from && h <= to ? p : base)));

assert.equal(rainOutlook(H(flat(24, 10)), now), "");
assert.equal(rainOutlook(withRain(16, 19, 60), now), "ab 16 Uhr Regen");
assert.equal(rainOutlook(withRain(12, 14, 70), now), "Regen bis 15 Uhr");
assert.equal(rainOutlook(withRain(12, 23, 70), now), "Regen bis in die Nacht");
assert.equal(rainOutlook(withRain(8, 10, 70), now), "");
assert.equal(rainOutlook(withRain(16, 16, 50), now), "ab 16 Uhr Regen");
assert.equal(rainOutlook(withRain(16, 16, 49), now), "");
assert.equal(rainOutlook([], now), "");
assert.equal(rainOutlook(undefined, now), "");

assert.equal(wasteSentence("Gelbe Tonne", "tomorrow", false), "Gelbe Tonne heute Abend rausstellen");
assert.equal(wasteSentence("Gelbe Tonne", "today", false), "Gelbe Tonne wird heute abgeholt");
assert.equal(wasteSentence("Gelbe Tonne", "tomorrow", true), "Gelbe Tonne heute Abend rausstellen (verlegt)");
for (const c of ["#2460D8", "#D43F7A", "#7A4BD6", "#14211B", "#e11d48", "#16a34a", "", undefined, "var(--accent)"]) assert.equal(inkOn(c), "#FFFFFF", String(c));
for (const c of ["#F2B705", "#FFE9A8", "#E8EDE4", "#fff"]) assert.equal(inkOn(c), "#14211B", c);

const people = [{ id: "p1", color: "#2460D8" }, { id: "p2" }];
const k = (o) => kidColor({ central: true, people, personId: "p1", calColor: "#F2B705", mealColor: "#16a34a", ...o });
assert.equal(k({}), "#2460D8"); // person colour wins in central mode
assert.equal(k({ central: false }), "#F2B705"); // local mode ignores people
assert.equal(k({ personId: "p2" }), "#F2B705"); // person without colour
assert.equal(k({ personId: "nobody" }), "#F2B705"); // unknown person
assert.equal(k({ personId: undefined }), "#F2B705");
assert.equal(k({ people: undefined }), "#F2B705");
assert.equal(k({ calColor: "" }), "#2460D8");
assert.equal(k({ central: false, calColor: undefined }), "#16a34a"); // meal colour
assert.equal(k({ central: false, calColor: undefined, mealColor: "" }), "var(--text)"); // fallback
console.log("wallfmt.test.mjs ok");
