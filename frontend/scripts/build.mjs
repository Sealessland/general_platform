import { mkdirSync, readFileSync, writeFileSync } from "node:fs";

mkdirSync("dist", { recursive: true });

const source = readFileSync("src/app.ts", "utf8");

function stripTs(src) {
  return src
    .replace(/export\s+interface\s+\w+\s*{[\s\S]*?}\s*/g, "")
    .replace(/interface\s+\w+\s*{[\s\S]*?}\s*/g, "")
    .replace(/export\s+type\s+\w+\s*=\s*[\s\S]*?;/g, "")
    .replace(/type\s+\w+\s*=\s*[\s\S]*?;/g, "")
    .replace(/\bexport\s+/g, "");
}

const js = stripTs(source);

const html = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8" />
<meta name="viewport" content="width=device-width, initial-scale=1.0" />
<title>RedCart Copilot</title>
<style>
  :root {
    --bg: #f3f5f7;
    --surface: #ffffff;
    --surface-soft: #f8fbff;
    --text: #161c24;
    --muted: #5f6b7a;
    --border: #d7dde5;
    --primary: #1d4ed8;
    --primary-soft: #dbeafe;
    --danger: #dc2626;
    --danger-soft: #fee2e2;
    --ok: #166534;
    --ok-soft: #dcfce7;
    --radius: 6px;
    --shadow: 0 1px 3px rgba(15, 23, 42, 0.08);
  }
  * { box-sizing: border-box; }
  html, body { height: 100%; }
  body {
    margin: 0;
    background: var(--bg);
    color: var(--text);
    font: 14px/1.5 -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif;
  }
  #app { min-height: 100%; display: flex; flex-direction: column; }
  .topbar {
    position: sticky;
    top: 0;
    z-index: 10;
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
    padding: 12px 16px;
    background: rgba(255, 255, 255, 0.92);
    backdrop-filter: blur(10px);
    border-bottom: 1px solid var(--border);
  }
  .top-left, .top-right, .meta, .actions, .summary-bar, .chip-row {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }
  .brand { font-size: 15px; font-weight: 700; }
  .role-tag {
    padding: 2px 8px;
    border-radius: 999px;
    background: var(--primary-soft);
    color: var(--primary);
    font-size: 12px;
    font-weight: 600;
  }
  .account, .muted, .metric-label { color: var(--muted); }
  .layout { display: flex; flex: 1; min-height: 0; }
  .nav {
    width: 220px;
    flex-shrink: 0;
    border-right: 1px solid var(--border);
    background: var(--surface);
    padding: 12px 10px;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .nav a {
    text-decoration: none;
    color: var(--text);
    padding: 8px 10px;
    border-radius: var(--radius);
  }
  .nav a.active {
    background: var(--primary-soft);
    color: var(--primary);
    font-weight: 600;
  }
  .main {
    flex: 1;
    min-width: 0;
    padding: 16px;
    overflow-y: auto;
  }
  .page-head { margin-bottom: 14px; }
  .page-head h1 { margin: 0 0 4px; font-size: 24px; }
  .panel, .card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    box-shadow: var(--shadow);
  }
  .panel { padding: 16px; }
  .panel.soft { background: var(--surface-soft); }
  .panel.narrow { max-width: 540px; }
  .card { padding: 12px; }
  .card h3 { margin: 0 0 6px; font-size: 15px; }
  .card.metric {
    min-height: 118px;
    display: flex;
    flex-direction: column;
    justify-content: space-between;
  }
  .metric-value { font-size: 22px; font-weight: 700; }
  .grid {
    display: grid;
    gap: 12px;
  }
  .grid.cols-2 { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .grid.cols-3 { grid-template-columns: repeat(3, minmax(0, 1fr)); }
  .summary-bar {
    justify-content: space-between;
    padding-top: 12px;
    margin-top: 14px;
    border-top: 1px solid var(--border);
  }
  .summary-money { font-weight: 700; }
  .form-row {
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin-bottom: 12px;
  }
  .label { color: var(--muted); font-size: 12px; font-weight: 600; }
  input, textarea, select {
    width: 100%;
    padding: 8px 10px;
    border-radius: var(--radius);
    border: 1px solid var(--border);
    background: #fff;
    color: var(--text);
    font: inherit;
  }
  .mini-input { width: 78px; }
  .btn, .chip {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
    border-radius: var(--radius);
    border: 1px solid var(--border);
    background: var(--surface);
    color: var(--text);
    cursor: pointer;
    padding: 7px 12px;
    font: inherit;
  }
  .btn.primary {
    background: var(--primary);
    border-color: var(--primary);
    color: #fff;
  }
  .btn.danger {
    background: var(--danger);
    border-color: var(--danger);
    color: #fff;
  }
  .btn.mini { padding: 5px 8px; font-size: 12px; }
  .btn:disabled { opacity: 0.55; cursor: not-allowed; }
  .chip { border-radius: 999px; padding: 4px 10px; font-size: 12px; }
  .chip.static { cursor: default; background: #f8fafc; }
  .badge {
    display: inline-flex;
    align-items: center;
    padding: 2px 8px;
    border-radius: 999px;
    font-size: 12px;
    font-weight: 600;
  }
  .badge.created { background: #e0ecff; color: #1d4ed8; }
  .badge.paid { background: #dcfce7; color: #166534; }
  .badge.shipped { background: #ffedd5; color: #c2410c; }
  .badge.finished { background: #e5e7eb; color: #374151; }
  .badge.cancelled { background: #fee2e2; color: #b91c1c; }
  .badge.refunding { background: #fef3c7; color: #92400e; }
  .badge.refunded { background: #f3e8ff; color: #6b21a8; }
  .notice, .error, .empty {
    border-radius: var(--radius);
    padding: 12px;
    margin-bottom: 12px;
  }
  .notice { background: var(--primary-soft); color: var(--primary); border: 1px solid #bfdbfe; }
  .notice.ok { background: var(--ok-soft); color: var(--ok); border-color: #86efac; }
  .notice.info { background: var(--primary-soft); color: var(--primary); }
  .error { background: var(--danger-soft); color: var(--danger); border: 1px solid #fecaca; }
  .empty { color: var(--muted); text-align: center; border: 1px dashed var(--border); background: rgba(255,255,255,0.75); }
  table {
    width: 100%;
    border-collapse: collapse;
    font-size: 13px;
  }
  th, td {
    padding: 10px 8px;
    text-align: left;
    border-bottom: 1px solid var(--border);
    vertical-align: top;
  }
  th { color: var(--muted); background: #fafbfc; font-weight: 600; }
  tr:last-child td { border-bottom: none; }
  .spinner {
    display: inline-block;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    border: 2px solid #cbd5e1;
    border-top-color: var(--primary);
    animation: spin 1s linear infinite;
  }
  @keyframes spin { to { transform: rotate(360deg); } }
  .a2ui-surface { padding: 12px; }
  .a2ui-column { display: flex; flex-direction: column; gap: 10px; }
  .a2ui-row { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  .a2ui-text { color: var(--text); }
  .a2ui-text.h2 { font-size: 20px; font-weight: 700; }
  .a2ui-unknown { color: var(--danger); font-family: monospace; font-size: 12px; }
  @media (max-width: 980px) {
    .grid.cols-2, .grid.cols-3 { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  }
  @media (max-width: 720px) {
    .layout { flex-direction: column; }
    .nav {
      width: auto;
      border-right: none;
      border-bottom: 1px solid var(--border);
      flex-direction: row;
      flex-wrap: wrap;
    }
    .grid.cols-2, .grid.cols-3 { grid-template-columns: 1fr; }
  }
</style>
</head>
<body>
<div id="app"></div>
<script>
${js}
</script>
</body>
</html>`;

writeFileSync("dist/index.html", html);
console.log("frontend build passed");
