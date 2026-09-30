// familydash reminders bridge
//
// Reads incomplete Apple Reminders via EventKit and pushes them to the
// dashboard (POST /api/reminders). Runs on any Mac signed in to the family's
// iCloud account.
//
//   one-shot:  DASH_URL=http://unraid:8080/api/reminders DASH_TOKEN=… ./familydash-reminders
//   daemon:    … ./familydash-reminders --watch      (pushes on every change + every DASH_INTERVAL s)
//
// Env:
//   DASH_URL       required, full URL of /api/reminders
//   DASH_TOKEN     REMINDERS_TOKEN of the dashboard
//   DASH_LISTS     optional, comma-separated list names to include (default: all)
//   DASH_SOURCE    optional, name of this source (default: computer name)
//   DASH_INTERVAL  optional, seconds between periodic pushes in --watch mode (default 300)

import AppKit
import EventKit
import Foundation

struct Item: Encodable {
    let id: String
    let title: String
    let due: Date?
    let dueAllDay: Bool
    let priority: Int
    let notes: String?
}

struct ListPayload: Encodable {
    let name: String
    let color: String?
    let items: [Item]
}

struct Payload: Encodable {
    let source: String
    let lists: [ListPayload]
}

let env = ProcessInfo.processInfo.environment
guard let urlString = env["DASH_URL"], let url = URL(string: urlString) else {
    fputs("DASH_URL fehlt, z.B. http://192.168.188.127:8080/api/reminders\n", stderr)
    exit(2)
}
let token = env["DASH_TOKEN"] ?? ""
let source = env["DASH_SOURCE"] ?? Host.current().localizedName ?? "mac"
let wanted = Set((env["DASH_LISTS"] ?? "")
    .split(separator: ",")
    .map { $0.trimmingCharacters(in: .whitespaces) }
    .filter { !$0.isEmpty })
let interval = TimeInterval(env["DASH_INTERVAL"] ?? "") ?? 300
let watch = CommandLine.arguments.contains("--watch")

let store = EKEventStore()

func requestAccess() -> Bool {
    let sem = DispatchSemaphore(value: 0)
    var granted = false
    let done: (Bool, Error?) -> Void = { ok, err in
        granted = ok
        if let err { fputs("Zugriffsfehler: \(err.localizedDescription)\n", stderr) }
        sem.signal()
    }
    if #available(macOS 14.0, *) {
        store.requestFullAccessToReminders(completion: done)
    } else {
        store.requestAccess(to: .reminder, completion: done)
    }
    sem.wait()
    return granted
}

func hex(_ cg: CGColor?) -> String? {
    guard let cg, let c = NSColor(cgColor: cg)?.usingColorSpace(.sRGB) else { return nil }
    return String(format: "#%02X%02X%02X",
                  Int((c.redComponent * 255).rounded()),
                  Int((c.greenComponent * 255).rounded()),
                  Int((c.blueComponent * 255).rounded()))
}

func collect() -> Payload {
    var cals = store.calendars(for: .reminder)
    if !wanted.isEmpty { cals = cals.filter { wanted.contains($0.title) } }

    let predicate = store.predicateForIncompleteReminders(withDueDateStarting: nil, ending: nil, calendars: cals)
    let sem = DispatchSemaphore(value: 0)
    var reminders: [EKReminder] = []
    store.fetchReminders(matching: predicate) { r in
        reminders = r ?? []
        sem.signal()
    }
    sem.wait()

    let byCal = Dictionary(grouping: reminders, by: { $0.calendar.calendarIdentifier })
    let lists = cals.map { cal -> ListPayload in
        let items = (byCal[cal.calendarIdentifier] ?? []).map { r -> Item in
            let comps = r.dueDateComponents
            return Item(
                id: r.calendarItemIdentifier,
                title: r.title ?? "",
                due: comps.flatMap { Calendar.current.date(from: $0) },
                dueAllDay: comps != nil && comps?.hour == nil,
                priority: r.priority,
                notes: r.notes
            )
        }
        return ListPayload(name: cal.title, color: hex(cal.cgColor), items: items)
    }
    return Payload(source: source, lists: lists)
}

func push() {
    let payload = collect()
    var req = URLRequest(url: url, timeoutInterval: 20)
    req.httpMethod = "POST"
    req.setValue("application/json", forHTTPHeaderField: "Content-Type")
    if !token.isEmpty { req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization") }
    let enc = JSONEncoder()
    enc.dateEncodingStrategy = .iso8601
    req.httpBody = try? enc.encode(payload)

    let count = payload.lists.reduce(0) { $0 + $1.items.count }
    let sem = DispatchSemaphore(value: 0)
    URLSession.shared.dataTask(with: req) { _, resp, err in
        if let err {
            fputs("Push fehlgeschlagen: \(err.localizedDescription)\n", stderr)
        } else if let http = resp as? HTTPURLResponse, http.statusCode >= 300 {
            fputs("Push: HTTP \(http.statusCode)\n", stderr)
        } else {
            print("\(ISO8601DateFormatter().string(from: Date())) – \(count) Erinnerungen aus \(payload.lists.count) Listen gepusht")
        }
        sem.signal()
    }.resume()
    sem.wait()
}

guard requestAccess() else {
    fputs("Kein Zugriff auf Erinnerungen. Freigeben unter Systemeinstellungen → Datenschutz & Sicherheit → Erinnerungen.\n", stderr)
    exit(1)
}

push()

if watch {
    var pending: DispatchWorkItem?
    NotificationCenter.default.addObserver(forName: .EKEventStoreChanged, object: store, queue: .main) { _ in
        // iCloud sync fires several notifications in a row – debounce.
        pending?.cancel()
        let work = DispatchWorkItem { push() }
        pending = work
        DispatchQueue.main.asyncAfter(deadline: .now() + 3, execute: work)
    }
    Timer.scheduledTimer(withTimeInterval: interval, repeats: true) { _ in push() }
    RunLoop.main.run()
}
