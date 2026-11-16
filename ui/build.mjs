// Build the fleet single page app into the Go package that embeds it.
//
// The output goes to ../internal/web/dist so `go build` can bake it into the
// master binary. Nothing here is served at runtime: the files are read from
// the filesystem only while building.
//
// Usage:
//   node build.mjs            one production build
//   node build.mjs --watch    rebuild on change, for development

import * as esbuild from "esbuild";
import { copyFile, mkdir, readdir } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const outDir = join(here, "..", "internal", "web", "dist");
const watch = process.argv.includes("--watch");

// One bundle for scripts and one for styles, both with stable names. Stable
// names mean the Go side can hard code them and no cache busting is needed:
// the whole bundle changes whenever the binary does.
const options = {
  entryPoints: {
    app: join(here, "src", "main.jsx"),
  },
  bundle: true,
  format: "esm",
  target: ["es2020"],
  jsx: "automatic",
  outdir: outDir,
  entryNames: "[name]",
  assetNames: "[name]",
  loader: {
    ".woff2": "file",
  },
  minify: !watch,
  sourcemap: watch ? "inline" : false,
  logLevel: "info",
  define: {
    "process.env.NODE_ENV": watch ? '"development"' : '"production"',
  },
};

if (watch) {
  const context = await esbuild.context(options);
  await context.watch();
  console.log("watching ui/src, writing to internal/web/dist");
} else {
  await esbuild.build(options);
}

// The HTML shell is a plain file: copy it next to the bundle so the Go side
// can embed the whole directory.
await mkdir(outDir, { recursive: true });
await copyFile(join(here, "index.html"), join(outDir, "index.html"));

// Everything in public/ is served as is, at the path its name gives. That is
// where the favicon lives.
const publicDir = join(here, "public");
await mkdir(publicDir, { recursive: true });
for (const name of await readdir(publicDir)) {
  await copyFile(join(publicDir, name), join(outDir, name));
}
