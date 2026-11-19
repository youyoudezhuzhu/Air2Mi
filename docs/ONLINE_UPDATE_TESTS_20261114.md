# Historical update verification — 2026-11-14

This application ships separately from the Engine. Obtain an approved binary separately and use local import. The source tree omits the previous stable download catalogs. Retain the notices, source and relinking materials accompanying any separately supplied third-party binaries.

- fnOS Go Host: complete race-enabled test suite passed. New regressions cover signed raw payload validation, component separation, old-catalog rejection, URL allowlist, corrupt primary/fallback download, failed Engine health check and preservation of identity/association data. App package download leaves the Engine binary and Engine updater state unchanged.
- Web: fnOS 16 / Android 19 tests passed; both TypeScript checks and production builds passed. Server-rendered regression checks two independent cards and installed versions; no remote “latest” state is invented before a check.
- Android management Python: complete 33-test suite passed; update-specific 10 tests passed including the App-downgrade guard. Java: 10 tests passed, including fixed read-only installer URI restrictions and existing background lifecycle tests. This is not a claim of automated OS installer confirmation.
- FPK: actual fnpack 1.2.3 package built with signed, locked Linux Engine; format, executable lifecycle scripts, payload inclusion and no private key checks passed. No NAS application was installed or stopped during this work.
- Stable Engine: original 497 root tests, 64 kernel tests and acceptance checks passed before final codec packaging. Linux hardened package self-check passed with 139 compiled modules and native authorization gate checks. The same frozen AAC encoder/configuration produced byte-identical ADTS for a 3-second two-channel test after removing GPL/nonfree build flags.
- Android Engine: real ARM64/Bionic build, AAR/APK/package audit passed. On the ARM64 test device, an isolated test App passed native free limits, C-import gate, dependency replacement/tampering rejection, actual API start and native AAC encoding. Both temporary test packages were removed; the existing formal App was preserved. No real CDK was used.

Remaining user/system verification: OS-confirmed Android overwrite installation and resumption with real AirPlay playback; fnOS App Center overwrite installation while preserving production configuration; playback across a live Engine upgrade. These are separate from compilation, unit tests, signed transport and native startup. The installer paths require system/user confirmation rather than elevated silent installation.

Published binaries retain their exact source pins in the signed catalogs even if documentation or catalogs subsequently receive another commit. CI-signed debug artifacts are not automatically promoted into the signed production download catalog.

## Published-source checks and subsequent local-build policy

The public Pages deployment and both previously triggered App CI builds completed successfully before the user requested local-only compilation. Automatic cloud build triggers are now removed; Pages deployment remains static download hosting only.

Actual anonymous checks passed for both signed catalogs. A temporary fnOS Host independently detected App 0.1.12 → 0.1.13 while reporting the installed stable Engine as current, downloaded and served the 93,084,532-byte verified FPK, and preserved its identity sentinel without starting an audio Engine. The fixed Release Engine bundle also downloaded and verified.

The Android update transport independently detected App and Engine updates, downloaded the 149,160,379-byte APK from Pages and verified SHA256. Its signing certificate matches the already-installed formal App on the attached phone. A separate on-device instrumentation test using Android's embedded Python passed real TLS and signature verification for both independent public catalogs. Isolated test packages were removed; no formal OS overwrite install was requested.


Binary download references have been omitted from this historical report.

Historical build evidence: fnOS run 43580201265, Android run 43580216440, Pages run 43580201342. This records completed runs; it does not dispatch new cloud compilation.
