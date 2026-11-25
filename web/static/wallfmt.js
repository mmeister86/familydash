// Pure text helpers for the clock tile (no DOM; loaded before app.js).

const RAIN_MIN_PROB = 50; // percent

// One-line rain outlook for the rest of today. hours: [{time: ISO, precipProb}].
// "" when no rain is likely, "Regen bis 15 Uhr" when it rains now,
// "ab 16 Uhr Regen" when it starts later.
function rainOutlook(hours, now) {
  if (!Array.isArray(hours) || !hours.length) return "";
  const day = now.toDateString(), from = now.getHours();
  const byHour = new Map();
  for (const h of hours) {
    const t = new Date(h.time);
    if (t.toDateString() === day && t.getHours() >= from) byHour.set(t.getHours(), h.precipProb >= RAIN_MIN_PROB);
  }
  let start = -1;
  for (let h = from; h < 24; h++) if (byHour.get(h)) { start = h; break; }
  if (start < 0) return "";
  if (start > from) return `ab ${start} Uhr Regen`;
  let end = start;
  while (end < 24 && byHour.get(end)) end++;
  return end >= 24 ? "Regen bis in die Nacht" : `Regen bis ${end} Uhr`;
}

// state: "today" (collected today) | "tomorrow" (put it out tonight)
function wasteSentence(name, state, moved) {
  const s = state === "today" ? `${name} wird heute abgeholt` : `${name} heute Abend rausstellen`;
  return moved ? `${s} (verlegt)` : s;
}
