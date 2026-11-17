# hello-ui: an app with a real backend and a bundled UI

A small app that shows the whole "apps declare their own UI" path with a real
bundler, not a hand-written `remoteEntry.js`:

- **Backend**: `app.py`, plain Python using the `zara_commands` SDK
  (`../../interactivity/zara_commands`): commands (`add_note`, `list_notes`,
  `clear_notes`), a control (`wave_hz`) and telemetry (`uptime_s`, `notes`, `wave`).
- **UI**: `ui/`, React pages built with **Rsbuild + Module Federation**
  (`@module-federation/rsbuild-plugin`). Two pages, `Overview` and `Notes`.
- **Image**: a multi-stage `Dockerfile`. A Node stage builds the UI, and the result is
  copied into `/etc/zaraos/ui/` of a `python:3.12-slim` image.

## Build and run

From `ZaraOS/examples` (the build context also provides the SDK):

```bash
docker build -f app-ui/hello-ui/Dockerfile -t hello-ui:dev .
# arm64 (the Pi): no emulation needed, see "arm64" below
docker buildx build --platform linux/arm64 -f app-ui/hello-ui/Dockerfile -t hello-ui:dev-arm64 --load .
```

Start it through Cortex like any app:

```bash
curl -X POST localhost:8080/instances -H 'Content-Type: application/json' \
  -d '{"app":"hello-ui","image":"hello-ui:dev"}'
```

Its pages then appear in the Driver Station under Apps → Hello Ui.

## How the UI build fits the contract

`ui/rsbuild.config.ts` is the part to copy into a new app. Each choice addresses
one of the open questions from the hand-written test page:

| Concern | What this build does |
|---|---|
| **One React per page** | `react`, `react-dom` and `react/jsx-runtime` are shared as `singleton` with **`import: false`**, so the build never contains React and always runs on the Driver Station's copy (React 19). No "Invalid hook call". |
| **Unknown robot address** | `output.assetPrefix: "auto"`. Chunks and assets resolve relative to wherever the manifest was loaded from (Cortex serves it at `/ui/{app}/{version}/`). Resolved URLs look like `…/static/js/../../static/svg/robot.svg`, which is correct. |
| **Entry** | `entry: "./mf-manifest.json"`. The manifest says the container is `"type": "global"`. With a manifest entry the runtime takes the type from the manifest, so the descriptor's `type` is not used. |
| **CSS** | **CSS Modules** (`*.module.css`). Every class gets a build-unique name, so nothing leaks into the host, and the page doesn't rely on host classes (the host generates Tailwind only for its own sources). The host's reset strips heading styles and gives the page no padding, so the page styles its own `h1`/`h2` and padding. The CSS is listed per exposed module in the manifest, and the runtime loads it. |
| **Lazy chunks and assets** | `components/LiveValues.tsx` is `lazy()`-loaded, and `assets/robot.svg` is shipped as a file (`dataUriLimit: 0`). Both load from the robot. |
| **Unique federation name** | `hello_ui_<version>`, derived from `ui/package.json`. The Driver Station refuses one name from two addresses. |
| **Descriptor** | Generated into the build as `zara-ui.json` from the same config, so pages, modules and the federation name can't drift apart. Cortex overwrites `appId`/`appVersion`. |

Output: about 250 kB in 11 files, far below Cortex's 64 MiB / 4096-file limits.

## arm64

The UI stage runs on the build machine's own platform (`FROM --platform=$BUILDPLATFORM`),
because its output is plain JS/CSS. The final stage only copies files. So
`--platform linux/arm64` builds in seconds without QEMU or binfmt, and the image's
Python is aarch64.

## Verified

Checked in a real browser against the real Driver Station, with Cortex reached at a
non-loopback address (signed requests, CORS):
- the page loads, and React hooks work with the host's React
- the manifest, entry, exposed modules, the lazy chunk, CSS and the SVG all load from the robot
- the page's own styles apply, and every CSS selector is scoped
- commands work, including the failing one's error message
- telemetry is live, and the control's value comes back through it

This held with both `CONTAINER_CLI=docker` and `CONTAINER_CLI=nerdctl`, and from a
Driver Station served over HTTPS against an `https://` robot.
