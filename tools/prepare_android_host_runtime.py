"""Build the management app's Chaquopy runtime from the pinned ARM64 host AAR."""
import argparse
import hashlib
import io
import json
import re
import zipfile
from pathlib import Path

from check_android_apk import audit

ASSETS = "assets/chaquopy/"
RUNTIME_ARCHIVES = {
    "bootstrap.imy", "requirements-arm64-v8a.imy", "requirements-common.imy",
    "stdlib-arm64-v8a.imy", "stdlib-common.imy",
}
JNI_LIBRARIES = {
    "libchaquopy_java.so", "libpython3.so", "libpython3.11.so",
    "libcrypto_chaquopy.so", "libcrypto_python.so", "libssl_chaquopy.so",
    "libssl_python.so", "libsqlite3_chaquopy.so", "libsqlite3_python.so",
}


def empty_zip():
    stream = io.BytesIO()
    with zipfile.ZipFile(stream, "w"):
        pass
    return stream.getvalue()


def public_crypto_dependency(archive):
    """Remove build-machine source paths from the pinned wheel's diagnostic strings."""
    member = "cryptography/hazmat/bindings/_rust.so"
    with zipfile.ZipFile(io.BytesIO(archive)) as source:
        original = source.read(member)
        if hashlib.sha256(original).hexdigest() != "0cc4e14f7c305d04478f13471f41aac4db02f1792421b35c61f997f95815ba18":
            return archive  # Other builds go through the final nested-archive audit.
        # Each match is in .rodata. Preserve Rust's stored string lengths and ELF offsets.
        prefix = rb"/vol[0-9]+/@appdata/[\w./+@-]+?/(?=registry/src/|target/aarch64-linux-android/)"
        def replacement(match):
            return b"/build/" + b"_" * (len(match.group()) - 8) + b"/"
        cleaned = re.sub(prefix, replacement, original)
        if hashlib.sha256(cleaned).hexdigest() != "4ea7b8bef839b1d1f869030da040ec608430a40795f57d0a9181f85d78b24936":
            raise ValueError("Unexpected cryptography diagnostic-path normalization")
        result = io.BytesIO()
        with zipfile.ZipFile(result, "w") as output:
            for entry in source.infolist():
                data = cleaned if entry.filename == member else source.read(entry)
                output.writestr(entry, data)
        return result.getvalue()


def prepare(source_path, output_path):
    with zipfile.ZipFile(source_path) as source:
        assets = {name: source.read(ASSETS + name) for name in RUNTIME_ARCHIVES | {"cacert.pem"}}
        for entry in source.infolist():
            if entry.filename.startswith(ASSETS + "bootstrap-native/arm64-v8a/") and entry.filename.endswith(".so"):
                assets[entry.filename[len(ASSETS):]] = source.read(entry)

        # Chaquopy expects an app archive. The management app supplies its own loader.
        assets["app.imy"] = empty_zip()
        metadata = json.loads(source.read(ASSETS + "build.json"))
        if metadata["python_version"] != "3.11":
            raise ValueError("Expected Chaquopy for Python 3.11")
        for name, data in assets.items():
            if name != "app.imy" and hashlib.sha1(data).hexdigest() != metadata["assets"][name]:
                raise ValueError(f"Chaquopy asset digest mismatch: {name}")
        assets["requirements-common.imy"] = public_crypto_dependency(assets["requirements-common.imy"])
        metadata = {
            "python_version": "3.11",
            "extract_packages": [],
            "assets": {name: hashlib.sha1(data).hexdigest() for name, data in sorted(assets.items())},
        }
        files = {ASSETS + name: data for name, data in assets.items()}
        files[ASSETS + "build.json"] = json.dumps(metadata, sort_keys=True).encode()
        for name in JNI_LIBRARIES:
            path = "jni/arm64-v8a/" + name
            files[path] = source.read(path)
        for name in ("libs/chaquopy_java-17.2.0.jar", "META-INF/com/android/build/gradle/aar-metadata.properties"):
            files[name] = source.read(name)

        files["AndroidManifest.xml"] = b'<manifest package="org.air2mi.app.pythonruntime"/>\n'
        files["R.txt"] = b""
        files["classes.jar"] = empty_zip()
        rules = source.read("proguard.txt").splitlines()
        files["proguard.txt"] = b"\n".join(line for line in rules if not line.startswith(b"#") and b"org.air2mi.engine" not in line) + b"\n"

    stream = io.BytesIO()
    with zipfile.ZipFile(stream, "w") as output:
        for name, data in sorted(files.items()):
            entry = zipfile.ZipInfo(name)
            entry.compress_type = zipfile.ZIP_DEFLATED
            entry.external_attr = 0o644 << 16
            output.writestr(entry, data)
    data = stream.getvalue()
    audit(data)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_bytes(data)
    report = {"file": output_path.name, "sha256": hashlib.sha256(data).hexdigest(),
              "size": len(data), "contains_engine_core": False,
              "runtime": "Chaquopy Python 3.11 / crypto dependencies only"}
    output_path.with_suffix(output_path.suffix + ".json").write_text(json.dumps(report, indent=2) + "\n")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("source", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    prepare(args.source, args.output)
