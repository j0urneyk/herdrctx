#!/usr/bin/env python3
"""Exercise the installer with local download fixtures and a native CLI binary."""

import hashlib
import io
import os
from pathlib import Path
import platform
import shutil
import subprocess
import sys
import tarfile
import tempfile
import unittest


REPO = Path(__file__).resolve().parent.parent
NATIVE_BINARY = REPO / "bin/herdrctx"
TARGETS = {
    ("Darwin", "x86_64"): "macos_x86_64",
    ("Darwin", "arm64"): "macos_aarch64",
    ("Linux", "x86_64"): "linux_x86_64",
    ("Linux", "aarch64"): "linux_aarch64",
}


class InstallerTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="hctx-install-")
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.checkout = self.root / "checkout with spaces"
        (self.checkout / "scripts").mkdir(parents=True)
        shutil.copyfile(REPO / "scripts/install.sh", self.checkout / "scripts/install.sh")
        self.manifest = self.checkout / "herdr-plugin.toml"
        self.version("0.0.2")
        self.tools = self.root / "tools"
        self.tools.mkdir()
        self.downloads = self.root / "downloads"
        self.downloads.mkdir()
        self.scratch = self.root / "tmp"
        self.scratch.mkdir()
        self.destination = self.root / "installed bin/herdrctx"
        self.log = self.root / "urls.txt"
        self.env = dict(os.environ)
        self.env.update(PATH=f"{self.tools}{os.pathsep}{os.environ['PATH']}",
                        HERDRCTX_INSTALL_DIR=str(self.destination.parent),
                        TMPDIR=str(self.scratch),
                        FIXTURE_DIR=str(self.downloads), FIXTURE_LOG=str(self.log),
                        FIXTURE_OS="Linux", FIXTURE_ARCH="x86_64", FIXTURE_FAIL="")
        self.executable("uname", '#!/bin/sh\ncase "$1" in -s) echo "$FIXTURE_OS";; -m) echo "$FIXTURE_ARCH";; esac\n')
        self.executable("curl", f"#!{sys.executable}\n" + '''
import os, pathlib, shutil, sys
args = sys.argv[1:]
url = args[args.index('-o') - 1]
with open(os.environ['FIXTURE_LOG'], 'a') as log:
    log.write(url + '\\n')
prefix = 'https://github.com/j0urneyk/herdrctx/releases/download/'
assert url.startswith(prefix), url
if os.environ['FIXTURE_FAIL']:
    sys.exit(22)
source = pathlib.Path(os.environ['FIXTURE_DIR']) / url.removeprefix(prefix)
if not source.is_file():
    sys.exit(22)
shutil.copyfile(source, args[args.index('-o') + 1])
''')

    def executable(self, name, text):
        path = self.tools / name
        path.write_text(text)
        path.chmod(0o755)

    def version(self, version):
        self.manifest.write_text(f'version = "{version}"\n[[build]]\ncommand = ["sh", "scripts/install.sh"]\n')

    def fixture(self, version="0.0.2", target="linux_x86_64", content=None, member="herdrctx"):
        directory = self.downloads / f"v{version}"
        directory.mkdir(exist_ok=True)
        asset = directory / f"herdrctx_{version}_{target}.tar.gz"
        if content is None:
            content = f"#!/bin/sh\nprintf 'herdrctx {version}\\n'\n".encode()
        with tarfile.open(asset, "w:gz") as archive:
            entry = tarfile.TarInfo(member)
            entry.mode = 0o755
            entry.size = len(content)
            archive.addfile(entry, io.BytesIO(content))
        checksums = directory / "checksums.txt"
        with checksums.open("a") as output:
            output.write(f"{hashlib.sha256(asset.read_bytes()).hexdigest()}  {asset.name}\n")
        return asset, checksums

    def install(self, success=True):
        result = subprocess.run(["sh", str(self.checkout / "scripts/install.sh")],
                                env=self.env, cwd=self.root, capture_output=True, text=True, timeout=20)
        if success:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertTrue(os.access(self.destination, os.X_OK))
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(list(self.destination.parent.glob(".herdrctx.*")), [])
        self.assertEqual(list(self.scratch.iterdir()), [])
        self.assertEqual(self.env.get("HOME"), os.environ.get("HOME"))
        return result

    def test_platforms_reinstall_and_version_replacement(self):
        for (system, arch), target in TARGETS.items():
            with self.subTest(target=target):
                self.env.update(FIXTURE_OS=system, FIXTURE_ARCH=arch)
                for version in ("0.0.2", "1.2.3-rc.1"):
                    self.fixture(version, target)
                    self.version(version)
                    for _ in range(2):
                        output = self.install()
                        self.assertIn(f"Installed herdrctx {version}", output.stdout)
                        self.assertIn(f"/v{version}/herdrctx_{version}_{target}.tar.gz", self.log.read_text())
                        self.assertEqual(subprocess.check_output([str(self.destination), "--version"], text=True).strip(),
                                         f"herdrctx {version}")

    def test_failures_preserve_existing_binary(self):
        asset, checksums = self.fixture()
        original_archive, original_checksums = asset.read_bytes(), checksums.read_text()
        self.destination.parent.mkdir()
        self.destination.write_bytes(b"existing binary")
        for failure in ("download", "checksum download", "missing checksum", "duplicate checksum",
                        "checksum mismatch", "invalid archive", "missing binary"):
            with self.subTest(failure=failure):
                asset.write_bytes(original_archive)
                checksums.write_text(original_checksums)
                self.env["FIXTURE_FAIL"] = "1" if failure == "download" else ""
                if failure == "checksum download":
                    checksums.unlink()
                elif failure == "missing checksum":
                    checksums.write_text(original_checksums.replace(asset.name, "another.tar.gz"))
                elif failure == "duplicate checksum":
                    checksums.write_text(original_checksums * 2)
                elif failure == "checksum mismatch":
                    checksums.write_text(f"{'0' * 64}  {asset.name}\n")
                elif failure == "invalid archive":
                    asset.write_bytes(b"invalid tar")
                    checksums.write_text(f"{hashlib.sha256(asset.read_bytes()).hexdigest()}  {asset.name}\n")
                elif failure == "missing binary":
                    checksums.unlink()
                    self.fixture(member="README.md")
                result = self.install(success=False)
                self.assertIn("herdrctx install:", result.stderr)
                self.assertEqual(self.destination.read_bytes(), b"existing binary")

    def test_invalid_manifest_fails_before_download(self):
        for manifest in ('', 'version = "latest"', 'version = "../1.2.3"', 'version = "01.2.3"',
                         'version = "1.2.3-01"', 'version = "1.2.3-rc.01"',
                         'version = "1.2.3"\nversion = "1.2.4"', 'version = 123',
                         '[[build]]\nversion = "1.2.3"'):
            with self.subTest(manifest=manifest):
                self.manifest.write_text(manifest)
                self.assertIn("version", self.install(success=False).stderr)
                self.assertFalse(self.log.exists())
        self.manifest.unlink()
        self.assertIn("Missing manifest", self.install(success=False).stderr)

    def test_unsupported_platform_and_unwritable_destination(self):
        self.env["FIXTURE_OS"] = "FreeBSD"
        self.assertIn("Supported platforms", self.install(success=False).stderr)
        self.env["FIXTURE_OS"] = "Linux"
        self.destination.parent.mkdir(mode=0o555)
        try:
            self.assertIn("not writable", self.install(success=False).stderr)
        finally:
            self.destination.parent.chmod(0o755)
        self.assertFalse(self.log.exists())

    def test_path_advice(self):
        self.fixture()
        self.executable("herdrctx", "#!/bin/sh\nexit 0\n")
        result = self.install()
        self.assertIn("Add " + str(self.destination.parent) + " to PATH", result.stdout)
        self.assertIn(f"PATH currently selects {self.tools}/herdrctx", result.stdout)
        self.env["PATH"] = f"{self.destination.parent}{os.pathsep}{self.env['PATH']}"
        result = self.install()
        self.assertNotIn("Add ", result.stdout)
        self.assertNotIn("PATH currently selects", result.stdout)

    def test_native_binary_version_and_help(self):
        self.assertTrue(NATIVE_BINARY.is_file(), "Build bin/herdrctx before running this test.")
        system, arch = platform.system(), platform.machine()
        self.env.update(FIXTURE_OS=system, FIXTURE_ARCH=arch)
        target = TARGETS[(system, arch)]
        self.fixture(target=target, content=NATIVE_BINARY.read_bytes())
        self.install()
        expected = subprocess.check_output([str(NATIVE_BINARY), "--version"], text=True)
        result = subprocess.check_output([str(self.destination), "--version"], text=True)
        self.assertEqual(result, expected)
        help_output = subprocess.check_output([str(self.destination), "--help"], stderr=subprocess.STDOUT, text=True)
        self.assertIn("Usage: herdrctx [flags]", help_output)


if __name__ == "__main__":
    unittest.main()
