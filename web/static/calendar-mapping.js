"use strict";

// Pure person/calendar mapping helpers for central (Convex) snapshots.
// Everything resolves through explicit stable ids: personForSource answers
// which person a source child belongs to via the binding (kind, externalId),
// calendarIdsForPerson answers which stable calendars belong to a person.
// Names and array positions never decide — a rename or reorder keeps every
// mapping, and a missing binding yields null/[] instead of a name guess.
// No framework, no build step; Node tests load this same file.
(function () {
  function personForSource(bindings, kind, externalId) {
    if (!Array.isArray(bindings) || !kind || !externalId) return null;
    for (const b of bindings) {
      if (b && b.kind === kind && b.externalId === externalId) {
        return b.personId || null;
      }
    }
    return null;
  }

  function stableId(c) {
    if (!c) return undefined;
    return c.calendarId !== undefined && c.calendarId !== "" ? c.calendarId : c.id;
  }

  function calendarIdsForPerson(calendars, personId) {
    if (!Array.isArray(calendars) || !personId) return [];
    const out = [];
    for (const c of calendars) {
      if (c && Array.isArray(c.personIds) && c.personIds.indexOf(personId) !== -1) {
        out.push(stableId(c));
      }
    }
    return out;
  }

  globalThis.FamilyCalendarMapping = { personForSource, calendarIdsForPerson };
})();
