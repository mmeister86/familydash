// Pure helpers for the clock tile and the children's cards (no DOM; loaded before app.js).

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

// Text colour for a name plate filled with `hex`: dark ink on light fills
// (WCAG relative luminance > 0.4), white otherwise or when unparsable.
function inkOn(hex) {
  let m = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(String(hex || "").trim());
  if (!m) return "#FFFFFF";
  let h = m[1];
  if (h.length === 3) h = h.replace(/./g, (c) => c + c);
  const lin = [0, 2, 4].map((i) => {
    const v = parseInt(h.slice(i, i + 2), 16) / 255;
    return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * lin[0] + 0.7152 * lin[1] + 0.0722 * lin[2] > 0.4 ? "#14211B" : "#FFFFFF";
}

// Colour of a child's card: in central mode the person's own colour wins,
// then the calendar's, then the lunch account's, else the ink colour.
function kidColor({ central, people, personId, calColor, mealColor }) {
  const person = central && personId ? (people || []).find((p) => p.id === personId) : null;
  return person?.color || calColor || mealColor || "var(--text)";
}
