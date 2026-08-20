# Neev Remote — File-transfer + Clipboard Regression Checklist

**Run this after ANY change to UI, IPC, event handling, session logic, the consent
popup, the transport/worker, or the helper — regardless of how unrelated the change
looks.** History (see PROJECT_MEMORY Change Log) shows file-transfer and clipboard
break as side effects of "unrelated" changes repeatedly; "looks unrelated" has not
been a reliable signal in this codebase.

## Core matrix (Windows↔Windows, the protected baseline — LD-13)

Test each independently and note PASS/FAIL:

1. **Import** (viewer → host): send a small file and a large file (>15 MB) from the
   viewer. Both land in the host user's Downloads and confirm ("Saved on host").
2. **Export** (host → viewer): request a file from the viewer; host picker appears
   on the host desktop; selected file arrives on the viewer.
3. **Clipboard text**: copy text on host → Ctrl+V on viewer, and the reverse.
4. **Clipboard image**: copy an image both directions.
5. **Clipboard file** (Ctrl+C a file on one side → Ctrl+V on the other): both
   directions. (Uses the helper clipagent on 127.0.0.1:47922.)

## Session-switch matrix (TransportMode — the recurring failure surface)

Switch through at least **3 different user profiles in sequence**, and in EACH
profile run the full Core matrix above, then:

6. Switch **back to the original profile** → re-run the Core matrix; it must still
   pass there too (guards the "global, non-recovering break" class).
7. Repeat the whole switch cycle **once more** → confirm no degradation across
   repeated switches.

## Unaffected-features sanity (must stay working)

8. Capture (live video), remote input (mouse/keyboard), secure-desktop / UAC prompt,
   switch-user session survival (no disconnect), chat, and the consent popup —
   confirm each still works after the switch cycles.

## Where to look when something fails

- Host `worker.log` (`%ProgramData%\NeevRemote\`): `receiving file` / `file transfer
  finished` / `create received file failed` (import); `export requested` / `picker
  closed` (export); `announcing host clipboard files` / `staged viewer files` /
  `clipagent write files failed` / `clipagent dial FAILED` (clipboard files).
- Host `transport.log`: `capture worker attached`, `incoming connect`, consent.
- Helper log (`svc`/`clip` tags): clipagent launch per session, worker/agent swap.
- Relay: `docker logs deploy-server-1` — `invalid password` / `connect request
  forwarded` / `agent registered` (auth + routing).
