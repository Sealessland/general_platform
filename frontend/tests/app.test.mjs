import { readFileSync } from "node:fs";
import assert from "node:assert/strict";

const source = readFileSync(new URL("../src/app.ts", import.meta.url), "utf8");

assert.match(source, /min_price_cent/);
assert.match(source, /api\/orders\/preview/);
assert.doesNotMatch(source, /priceFloat/);
console.log("frontend tests passed");
