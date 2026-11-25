// Pure text helpers of the clock tile. Run: node tests/wallfmt.test.mjs
import { readFileSync } from "node:fs";
import assert from "node:assert/strict";

const src = readFileSync(new URL("../web/static/wallfmt.js", import.meta.url), "utf8");
const { rainOutlook, wasteSentence } = new Function(`${src}; return { rainOutlook, wasteSentence };`)();

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
console.log("wallfmt.test.mjs ok");
