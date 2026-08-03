// 极简 lint：源码中出现 any 即失败（公共前端契约禁用 any）。
import { readFileSync } from "node:fs";

const source = readFileSync("src/app.ts", "utf8");
if (source.includes("any")) {
  throw new Error("avoid any in public frontend contracts");
}
console.log("frontend lint passed");
