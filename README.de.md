# NASsurfer – ein echter Chromium-Browser für Synology DSM

[English](README.md) · **Deutsch**

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
![DSM 7.0–7.2](https://img.shields.io/badge/DSM-7.0%E2%80%937.2-informational.svg)
![x86_64](https://img.shields.io/badge/arch-x86__64-lightgrey.svg)

NASsurfer ist ein Paket für Synology DSM, das einen **echten Chromium-Browser auf der NAS**
laufen lässt und ihn in einem normalen DSM-Fenster zeigt. Wenn du unterwegs bist und
**kein VPN geht**, meldest du dich wie gewohnt bei DSM an und kommst trotzdem an die
Weboberflächen in deinem Netz: Router, Drucker, Proxmox, Docker-Werkzeuge, Intranet.

- ein echter Browser: moderne Web-Apps, Konsolen, Live-Grafiken und WebSockets
  funktionieren, weil nichts umgeschrieben wird
- **kein Docker**, kein Container Manager – nur ein Paket
- läuft auf **DSM 7.0, 7.1 und 7.2** (Intel/AMD, `x86_64`)
- Chromium bringt eigene Bibliotheken und Schriften mit und ist unabhängig von den
  DSM-Systembibliotheken
- Sprache und Zeitzone richten sich nach **deinem** Browser
- Kopieren und Einfügen, Downloads, Datei-Uploads und JavaScript-Dialoge funktionieren
- **nur für Administratoren**

---

## Installation

1. Die neueste `NASsurfer-x86_64-….spk` unter [Releases](../../releases) herunterladen.
2. In DSM **Paket-Zentrum → Manuelle Installation** öffnen und die Datei wählen.
3. Den Hinweis auf Drittanbieter-Pakete bestätigen.
4. **NASsurfer** im DSM-Hauptmenü öffnen.

Das Paket ist rund 100 MB groß. Jede offene Browser-Sitzung braucht einige hundert MB RAM.

## Bedienung

- Adresse in die Adressleiste eingeben, z. B. `192.168.1.1` oder `https://pve.lan:8006`.
  Ohne `http://` oder `https://` wird `http://` verwendet.
- **Strg+C / Strg+V** kopieren und fügen zwischen NASsurfer und deinem Rechner ein
  (auf dem Mac: **Cmd+C / Cmd+V**).
- Downloads landen zuerst auf der NAS; NASsurfer fragt dann, ob du sie auf deinem Gerät
  speichern willst. Datei-Uploads laufen umgekehrt.
- Anmeldungen bei deinen Geräten bleiben gespeichert. **⋯ → Anmeldungen und Verlauf
  löschen** entfernt das komplette Browser-Profil.
- Nach **15 Minuten** ohne Nutzung beendet sich der Browser auf der NAS und gibt den
  Speicher frei. Beim nächsten Öffnen wird die letzte Seite wieder geladen.

## So funktioniert es

```
DSM-Fenster (Canvas, Tastatur)  ──►  browser.cgi (prüft die DSM-Anmeldung, stellt ein Token aus)
                                         │  127.0.0.1:47391
                                         ▼
                              nassurfer daemon  ──►  Chromium (DevTools-Protokoll über eine Pipe)
```

- `browser.cgi` prüft die DSM-Anmeldung und stellt ein Token aus, das 8 Stunden gilt.
- Der Dienst prüft jedes Token noch einmal und startet pro DSM-Benutzer einen Chromium
  mit eigenem Profil.
- Das Bild kommt als JPEG-Strom, nur wenn sich etwas ändert. Eingaben gehen in kleinen
  Paketen zurück.

## Sicherheit

- Nur Mitglieder der Gruppe **administrators** können NASsurfer benutzen. Ein Browser auf
  der NAS erreicht alles, was die NAS erreicht.
- Der Dienst lauscht nur auf `127.0.0.1`; Chromium wird über eine Pipe gesteuert, nicht
  über einen offenen Debug-Port.
- Chromium läuft unter dem eigenen, eingeschränkten Paket-Benutzer. DSM erlaubt die
  Namensräume nicht, die Chromiums eigene Sandbox braucht; Chromium läuft deshalb mit
  `--no-sandbox`.
- Zertifikatsfehler werden bewusst ignoriert: Geräte im eigenen Netz haben fast immer
  selbst signierte Zertifikate.
- Chromium braucht regelmäßige Sicherheitsupdates – halte das Paket aktuell.

## Bauen

```sh
tools/prepare-chromium.sh build/chromium   # Chromium + Bibliotheken + Schriften, fest auf den Paketpfad verdrahtet
./build.sh                                 # → dist/NASsurfer-x86_64-<version>.spk
```

Benötigt: Go, npm, python3 mit `brotli`, `patchelf`, `apt-get download` (Ubuntu/Debian).

## Bekannte Grenzen

- Nur `x86_64` (Intel/AMD). Chromium-Builds für ARM-NAS-Modelle gibt es hier nicht.
- Das Bild wird in normaler Auflösung übertragen und kann auf hochauflösenden
  Bildschirmen leicht unscharf wirken.
- Es fühlt sich an wie eine Fernsteuerung: Es gibt eine kurze Verzögerung, besonders
  über langsame Verbindungen.

## Haftungsausschluss

Ich habe NASsurfer in erster Linie für meinen eigenen Einsatz gebaut und nutze es selbst.
Wer es verwendet, tut das in eigener Verantwortung.

NASsurfer steht in keiner Verbindung zu Synology Inc. oder Google LLC.

## Lizenz

[MIT](LICENSE) für den eigenen Code von NASsurfer. Das Paket enthält außerdem
Fremdsoftware unter eigenen Lizenzen, siehe [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
