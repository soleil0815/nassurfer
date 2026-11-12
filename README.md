# NASsurfer – a real Chromium browser for Synology DSM

**English** · [Deutsch](README.de.md)

[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)
![DSM 7.0–7.2](https://img.shields.io/badge/DSM-7.0%E2%80%937.2-informational.svg)
![x86_64](https://img.shields.io/badge/arch-x86__64-lightgrey.svg)

NASsurfer is a package for Synology DSM that runs a **real Chromium browser on your NAS**
and shows it in a normal DSM window. When you are away and **no VPN is available**, you
sign in to DSM as usual and can still reach the web interfaces inside your network:
router, printer, Proxmox, Docker tools, intranet pages.

- a real browser: modern web apps, consoles, live charts and WebSockets work, because
  nothing is rewritten
- **no Docker**, no Container Manager – just a package
- runs on **DSM 7.0, 7.1 and 7.2** (Intel/AMD, `x86_64`)
- Chromium brings its own libraries and fonts and does not depend on DSM's system libraries
- language and time zone follow **your** browser
- tabs, favorites (stored on the NAS), downloads list
- copy & paste, downloads, file uploads and JavaScript dialogs work
- passwords are never stored
- **administrators only**

---

## Installation

1. Download the latest `NASsurfer-x86_64-….spk` from the
   [Releases](../../releases) page.
2. In DSM open **Package Center → Manual Install** and choose the file.
3. Confirm the notice about third-party packages.
4. Open **NASsurfer** from the DSM main menu.

The package is about 100 MB. Each open browser session uses a few hundred MB of RAM.

## Usage

- Type an address in the address bar, e.g. `192.168.1.1` or `https://pve.lan:8006`.
  Without `http://` or `https://`, `http://` is used.
- **Ctrl+C / Ctrl+V** copy and paste between NASsurfer and your computer
  (on a Mac: **Cmd+C / Cmd+V**).
- **Tabs:** `+` opens a new tab, `✕` (or a middle click) closes it. Links that open a
  new window appear as a new tab. Up to 12 tabs.
- **Favorites:** `☆` saves the current page, `★ Favorites` lists them. They are also shown
  on the start page. Favorites are stored per DSM user on the NAS, so they are available
  on every device.
- **Downloads** are first saved on the NAS; NASsurfer then asks whether to save them on
  your device. `⬇` lists the downloads of the current session so you can save them again.
  File uploads go the other way.
- **Passwords are never stored.** Chromium's password manager and form autofill are
  switched off, and any password/form data is deleted on every start. Sign-in cookies are
  kept so you stay signed in; **⋯ → Delete sign-ins and history** removes the whole
  browser profile (favorites are kept).
- After **15 minutes** without use the browser on the NAS quits and frees its memory.
  The next time you open the window, the last page is loaded again.

## How it works

```
DSM window (canvas, keyboard)  ──►  browser.cgi (checks the DSM sign-in, issues a token)
                                        │  127.0.0.1:47391
                                        ▼
                             nassurfer daemon  ──►  Chromium (DevTools protocol over a pipe)
```

- `browser.cgi` checks the DSM sign-in and issues a token valid for 8 hours.
- The daemon checks every token again and starts one Chromium per DSM user with its
  own profile.
- The picture is sent as JPEG frames, only when something changes. Input is sent back
  in small batches.

## Security

- Only members of the **administrators** group can use NASsurfer. A browser on the NAS
  can reach everything the NAS can reach.
- The daemon only listens on `127.0.0.1`; Chromium is controlled through a pipe, not
  through an open debugging port.
- Chromium runs as the package's own restricted user. DSM does not allow the namespaces
  Chromium's own sandbox needs, so Chromium runs with `--no-sandbox`.
- Certificate errors are ignored on purpose: devices inside a network almost always use
  self-signed certificates.
- Chromium needs regular security updates – keep the package up to date.

## Building

```sh
tools/prepare-chromium.sh build/chromium   # Chromium + libraries + fonts, wired to the package path
./build.sh                                 # → dist/NASsurfer-x86_64-<version>.spk
```

Requirements: Go, npm, python3 with `brotli`, `patchelf`, `apt-get download` (Ubuntu/Debian).

## Known limitations

- Only `x86_64` (Intel/AMD). Chromium builds for ARM NAS models are not available here.
- The picture is transferred in normal resolution and can look slightly soft on
  high-DPI screens.
- It feels like a remote session: there is a short delay, especially over slow connections.

## Disclaimer

I built NASsurfer mainly for my own use and use it myself. If you use it, you do so at
your own risk.

NASsurfer is not affiliated with Synology Inc. or Google LLC.

## License

NASsurfer's own code is licensed under the [GNU Affero General Public License v3.0](LICENSE)
(AGPL-3.0-only). You may use, change and share it. If you distribute a modified version
or offer it to others over a network, you must make your source code available under the
same license.

The package also contains third-party software under its own licenses, see
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
