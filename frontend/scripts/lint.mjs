import { readFileSync } from "node:fs";

const source = readFileSync("src/app.ts", "utf8");
if (source.includes("any")) {
  throw new Error("avoid any in public frontend contracts");
}
console.log("frontend lint passed");
