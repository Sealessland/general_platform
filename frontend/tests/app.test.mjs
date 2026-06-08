import { readFileSync } from "node:fs";
import assert from "node:assert/strict";

const source = readFileSync(new URL("../src/app.ts", import.meta.url), "utf8");

assert.match(source, /min_price_cent/);
assert.match(source, /api\/orders\/preview/);
assert.doesNotMatch(source, /priceFloat/);

assert.match(source, /function formatMoney\(cents\)/);
assert.match(source, /Number\.isInteger\(cents\)/);
assert.match(source, /throw new Error\("amount must be integer cents"\)/);
assert.match(source, /return "¥" \+ \(cents \/ 100\)\.toFixed\(2\)/);

assert.match(source, /function calcProductStock\(product\)/);
assert.match(source, /Math\.max\(0, sku\.stock - sku\.locked_stock\)/);
assert.match(source, /function calcProductPrice\(product\)/);
assert.match(source, /sku\.price_cent < min/);

assert.match(source, /const idemKey = "web-" \+ Date\.now\(\)/);
assert.match(source, /headers: \{ "Idempotency-Key": idemKey \}/);
assert.match(source, /body: JSON\.stringify\(\{\}\)/);

assert.match(source, /request\("\/api\/cart"\)/);
assert.match(source, /request\("\/api\/cart\/items\/" \+ item\.id,\s*\{\s*method: "PUT"/s);
assert.match(source, /request\("\/api\/cart\/items\/" \+ item\.id,\s*\{ method: "DELETE" \}/s);

assert.match(source, /request\("\/api\/merchant\/dashboard\/summary"\)/);
assert.match(source, /request\("\/api\/merchant\/dashboard\/funnel"\)/);
assert.match(source, /request\("\/api\/merchant\/dashboard\/products"\)/);
assert.match(source, /request\("\/api\/ai\/business-review"/);
assert.match(source, /request\("\/api\/ai\/product-selling-points"/);

console.log("frontend tests passed");
