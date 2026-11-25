// Preview server: serves web/static with demo data instead of a real backend.
// Usage: node web/preview.mjs [--port 8099] [--scene <id>] [--palette paper|board]
import http from "node:http";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { parseArgs } from "node:util";
import { demoDashboard } from "./preview-data.mjs";

const staticDir = path.join(path.dirname(fileURLToPath(import.meta.url)), "static");
const TYPES = {
  ".html": "text/html; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".mjs": "text/javascript; charset=utf-8",
  ".woff2": "font/woff2",
  ".svg": "image/svg+xml",
};

const { values } = parseArgs({
  options: { port: { type: "string", default: "8099" }, scene: { type: "string" }, palette: { type: "string" } },
});

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url, "http://localhost");
  const send = (status, type, body) => {
    res.writeHead(status, { "Content-Type": type, "Cache-Control": "no-store" });
    res.end(body);
  };
  const json = (body) => send(200, "application/json", JSON.stringify(body));

  if (url.pathname === "/api/dashboard" || url.pathname === "/api/scene") {
    const scene = url.searchParams.get("scene") || values.scene || "afternoon";
    const d = demoDashboard(new Date(), { scene, palette: values.palette });
    return json(url.pathname === "/api/scene" ? { scene: d.scene, layout: d.layout } : d);
  }

  const rel = url.pathname === "/" ? "index.html" : decodeURIComponent(url.pathname.slice(1));
  const file = path.join(staticDir, rel);
  const type = TYPES[path.extname(file)];
  if (!type || !file.startsWith(staticDir + path.sep)) return send(404, "text/plain", "not found");
  try {
    send(200, type, await readFile(file));
  } catch {
    send(404, "text/plain", "not found");
  }
});

server.listen(Number(values.port), "127.0.0.1", () => {
  console.log(`preview: http://127.0.0.1:${values.port}/`);
});
