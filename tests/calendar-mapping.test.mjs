import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";

// Load the same script the wall includes, without any framework or build step.
const src = readFileSync(new URL("../web/static/calendar-mapping.js", import.meta.url), "utf8");
vm.runInThisContext(src + "\n//# sourceURL=calendar-mapping.js");
const M = globalThis.FamilyCalendarMapping;

const bindings = [
  { personId: "u-lukas", kind: "besteschule", externalId: "bs-lukas" },
  { personId: "u-lukas", kind: "timetable", externalId: "plan-42" },
  { personId: "u-hannah", kind: "timetable", externalId: "plan-01" },
];

const calendars = [
  { calendarId: "c-lukas", name: "Schule Lukas", panel: "school", order: 0, personIds: ["u-lukas"] },
  { calendarId: "c-hannah", name: "Schule Hannah", panel: "school", order: 1, personIds: ["u-hannah"] },
  { calendarId: "c-fam", name: "Familie", panel: "column", order: 2, personIds: ["u-lukas", "u-hannah"] },
];

test("renamed and reordered calendars still resolve by stable ids", () => {
  const renamed = [...calendars].reverse().map((c) => ({ ...c, name: c.name + " (neu)" }));
  assert.equal(M.personForSource(bindings, "timetable", "plan-01"), "u-hannah");
  assert.deepEqual(M.calendarIdsForPerson(renamed, "u-lukas"), ["c-fam", "c-lukas"]);
});

test("same-name children resolve by id, never by name", () => {
  const same = [
    ...bindings,
    { personId: "u-2", kind: "besteschule", externalId: "bs-2" },
  ];
  assert.equal(M.personForSource(same, "besteschule", "bs-lukas"), "u-lukas");
  assert.equal(M.personForSource(same, "besteschule", "bs-2"), "u-2");
});

test("missing binding yields no guessed match", () => {
  assert.equal(M.personForSource(bindings, "besteschule", "bs-unknown"), null);
  assert.equal(M.personForSource(bindings, "timetable", "Lukas Meister"), null);
  assert.equal(M.personForSource(bindings, "besteschule", "plan-42"), null);
  assert.equal(M.personForSource([], "timetable", "plan-01"), null);
  assert.deepEqual(M.calendarIdsForPerson(calendars, "u-unknown"), []);
});
