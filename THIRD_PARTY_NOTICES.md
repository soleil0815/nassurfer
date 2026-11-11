# Third-party software / Fremdsoftware

NASsurfer's own code is licensed under the [GNU AGPL v3.0](LICENSE) (AGPL-3.0-only). The package
(`.spk`) also ships the following third-party software, each under its own license.
The license texts are included in the package under `chromium/licenses/`.

Der eigene Code von NASsurfer steht unter der [GNU AGPL v3.0](LICENSE) (AGPL-3.0-only). Das Paket
(`.spk`) enthält außerdem folgende Fremdsoftware unter ihren eigenen Lizenzen.
Die Lizenztexte liegen im Paket unter `chromium/licenses/`.

| Component | License | Source |
|---|---|---|
| Chromium (headless shell) | BSD-3-Clause and others (see Chromium's credits) | https://www.chromium.org/ |
| Chromium build for servers (`@sparticuz/chromium`) | MIT | https://github.com/Sparticuz/chromium |
| NSS / NSPR | MPL-2.0 | https://github.com/nss-dev/nss |
| SwiftShader, Vulkan loader | Apache-2.0 | https://swiftshader.googlesource.com/SwiftShader |
| Expat | MIT | https://libexpat.github.io/ |
| GNU C Library (glibc) | LGPL-2.1-or-later | https://www.gnu.org/software/libc/ · Ubuntu source packages |
| GCC runtime library (libgcc_s) | GPL-3.0 with GCC Runtime Library Exception | https://gcc.gnu.org/ |
| SQLite | Public domain | https://www.sqlite.org/ |
| DejaVu fonts | Bitstream Vera / DejaVu license | https://dejavu-fonts.github.io/ |
| Liberation fonts | SIL Open Font License 1.1 | https://github.com/liberationfonts |

The glibc, libgcc and SQLite libraries are taken unchanged from Ubuntu's binary
packages; their exact sources are available from the corresponding Ubuntu source
packages. NASsurfer only changes the library search path of these binaries
(`patchelf`) so that they run independently of DSM's own libraries.

glibc, libgcc und SQLite stammen unverändert aus den Ubuntu-Binärpaketen; die
passenden Quellen gibt es in den jeweiligen Ubuntu-Quellpaketen. NASsurfer ändert
an diesen Programmen nur den Suchpfad für Bibliotheken (`patchelf`), damit sie
unabhängig von den DSM-eigenen Bibliotheken laufen.

NASsurfer is not affiliated with, endorsed or sponsored by Synology Inc. or Google LLC.
"Synology" and "DSM" are trademarks of Synology Inc.; "Chromium" and "Chrome" are
trademarks of Google LLC.

NASsurfer steht in keiner Verbindung zu Synology Inc. oder Google LLC und wird von
ihnen weder unterstützt noch gesponsert. „Synology“ und „DSM“ sind Marken von
Synology Inc.; „Chromium“ und „Chrome“ sind Marken von Google LLC.
