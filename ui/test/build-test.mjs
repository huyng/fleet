// Bundle the test harness so node can load it.
//
// The app is JSX, which node cannot parse on its own, and the tests run in
// plain node with jsdom rather than in a browser. Bundling the harness once
// keeps the test file free of build concerns.
//
// Usage:
//   node test/build-test.mjs           build once
//   node test/build-test.mjs --watch   rebuild on change

import * as esbuild from "esbuild";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const watch = process.argv.includes("--watch");

const options = {
  entryPoints: { harness: join(here, "harness.jsx") },
  bundle: true,
  format: "esm",
  platform: "node",
  target: ["node20"],
  jsx: "automatic",
  outdir: join(here, ".build"),
  entryNames: "[name]",
  // react and react-dom are bundled in, which is what makes the bundle
  // self-contained. Only node built-ins stay external.
  external: ["node:*"],
  logLevel: "info",
  define: { "process.env.NODE_ENV": '"development"' },
};

if (watch) {
  const context = await esbuild.context(options);
  await context.watch();
} else {
  await esbuild.build(options);
}
