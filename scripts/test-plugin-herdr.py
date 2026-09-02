#!/usr/bin/env python3
"""Check plugin registration, removal, and nested TUI guards in isolated Herdr storage."""

import argparse
import json
import os
from pathlib import Path
import runpy
import shutil
import subprocess
import tempfile
import time


REPO = Path(__file__).resolve().parent.parent
Terminal = runpy.run_path(str(REPO / "scripts/test-herdr-integration.py"))["Terminal"]


def run(args):
    herdr = str(args.herdr.resolve(strict=True))
    binary = args.binary.resolve(strict=True)
    artifacts = REPO / "dist/integration"
    artifacts.mkdir(parents=True, exist_ok=True)
    artifacts = Path(tempfile.mkdtemp(prefix="plugin-", dir=artifacts))
    server = None
    with tempfile.TemporaryDirectory(prefix="hctx-plugin-", dir="/tmp") as directory:
        root = Path(directory).resolve()
        for name in ("config/herdr", "data", "state", "runtime", "bin", "tmp"):
            (root / name).mkdir(parents=True)
        config = root / "config/herdr/config.toml"
        config.write_text('onboarding = false\n[terminal]\ndefault_shell = "/bin/sh"\n[ui.sound]\nenabled = false\n')
        env = {"PATH": os.environ["PATH"], "HOME": os.environ["HOME"],
               "XDG_CONFIG_HOME": str(root / "config"), "XDG_DATA_HOME": str(root / "data"),
               "XDG_STATE_HOME": str(root / "state"), "XDG_RUNTIME_DIR": str(root / "runtime"),
               "HERDR_CONFIG_PATH": str(config), "HERDRCTX_INSTALL_DIR": str(root / "bin"),
               "TMPDIR": str(root / "tmp"), "TERM": "xterm-256color", "LANG": "en_US.UTF-8",
               "HERDR_DISABLE_SOUND": "1", "SHELL": "/bin/sh"}
        installed = root / "bin/herdrctx"

        def cli(*command, timeout=15):
            result = subprocess.run([herdr, *command], env=env, cwd=root,
                                    capture_output=True, text=True, timeout=timeout)
            with (artifacts / "commands.log").open("a") as log:
                log.write(f"{command!r}: exit {result.returncode}\n{result.stdout}{result.stderr}\n")
            result.check_returncode()
            return result.stdout

        def sessions():
            entries = json.loads(cli("session", "list", "--json"))["sessions"]
            for entry in entries:
                for field in ("session_dir", "socket_path"):
                    if not Path(entry[field]).resolve().is_relative_to(root):
                        raise AssertionError(f"Non-isolated {field}: {entry[field]}")
            return entries

        def plugins():
            return json.loads(cli("plugin", "list", "--json"))["result"]["plugins"]

        try:
            version = cli("--version").strip()
            entries = sessions()
            assert not any(entry["running"] for entry in entries), entries
            assert not plugins(), "Expected an empty isolated registry"
            print(f"Testing {version} with isolated storage at {root}", flush=True)

            if args.source_ref:
                # This path exercises a real public GitHub checkout and its build command.
                cli("plugin", "install", "j0urneyk/herdrctx", "--ref", args.source_ref, "--yes", timeout=300)
                assert installed.is_file(), "The build command did not install the binary"
                assert not any(entry["running"] for entry in sessions()), "Install started a server"
            else:
                if args.start_server:
                    with (artifacts / "server.log").open("w") as log:
                        server = subprocess.Popen([herdr, "server"], env=env, cwd=root,
                                                  stdout=log, stderr=subprocess.STDOUT)
                    deadline = time.monotonic() + 15
                    while not any(entry["running"] for entry in sessions()):
                        if server.poll() is not None or time.monotonic() >= deadline:
                            raise AssertionError("Isolated Herdr server did not start")
                        time.sleep(0.1)
                cli("plugin", "link", str(REPO))
                assert not installed.exists(), "plugin link unexpectedly ran the installer"
                shutil.copy2(binary, installed)

            entries = plugins()
            assert len(entries) == 1, entries
            plugin = entries[0]
            assert plugin["plugin_id"] == "herdrctx", plugin
            assert plugin["build"] == [{"command": ["sh", "scripts/install.sh"]}], plugin
            for key in ("panes", "actions", "startup", "events", "link_handlers"):
                assert not plugin.get(key), plugin
            config_dir = Path(cli("plugin", "config-dir", "herdrctx").strip()).resolve()
            assert config_dir.is_relative_to(root), config_dir
            assert (root / "state/herdr/plugins/herdrctx").is_dir()
            installed_version = subprocess.check_output([str(installed), "--version"], text=True).strip()
            if args.source_ref:
                assert installed_version == f"herdrctx {plugin['version']}", installed_version
            output = subprocess.check_output([str(installed), "--help"], stderr=subprocess.STDOUT, text=True)
            assert "Usage: herdrctx [flags]" in output

            # Both signals must block attach and both create shortcuts before launching Herdr.
            for signal in ({"HERDR_ENV": "1"}, {"HERDR_SOCKET_PATH": sessions()[0]["socket_path"]}):
                for key, title in ((b"a", "Cannot attach from inside Herdr"),
                                   (b"n", "Cannot create from inside Herdr"),
                                   (b"N", "Cannot create from inside Herdr")):
                    with (artifacts / f"nested-{next(iter(signal))}-{key.decode()}.log").open("wb") as log:
                        terminal = Terminal([str(installed), "--herdr-bin", herdr],
                                            {**env, **signal}, root, log)
                        try:
                            terminal.expect("Loaded 1 session(s).")
                            start = len(terminal.output)
                            terminal.send(key)
                            terminal.expect(title, start)
                            terminal.send(b"\rq")
                            terminal.wait("normal TUI exit", terminal.exited)
                            assert terminal.status == 0, terminal.status
                        finally:
                            terminal.close()

            before = installed.read_bytes()
            managed_root = Path(plugin["plugin_root"])
            cli("plugin", "uninstall", "herdrctx")
            assert not plugins(), "Plugin remains registered"
            assert installed.read_bytes() == before, "Uninstall changed the external binary"
            if args.source_ref:
                assert managed_root.is_relative_to(root), managed_root
                assert not managed_root.exists(), "Managed checkout remains after uninstall"
            else:
                assert (REPO / "herdr-plugin.toml").is_file(), "Local checkout was removed"
            (artifacts / "result.json").write_text(json.dumps({
                "herdr": version, "binary": installed_version, "source_ref": args.source_ref,
                "plugin": plugin, "passed": True,
            }, indent=2) + "\n")
            print("PASS: registration, build-only manifest, CLI, nested guards, uninstall", flush=True)
        finally:
            if server is not None:
                if server.poll() is None:
                    try:
                        cli("server", "stop")
                    finally:
                        try:
                            server.wait(timeout=10)
                        except subprocess.TimeoutExpired:
                            server.kill()
                            server.wait(timeout=5)
            print(f"Artifacts: {artifacts}", flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--herdr", type=Path, default=Path("bin/herdr-plugin-ci"))
    parser.add_argument("--binary", type=Path, default=Path("bin/herdrctx"))
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--start-server", action="store_true", help="required for local link on Herdr 0.7.0")
    mode.add_argument("--source-ref", help="install a published Git ref instead of linking the working tree")
    run(parser.parse_args())
