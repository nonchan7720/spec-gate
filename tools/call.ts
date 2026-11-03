// example claim の binding 用アダプタ（TypeScript 関数向け）。
// 使い方: npx tsx tools/call.ts <module path> <export name>
// stdin: {"in": <引数 or 引数配列>}  stdout: {"out": <戻り値>}
import { pathToFileURL } from "node:url";
import path from "node:path";

const [, , mod, fn] = process.argv;
const chunks: Buffer[] = [];
for await (const c of process.stdin) chunks.push(c as Buffer);
const { in: input } = JSON.parse(Buffer.concat(chunks).toString("utf8") || "{}");
const m = await import(pathToFileURL(path.resolve(mod)).href);
const f = m[fn];
if (typeof f !== "function") {
  console.error(`export "${fn}" が ${mod} にない`);
  process.exit(3);
}
const args = Array.isArray(input) ? input : [input];
const out = await f(...args);
process.stdout.write(JSON.stringify({ out: out === undefined ? null : out }));
