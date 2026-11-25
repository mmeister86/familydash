// Theme foundation: fonts, palette tokens and the no-uppercase-labels rule.
import { readFileSync, statSync } from "node:fs";
import assert from "node:assert/strict";

const dir = new URL("../web/static/", import.meta.url);
const css = readFileSync(new URL("style.css", dir), "utf8");

const fonts = ["sofia-sans", "sofia-sans-condensed", "sofia-sans-extra-condensed"];
assert.equal((css.match(/@font-face/g) || []).length, 3, "three @font-face blocks");
for (const f of fonts) {
  assert.ok(css.includes(`url(fonts/${f}.woff2)`), `font-face for ${f}`);
  assert.ok(statSync(new URL(`fonts/${f}.woff2`, dir)).size > 20000, `${f}.woff2 exists and is real`);
}

assert.ok(css.includes('html[data-palette="board"]'), "board palette selector");
const lower = css.toLowerCase();
const paper = ["#E6ECE5", "#FAFCF8", "#14211B", "#55645B", "#93A097", "#CDD7CE", "#C4372C", "#8F5B00", "#2F7FD1", "#E0A000", "#2E9E6B", "#FFE9A8", "#4A3600"];
const board = ["#101D17", "#192A22", "#E8EDE4", "#9BAA9F", "#62746A", "#2A3F34", "#FF7B6B", "#F2B705", "#6FB2F2", "#46C28E", "#4A3B0C", "#FFE39A"];
for (const v of [...paper, ...board]) assert.ok(lower.includes(v.toLowerCase()), `palette value ${v}`);

const start = css.indexOf("/* night:start */");
const end = css.indexOf("/* night:end */");
assert.ok(start >= 0 && end > start, "night markers");
const outside = css.slice(0, start) + css.slice(end);
assert.ok(!/text-transform:\s*uppercase/.test(outside), "no uppercase labels outside night");
for (const m of outside.matchAll(/letter-spacing:\s*(-?[\d.]+)em/g)) {
  assert.ok(parseFloat(m[1]) <= 0.03, `letter-spacing ${m[1]}em too large`);
}

for (const old of ["#0E1116", "#161B22", "#4F8EF7"]) assert.ok(!lower.includes(old.toLowerCase()), `old value ${old} gone`);
console.log("theme.test.mjs ok");
