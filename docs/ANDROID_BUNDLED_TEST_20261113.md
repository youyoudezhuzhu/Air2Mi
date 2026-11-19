# Air2Mi local bundled Engine test build

This is the full Android management App, not the separate Engine Test launcher.
The special `bundledTest` build embeds a checksum-pinned external Engine ZIP only
in generated build assets. No Engine binary or audio-core source is stored in
this repository. Normal debug/public builds remain Engine-free; their existing
strict APK audit is unchanged and rejects the bundled APK as intended.

## Delivery

* App: `1.1.0-bundled-beta10`, ARM64, Android 10+.
* Label: **Air2Mi 测试版**; application ID: `org.air2mi.app.bundledtest`.
* Engine package: `0.3.11-beta.10-hardened`.
* External component ZIP SHA256:
  `864c60531c872f55483f8bfc125e374ae0fc564d0b721c78f85bc7d0772e17fd`.
* APK: `Air2Mi-1.1.0-bundled-engine-beta10-arm64.apk`, SHA256
  `27bd08c550fd0dc0f9183b52ad3eab15d485bfbc5daa0d968c2bea7b4b22afd2`.
* Local workspace output: `artifacts/air2mi-android-bundledtest-20261113/`,
  including DELIVERY.json, SHA256SUMS, Linux/Android security reports and logs.

The APK is locally test-signed but **not debuggable**; WebView debugging is also
disabled by BuildConfig.DEBUG. It can coexist with normal Air2Mi and has separate
data. On the first state request/start it checks the embedded ZIP against the
compiled digest and installs using the existing verified/atomic importer. An
already installed or manually imported Engine is not silently overwritten.
The bundled asset is not served to the WebView. This is not a production release
and is not uploaded as a public binary; existing Engine redistribution/license
constraints still apply.

## Functional testing for the user

Install this APK and open Air2Mi 测试版; importing a separate Engine ZIP is not
needed. Grant the requested network/notification permissions, start the Engine,
discover speakers, configure receiver associations and test playback, source
changes, pause/resume and background behavior. Only run one bridge instance at a
time to avoid duplicate advertisements/receiver ports. This installation has its
own configuration. Unactivated limits remain two receivers/two output speakers;
the test build does not bypass activation. No real CDK was consumed by our tests.

The new immersive status bar/insets source is included. Both SDK29 cutout and
SDK34 keyboard/system-bar regression tests now pass. Audio listening and real
iPhone-to-speaker behavior on Android remain user acceptance, separate from the
security tests below.

## Actual security checks performed

Linux checks used the actual beta.10 `.run` and its embedded native interpreter:
the existing native gate and dependency-replacement checker passed, and extra
probes rejected executable injection under local/, startup hooks, changed code,
removing the integrity metadata flag and supplying a Python replacement for the
missing native anchor. Extraction remains possible by design and was confirmed.
The Linux/NAS daily service was not replaced or restarted.

Android checks ran the **final delivered APK** on a real ARM64 phone using a
same-signature instrumentation package and an otherwise unused isolated package
ID. Actual compiled modules rejected:

* a third unlicensed receiver or output;
* replacing the final native verifier type's enforcement method;
* patched public licensing functions/constants or the Python-exported gate,
  while the Registry still called its C-imported gate;
* an encrypted envelope with an invalid Ed25519 signature, even after replacing
  public/native key factories, JSON parsing and same-named guard attributes;
* modified native binaries with rewritten metadata and added modules, including
  a local/ executable injection.

The same run verified API start and native AAC encoding. It did not connect real
speakers or use a real CDK. The initial instrumentation attempt failed due to
using Python attribute lookup instead of dictionary lookup in the test harness;
that call was fixed and the final APK rerun passed. No failing security check was
skipped or changed to accept an invalid credential. Temporary target and
instrumentation packages created in user 0 were then uninstalled; the phone's
formal Air2Mi and all other users/apps were left alone.

The App's own generic loader was also hardened: reused extraction is compared
against the archive; unsafe/duplicate entries, symlinks and additional/missing
files are rejected; mapped libraries are not repaired in place; the native
integrity anchor is loaded by exact path and mismatched loaded builds require a
process restart. JNI files are rechecked before System.load.

These are targeted adversarial regressions, **not an independent penetration
audit**. They do not establish resistance to machine-code patching, pre-init
hooks, a patched interpreter/anchor, OS identity spoofing or APK resigning. A
user-imported package's hashes are not a publisher signature. Successful-start
integrity memoization is not a continuous monitor. No claim of unbreakability or
production public-release readiness is made.

## Build and test reproduction

Normal builds remain `:app:assembleDebug` and `tools/check_android_apk.py`.
The optional test build requires an external ZIP and an explicit pinned digest:

```sh
gradle -p android \
  -PbundledEnginePackage=/absolute/path/to/components.zip \
  -PbundledEngineSha256=864c60531c872f55483f8bfc125e374ae0fc564d0b721c78f85bc7d0772e17fd \
  :app:assembleBundledTest
python3 tools/check_android_bundled_apk.py \
  android/app/build/outputs/apk/bundledTest/app-bundledTest.apk \
  --sha256 864c60531c872f55483f8bfc125e374ae0fc564d0b721c78f85bc7d0772e17fd
```

For the isolated device security test, add `-PbundledTestInstrumentation` and
build `:app:assembleBundledTestAndroidTest`, then explicitly select
`org.air2mi.app.BundledSecurityTest` with AndroidJUnitRunner. The probe lives only
in the instrumentation APK, not in the delivered functional APK.

Results: 23 Python management/loader tests, 18 Web tests and 9 Android JVM tests
passed; TypeScript/Vite build and bundled APK lint passed. The final real-device
security instrumentation passed (1 test with the probes above). Ordinary APK
audit and explicit bundled-asset SHA256 audit both passed; the normal audit
correctly rejects the special APK. Sandbox socket restrictions required normal
port-probe tests and Gradle to run with local-socket permissions. Only the App
android branch is changed; no cloud-build success is claimed.
