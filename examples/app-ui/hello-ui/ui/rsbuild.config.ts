import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { pluginModuleFederation } from "@module-federation/rsbuild-plugin";
import { defineConfig, type RsbuildPlugin } from "@rsbuild/core";
import { pluginReact } from "@rsbuild/plugin-react";
import pkg from "./package.json" with { type: "json" };

// Each immutable UI build needs its own federation name: the Driver Station refuses a
// name it already loaded from another address. Derive it from the version.
const federationName = `hello_ui_${pkg.version.replace(/\W/g, "_")}`;

// Pages the Driver Station lists, and the modules that implement them.
const pages = {
  overview: { title: "Overview", module: "./Overview", file: "./src/pages/Overview.tsx" },
  notes: { title: "Notes", module: "./Notes", file: "./src/pages/Notes.tsx" },
};

// Shared with the host. `import: false` means this build never ships its own copy, so
// a page can only ever run against the Driver Station's React (one React per page).
const fromHost = { singleton: true, requiredVersion: "^19.0.0", import: false } as const;

/** Writes /etc/zaraos/ui/zara-ui.json next to the build, from the same config. */
const pluginZaraDescriptor = (): RsbuildPlugin => ({
  name: "zara-ui-descriptor",
  setup(api) {
    api.onAfterBuild(() => {
      const descriptor = {
        schemaVersion: 1,
        sdkVersion: 1,
        // Cortex overwrites appId/appVersion with the instance's app name and version.
        appId: pkg.name,
        appVersion: pkg.version,
        federation: { name: federationName, entry: "./mf-manifest.json", type: "module" },
        pages: Object.fromEntries(
          Object.entries(pages).map(([id, { title, module }]) => [id, { title, module }])
        ),
      };
      writeFileSync(join(api.context.distPath, "zara-ui.json"), `${JSON.stringify(descriptor, null, 2)}\n`);
    });
  },
});

export default defineConfig({
  plugins: [
    pluginReact(),
    pluginModuleFederation({
      name: federationName,
      exposes: Object.fromEntries(Object.values(pages).map(({ module, file }) => [module, file])),
      shared: {
        react: fromHost,
        "react-dom": fromHost,
        "react/jsx-runtime": fromHost,
      },
      manifest: true,
      dts: false,
    }),
    pluginZaraDescriptor(),
  ],
  // The robot's address isn't known at build time: resolve chunks and assets relative to
  // wherever the manifest was loaded from (Cortex serves it at /ui/{app}/{version}/).
  output: {
    assetPrefix: "auto",
    distPath: { root: "dist" },
    // Ship assets as files (not inlined data URIs), so asset URLs are exercised too.
    dataUriLimit: 0,
    cleanDistPath: true,
  },
  // A federation-only build: no standalone page of its own.
  html: { template: undefined },
  tools: { htmlPlugin: false },
  source: { entry: { index: "./src/index.ts" } },
});
