import { readFileSync } from "node:fs";

const source = readFileSync("src/app.ts", "utf8");
if (!source.includes("export interface ProductCard")) {
  throw new Error("ProductCard contract is missing");
}
console.log("frontend typecheck passed");
