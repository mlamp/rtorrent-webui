# Third-party licenses

`rtorrent-webui` is licensed under Apache-2.0 (see [`LICENSE`](LICENSE)). The compiled
artifacts redistribute the third-party components below. Data attributions (DB-IP) are in
[`NOTICE`](NOTICE). This list covers components shipped in the binary and the embedded SPA;
build- and test-only tools (TypeScript, Vite, svelte-check, Playwright) are not redistributed.

## Go (compiled into the binary)

| Module | License |
|---|---|
| github.com/BurntSushi/toml | MIT |
| github.com/oschwald/geoip2-golang/v2 | ISC |
| github.com/oschwald/maxminddb-golang/v2 | ISC |
| golang.org/x/crypto | BSD-3-Clause |
| golang.org/x/sys | BSD-3-Clause |
| modernc.org/sqlite | BSD-3-Clause |
| modernc.org/libc | BSD-3-Clause |
| modernc.org/mathutil | BSD-3-Clause |
| modernc.org/memory | BSD-3-Clause |
| github.com/dustin/go-humanize | MIT |
| github.com/google/uuid | BSD-3-Clause |
| github.com/mattn/go-isatty | MIT |
| github.com/ncruces/go-strftime | BSD-3-Clause |
| github.com/remyoudompheng/bigfft | BSD-3-Clause |

Regenerate/verify with `go-licenses report ./...` (CI checks this).

## Web (bundled into the embedded SPA)

| Package | License |
|---|---|
| svelte (runtime) | MIT |
| bits-ui | MIT |
| @internationalized/date (via bits-ui) | Apache-2.0 |
| @lucide/svelte | ISC (icon set derives in part from Feather, MIT) |
| @fontsource/jetbrains-mono — wrapper | MIT |
| JetBrains Mono (the font files) | **SIL Open Font License 1.1 (OFL-1.1)** |
| clsx | MIT |
| mode-watcher | MIT |
| svelte-sonner | MIT |
| tailwind-merge | MIT |
| tailwind-variants | MIT |
| Tailwind CSS (compiled output) | MIT |

The OFL-1.1 requires the license to travel with the font and forbids selling the fonts on
their own; the "JetBrains Mono" Reserved Font Name must not be used for modified versions.
The full OFL text is bundled with the font at
`web/node_modules/@fontsource/jetbrains-mono/LICENSE` and is reproduced in the image at
`/usr/share/licenses/OFL-1.1.txt`.
