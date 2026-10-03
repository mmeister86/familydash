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

// ---- renderSchoolRow central branch (the real wall script, stubbed DOM) ----
// A school calendar named exactly like the child must not merge into the
// child's card through name matching when the central snapshot has no
// binding for the child — especially when the mapping helper itself failed
// to load (FamilyCalendarMapping missing).

const appSrc = readFileSync(new URL("../web/static/app.js", import.meta.url), "utf8");
const mappingSrc = readFileSync(new URL("../web/static/calendar-mapping.js", import.meta.url), "utf8");

function loadWall({ withMapping }) {
  const elements = new Map();
  const element = () => ({ innerHTML: "", textContent: "", classList: { toggle() {} } });
  const sandbox = {
    console,
    URLSearchParams,
    document: {
      body: element(),
      getElementById: (id) => {
        if (!elements.has(id)) elements.set(id, element());
        return elements.get(id);
      },
    },
    location: { search: "" },
    fetch: () => Promise.reject(new Error("offline")),
    setInterval: () => 0,
    clearInterval: () => {},
    setTimeout: () => 0,
    clearTimeout: () => {},
  };
  vm.createContext(sandbox);
  if (withMapping) vm.runInContext(mappingSrc, sandbox, { filename: "calendar-mapping.js" });
  vm.runInContext(appSrc, sandbox, { filename: "app.js" });
  return { sandbox, html: (id) => elements.get(id)?.innerHTML ?? "" };
}

// Same-name school calendar, but the child has no binding to any person.
function renderData({ central }) {
  return {
    calendar: {
      ...(central ? { central: true } : {}),
      calendars: [
        { id: 0, calendarId: "c-schule", name: "Lukas", color: "#123456", panel: "school", personIds: ["u-other"] },
      ],
      events: [],
      bindings: [],
    },
    school: { students: [{ name: "Lukas", id: "bs-lukas", day: { date: "2026-10-03", lessons: [] } }] },
    timetables: [],
    meals: { children: [] },
    todos: { tasks: [] },
  };
}

test("central mode without mapping helper attaches no calendar (treat as unmapped)", () => {
  const { sandbox, html } = loadWall({ withMapping: false });
  assert.equal(vm.runInContext("typeof FamilyCalendarMapping", sandbox), "undefined");
  sandbox.renderSchoolRow(renderData({ central: true }), "full");
  const out = html("school");
  assert.match(out, /panel fade/, "same-name calendar stays a standalone card");
  assert.doesNotMatch(out, /Termine/, "no name-matched calendar section in the child card");
});

test("local mode keeps the legacy name match", () => {
  const { sandbox, html } = loadWall({ withMapping: false });
  sandbox.renderSchoolRow(renderData({ central: false }), "full");
  const out = html("school");
  assert.match(out, /Termine/, "same-name calendar merges into the child card");
  assert.doesNotMatch(out, /panel fade/, "no standalone card left behind");
});

test("central mode with mapping helper and binding attaches by stable id", () => {
  const { sandbox, html } = loadWall({ withMapping: true });
  const data = renderData({ central: true });
  data.calendar.calendars[0].personIds = ["u-lukas"];
  data.calendar.bindings = [{ personId: "u-lukas", kind: "besteschule", externalId: "bs-lukas" }];
  sandbox.renderSchoolRow(data, "full");
  const out = html("school");
  assert.match(out, /Termine/, "bound calendar merges into the child card");
  assert.doesNotMatch(out, /panel fade/, "no standalone card left behind");
});
