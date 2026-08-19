# cs-sleeper v1.1.0-rc2 — Funktions- und Security-Analyse

> **Update (v1.1.0-rc3, 2026-08-19):** Alle fünf Empfehlungen aus Abschnitt 4
> sind umgesetzt und veröffentlicht. Status pro Fund:
>
> - **3.1 Safety-Model vs. Implementierung (Boot-Disk/-Pool-Bypass in
>   One-Shot-Befehlen)** — **behoben.** `sleepnow`, `sleeppool` und
>   `export-now` prüfen jetzt vor jeder Aktion gegen dieselbe
>   Never-Sleep-Menge (Boot-Disk, Boot-Pool, Flash-Vdevs, `exclude`) wie der
>   Daemon-Loop (`guardDevice`/`guardBootPool` in `util.go`); `sleeppool`/
>   `export-now` verweigern zusätzlich jede Aktion, wenn `--pool` der
>   Boot-Pool ist.
> - **3.2 PATH-basierte Auflösung externer Tools** — **behoben.** Neues
>   Package `internal/xpath` löst `smartctl`, `zpool`, `qm`, `pgrep`,
>   `iostat`, `findmnt`, `lsblk`, `df`, `diskutil`, `sync`, `powershell`,
>   `taskkill`, `tasklist` zuerst über feste, bekannte Installationspfade auf;
>   `$PATH` ist nur noch der letzte Fallback.
> - **3.3 Keine Validierung von Disk-/Pool-Namen** — **behoben.** Neues
>   `sysio.Valid()` weist Namen mit führendem `-` zurück, geprüft in
>   `sleeper.Sleep/Wake/SetStandbyTimer/SleepAll/WakeAll` und
>   `zfs.Export/Import/Status/DisksOfPool/NeverSleepDisks/Sync` — inklusive
>   des zuvor ungeschützten Windows-Pfads.
> - **3.4 Fehlende Timeouts bei zpool/iostat/qm** — **behoben.** Alle
>   verbleibenden `exec.Command`-Aufrufe ohne Timeout laufen jetzt über
>   `context.WithTimeout` (analog zum bisherigen `smartctl`-Muster).
> - **3.8 Supply-Chain / Build-Pipeline** — **behoben.** Der Release-Workflow
>   erzeugt jetzt `checksums.txt` (SHA-256) für alle Artefakte, und sämtliche
>   GitHub Actions sind auf einen konkreten Commit-SHA statt auf einen
>   Major-Version-Tag gepinnt (mit der Version als Kommentar).
>
> Nicht angefasst (bewusst außerhalb des Fix-Scopes, siehe jeweilige
> Abschnitte): 3.6 (Replikations-Gate bleibt best-effort/`pgrep`-basiert —
> Design-Entscheidung), 3.7 (keine Privilegien-Reduktion — architektureller
> Punkt, kein Quick-Fix), 3.9 (PID-Lock-TOCTOU — sehr geringe praktische
> Relevanz). Details siehe `CHANGELOG.md` (Eintrag `v1.1.0-rc3`) im Repo.

**Repo:** https://github.com/guenther-alka/cs-sleeper
**Release:** [v1.1.0-rc2](https://github.com/guenther-alka/cs-sleeper/releases/tag/v1.1.0-rc2) (Release Candidate / Pre-Release)
**Analysiert:** Quellcode 1:1 aus `C:\opt\cs-sleeper-src` (lokaler Checkout) — `main` und Tag `v1.1.0-rc2` zeigen auf denselben Commit (`33990ec`), `origin/main` ist auf demselben Stand, `git remote` zeigt auf das genannte GitHub-Repo. Der lokale Code ist damit inhaltlich identisch mit dem veröffentlichten Release; keine externen Go-Abhängigkeiten (`go.mod` = nur Modulname + `go 1.22`, keine `require`-Zeilen).
**Umfang:** ~3.164 Zeilen Go, 8 Plattform-Targets (mswin/linux/illumos/solaris/freebsd/darwin amd64+arm64).

---

## 1. Zweck

`cs-sleeper` ist ein eigenständiger, abhängigkeitsfreier Go-Daemon für ZFS-Hosts (Teil der napp-it/csweb-gui-Toolfamilie), der:

- pro Platte die I/O-Aktivität überwacht und nach konfigurierbarer Idle-Zeit per `smartctl -s standby,now` in Standby versetzt,
- vor jeder Sleep-/Export-Aktion prüft, ob gerade `zfs send`/`receive` läuft, und die Aktion in dem Fall verweigert,
- ganze Pools schlafen legen kann (`sleeppool`: Standby aller Member-Disks oder `--export` für Backup-Pools), inklusive optionalem Pausieren/Herunterfahren von Proxmox-VMs auf diesem Pool,
- einen einfachen Zeitplaner (`--at HH:MM`) sowie einen Autostart-Mechanismus (`enable`/`disable`) mitbringt.

Neu in v1.1.0-rc2 gegenüber rc1: `wake = on-access`-Tracking (zeigt `last-wake` in `status`) und `parallel = 0` bedeutet jetzt „unbegrenzt parallel“ statt (fälschlich) sequentiell.

---

## 2. Funktionsweise im Detail

**Konfiguration** (`config.go`): Key-Value-Datei unter `/opt/csweb-gui/_cfg/cs-sleeper` (Default, überschreibbar via `--config`/`CS_SLEEPER_CONFIG`), wird beim ersten Lauf mit Defaults angelegt. Unbekannte Keys führen zu einem harten Parse-Fehler (fail-closed statt stillem Ignorieren).

**Idle-Engine** (`internal/sleeper/sleeper.go`): pro Platte wird nach `wait` Sekunden Inaktivität und `verify-idle` aufeinanderfolgenden Idle-Samples geschlafen; „Aktivität" kommt je nach Plattform aus `/proc/diskstats` (Linux, kumulativ, Delta-Vergleich) oder aus `iostat`/PowerShell-Get-Counter (illumos/Solaris/FreeBSD/macOS/Windows, Intervall-Rate). `AllowSleep` blockiert das Schlafenlegen innerhalb konfigurierter `activity`-Fenster.

**Pool-Sleep** (`oneshot.go`): `sleeppool` ruft `zfs.Sync(pool)` (`zpool sync`, auf illumos/Solaris Fallback auf POSIX `sync`) auf, um den Schreib-Cache zu flushen, sampled danach die Disk-I/O erneut (`reverifyIdle`) und lässt nur die wirklich idle gebliebenen Platten in Standby gehen — das verhindert, dass eine Platte sofort durch den nächsten ZFS-Transaction-Group-Commit wieder aufwacht.

**VM-Handling** (`internal/vm/vm_proxmox.go`): löst über `/etc/pve/storage.cfg` + `qm config <id>` auf, welche VMs auf dem Ziel-Pool liegen, und pausiert/fährt sie herunter (`qm suspend --todisk` / `qm shutdown`) bzw. weckt sie wieder. Betroffene VM-IDs werden in `state-dir/vmstate.json` persistiert, damit `wakepool` genau die zuvor geschlafenen VMs wieder aufweckt.

**Scheduling** (`schedule.go`): `--at HH:MM` schreibt eine Aufgabe in `state-dir/schedule.json`; der laufende Daemon führt fällige Aufgaben in seiner Sample-Schleife aus (`runDueTasks`).

**Sicherheitsnetz laut README** („Safety model"): Replikations-Gate (pgrep-basiert), Idle+Verify-Schwelle, Activity-Fenster, Allow-Liste (`hd`/`disks`), kein erzwungenes Export/Import ohne `--force`, sowie ein „Never-sleep"-Set (OS-Bootdisk, Boot-Pool-Disks, SLOG/L2ARC/special/dedup) zusätzlich zu `exclude`.

---

## 3. Security-Analyse

### 3.1 Wichtigster Fund: Safety-Model gilt nicht für die One-Shot-Befehle

Das in der README beschriebene „Never-sleep"-Set (Boot-Disk, Boot-Pool, Flash-Vdevs, `exclude`) wird ausschließlich in `managedDevices()` (`util.go`) berechnet — und **nur der kontinuierliche Daemon-Loop** (`daemon.go`) benutzt diese Funktion.

Die One-Shot-Befehle prüfen das nicht:

- `sleepnow --disk <name>` (`oneshot.go`, `diskCmd`) ruft `sleeper.Sleep(*disk)` direkt auf — kein Abgleich mit Boot-Disk, Boot-Pool oder `exclude`. Ein `cs-sleeper sleepnow --disk <bootdisk>` legt die laufende OS-Platte in Standby.
- `sleeppool --pool <name>` / `export-now --pool <name>` (`execSleepPool`, `poolCmd`) lösen die Member-Disks über `zfs.DisksOfPool(pool)` auf und legen sie schlafen bzw. exportieren den Pool — ohne zu prüfen, ob `pool` zufällig der Boot-Pool ist. `cs-sleeper export-now --pool rpool --force` würde anstandslos versucht.

Einzig `replcheck` (laufendes `zfs send/receive`) wird bei `sleep`/`export`-Aktionen geprüft; Flash-Devices sind bei `sleeppool` nur deshalb sicher, weil `zfs.DisksOfPool` sie strukturell nicht liefert (nicht wegen einer expliziten Prüfung) — bei `sleepnow --disk <slog-device>` greift dieser Schutz aber nicht, da dort gar keine Pool-Auflösung stattfindet.

**Einordnung:** Kein klassischer Exploit, aber ein reales operatives Risiko auf einem Storage-Host mit Root-Rechten — insbesondere wenn csweb-gui diese Befehle aus einem Web-Formular heraus mit Nutzereingabe für `--disk`/`--pool` aufruft. Die dokumentierte Garantie „the OS boot disk … are never spun down" stimmt schlicht nicht für die interaktiven/GUI-getriggerten Befehle.

### 3.2 PATH-basierte Auflösung externer Tools (Root-Kontext)

Der Daemon läuft praktisch immer mit Root-/Administrator-Rechten (nötig für `smartctl` Spin-down und `zpool import/export`) und ruft `zpool`, `zfs`, `pgrep`, `iostat`, `findmnt`, `lsblk`, `df`, `qm`, `powershell`, `taskkill`, `tasklist` ausschließlich über den **bloßen Programmnamen** via `exec.Command(...)` auf — d. h. Auflösung über `$PATH`. Nur `smartctl` bekommt eine defensive Fallback-Liste fester Pfade (`internal/sleeper/sleep.go`), aber auch dort wird zuerst `exec.LookPath("smartctl")` (PATH-Suche) probiert.

Läuft der Daemon in einer Umgebung, in der `$PATH` (z. B. durch ein Start-Skript, systemd-Unit ohne explizites `PATH=`, oder eine vom Angreifer beeinflussbare frühere Verzeichnis-Reihenfolge) manipulierbar ist, kann ein lokaler Angreifer eine bösartige Binärdatei namens `zpool`, `smartctl` etc. in einem früher gelisteten Verzeichnis platzieren und so Code mit den Rechten des Daemons (root) ausführen — klassisches PATH-Hijacking. Empfehlung: alle Aufrufe auf feste, absolute Pfade umstellen (analog zur bereits vorhandenen `smartctl`-Fallback-Logik) oder zumindest beim Start ein explizites, minimales `PATH` setzen.

### 3.3 Keine Validierung von Disk-/Pool-Namen vor der Weitergabe als CLI-Argument

Datei-/Pool-/Disk-Namen aus Config oder `--disk`/`--pool` werden unverändert als letztes Argument an `smartctl`/`zpool` übergeben. Da `exec.Command` **keine Shell** aufruft, ist klassische Shell-Injection ausgeschlossen — aber ein Name, der mit `-` beginnt, wird vom Zielprogramm selbst als Flag interpretiert:

- Unter Linux/FreeBSD/macOS/illumos wird die Disk-`DevicePath()` immer mit einem festen Präfix (`/dev/…`) versehen, was einen führenden `-` neutralisiert.
- Unter **Windows** gibt `DevicePath()` den Namen unverändert zurück (`sysio_windows.go`, Zeile „return name“) — ein Disk-Name wie `-d` würde 1:1 an `smartctl` durchgereicht.
- Pool-Namen (`zfs.Export`/`Import`/`DisksOfPool`) werden nirgends auf ein führendes `-` geprüft, unabhängig von der Plattform.

Impact ist moderat (Argument-Verwirrung bei `zpool`/`smartctl`, kein Remote-Code-Execution), aber relevant, falls csweb-gui Pool-/Disk-Namen aus einem Web-Formular ungeprüft an `--pool`/`--disk` durchreicht.

### 3.4 Fehlende Timeouts bei den meisten Subprozess-Aufrufen

Nur die `smartctl`-Aufrufe (`internal/sleeper/sleep.go`) laufen mit einem 30s-`context.WithTimeout`. Alle anderen externen Aufrufe — `zpool sync/export/import/status`, `iostat`, `qm list/config/suspend/shutdown`, `findmnt`, `lsblk`, `df`, PowerShell `Get-Counter` — laufen ohne Timeout über einfaches `exec.Command(...).Output()`. Hängt eines dieser Kommandos (z. B. `zpool` bei einem hängenden/nicht mehr erreichbaren Device, oder `qm` bei einem nicht antwortenden Proxmox-Stack), blockiert die komplette Sample-Schleife des Daemons unbegrenzt — Verfügbarkeits-/Robustheitsrisiko, kein reiner Security-Bug, aber auf einem Storage-Host mit potenziell hängenden Devices durchaus relevant.

### 3.5 Dateiberechtigungen

Config (`0644`), State-/Schedule-/VM-State-Dateien (`0644`) und deren Verzeichnisse (`0755`) sind world-readable. Enthalten sind Disk-/Pool-/VM-Namen und smartctl-/zpool-Kommandoausgaben (ggf. inkl. Seriennummern) — keine Secrets, aber ein gewisses Informationsleck an lokale, nicht-privilegierte Nutzer über die Storage-Topologie des Hosts. Kein Hardening auf `0600`/`0640`.

### 3.6 Replikations-Gate ist best-effort

`replcheck.Check()` (Unix) basiert auf `pgrep -f '[z]fs send'` etc. — eine reine Prozessname-/Cmdline-Heuristik. Sie schützt vor versehentlichem Eingriff während eines laufenden `zfs send`/`receive`, ist aber kein hartes Sicherheitsmerkmal: umbenannte Wrapper, Prozesse in Containern/anderen PID-Namespaces oder eine Cmdline ohne die literalen Strings würden nicht erkannt. Für den dokumentierten Zweck („Schutz vor versehentlichem Unterbrechen einer Replikation") ist das ein akzeptabler, aber klar limitierter Ansatz — sollte nicht als belastbare Sicherheitsgrenze missverstanden werden.

### 3.7 Keine Privilegien-Reduktion

Der Daemon läuft dauerhaft mit vollen Root-/Administrator-Rechten, auch während er nur `/proc/diskstats` liest oder `iostat` aufruft (Tätigkeiten, die keine Root-Rechte bräuchten). Es gibt keinen Versuch, nach der Initialisierung Rechte abzugeben (z. B. Linux Capabilities statt vollem root). Kein Codefehler im engeren Sinn, aber ein Defense-in-Depth-Manko: eine Schwachstelle im Daemon selbst (z. B. ein Parsing-Bug in einem der `iostat`/`Get-Counter`-Parser) hätte vollen Root-Impact statt eingeschränkten.

### 3.8 Supply-Chain / Build-Pipeline

Positiv: keine Go-Fremdabhängigkeiten (0 Zeilen `require` in `go.mod`) → minimale Angriffsfläche über Drittbibliotheken. Build mit `CGO_ENABLED=0`, `-trimpath`, statisch gelinkt, 8 Plattform-Targets, Release-Workflow triggert ausschließlich auf `push: tags: v*` (kein `pull_request`-Trigger, also kein „pwn request"-Risiko von außen), `permissions: contents: write` ist minimal gescoped.

Verbesserungspotenzial: Die GitHub Actions (`actions/checkout@v4`, `actions/setup-go@v5`, `softprops/action-gh-release@v2` etc.) sind auf **Major-Version-Tags** statt auf feste Commit-SHAs gepinnt — ein kompromittiertes Upstream-Tag könnte unbemerkt Code in den Build-Prozess einschleusen. Zudem generiert die Pipeline keine Checksums (`sha256sum`) oder Signaturen (z. B. `cosign`) für die veröffentlichten `.tar.gz`-Artefakte, und die README erwähnt keinen Integritäts-Check bei der Installation — wer die Binaries von der Releases-Seite lädt, hat außer HTTPS/GitHub-Auth keine Möglichkeit, die Herkunft zu verifizieren.

### 3.9 Sonstiges

- **PID-Lock Race (TOCTOU):** `acquireLock()` (`util.go`) liest die PID-Datei, prüft `processAlive`, und schreibt danach die eigene PID — zwischen Prüfung und Schreiben liegt ein (sehr kleines) Zeitfenster, in dem zwei Daemon-Instanzen gleichzeitig starten könnten. Geringe praktische Relevanz (lokaler, i. d. R. durch systemd/den GUI-Backend kontrollierter Start).
- Keine Netzwerk-Exposition: `cs-sleeper` selbst öffnet keine Ports und hat keine Netzwerk-Schnittstelle; die gesamte Angriffsfläche ist lokal (Config-Datei, State-Verzeichnis, CLI-Argumente) bzw. hängt von der Absicherung des csweb-gui-Backends ab, das die Befehle mit Nutzereingaben aufruft.
- `--force` reicht `-f` unverändert an `zpool export/import` durch — das ist als bewusst opt-in dokumentiertes Feature zu werten, kombiniert mit 3.1 (kein Boot-Pool-Check) aber besonders vorsichtig zu behandeln.

---

## 4. Einordnung

`cs-sleeper` ist sauber strukturierter, minimalistischer Go-Code ohne Fremdabhängigkeiten, mit durchgängiger `exec.Command`-Argumentliste statt Shell-Aufrufen (das größte klassische Risiko — Shell-Injection — ist damit von vornherein ausgeschlossen). Die Implementierung ist als **Release Candidate** klar gekennzeichnet (README + `prerelease: true` im Workflow), was zur beobachteten Lückenhaftigkeit beim Safety-Model (3.1) passt. Die relevantesten Punkte für den produktiven Einsatz auf einem Storage-Host sind:

1. Safety-Checks (Boot-Disk/-Pool, Flash-Devices, `exclude`) auf **alle** Befehle ausweiten, nicht nur auf den Daemon-Loop (höchste Priorität — Datenverlust-/Ausfallrisiko).
2. Externe Kommandos über feste, absolute Pfade statt PATH-Suche aufrufen (Root-Kontext).
3. Disk-/Pool-Namen vor der CLI-Übergabe validieren (kein führendes `-`), besonders unter Windows.
4. Timeouts auch für `zpool`/`iostat`/`qm`-Aufrufe ergänzen.
5. Für die Release-Artefakte Checksums/Signatur ergänzen und GitHub Actions auf Commit-SHA statt Major-Tag pinnen.

Keiner der Punkte deutet auf eine remote ausnutzbare Schwachstelle hin — die Angriffsfläche ist rein lokal und setzt entweder Schreibzugriff auf Config/PATH oder ungeprüfte Eingaben aus einer aufrufenden Schicht (z. B. csweb-gui) voraus. Punkt 1 ist aber unabhängig davon ein funktionaler Fehler mit realem Schadenspotenzial (versehentliches Schlafenlegen/Exportieren des Boot-Pools) und sollte vor einem GA-Release (`v1.1.0`) behoben werden.
