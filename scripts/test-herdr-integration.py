#!/usr/bin/env python3
"""Exercise the real TUI and Herdr 0.6.5 through a Unix pseudo-terminal."""

import argparse
import errno
import fcntl
import json
import os
from pathlib import Path
import platform
import pty
import re
import select
import shutil
import signal
import struct
import subprocess
import sys
import tempfile
import termios
import time
import traceback
import uuid


ANSI = re.compile(rb"\x1b\].*?(?:\x07|\x1b\\)|\x1b\[[0-?]*[ -/]*[@-~]", re.DOTALL)


def process_running(pid):
    result = subprocess.run(["/bin/ps", "-o", "stat=", "-p", str(pid)],
                            capture_output=True, text=True, timeout=5)
    if result.returncode not in (0, 1):
        result.check_returncode()
    return result.returncode == 0 and not result.stdout.strip().startswith("Z")


class Terminal:
    def __init__(self, command, env, cwd, log):
        self.output = bytearray()
        self.log = log
        self.status = None
        self.pid, self.fd = pty.fork()
        if self.pid == 0:
            try:
                os.chdir(cwd)
                fcntl.ioctl(0, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 160, 0, 0))
                os.execve(command[0], command, env)
            except BaseException:
                os._exit(127)
        self.query_offset = 0

    def read(self, timeout=0.1):
        if not select.select([self.fd], [], [], timeout)[0]:
            return
        try:
            chunk = os.read(self.fd, 65536)
        except OSError as error:
            if error.errno != errno.EIO:
                raise
            chunk = b""
        self.output.extend(chunk)
        self.log.write(chunk)
        self.log.flush()
        if len(self.output) > 8 * 1024 * 1024:
            raise AssertionError("PTY output exceeded 8 MiB")
        # Answer terminal capability queries without enabling extended key protocols.
        queries = {b"\x1b[6n": b"\x1b[1;1R", b"\x1b[c": b"\x1b[?1;2c", b"\x1b[?u": b"\x1b[?0u"}
        start = max(0, self.query_offset - 8)
        for query, response in queries.items():
            offset = start
            while (offset := self.output.find(query, offset)) != -1:
                end = offset + len(query)
                if end > self.query_offset:
                    self.send(response)
                offset = end
        self.query_offset = len(self.output)

    def send(self, data):
        os.write(self.fd, data)

    def exited(self):
        if self.status is None:
            pid, status = os.waitpid(self.pid, os.WNOHANG)
            if pid:
                self.status = os.waitstatus_to_exitcode(status)
        return self.status is not None

    def wait(self, description, predicate, timeout=20):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            self.read()
            if predicate():
                return
            if self.exited():
                raise AssertionError(f"TUI exited ({self.status}) while waiting for {description}")
        raise TimeoutError(f"Timed out waiting for {description}")

    def expect(self, text, since=0):
        self.wait(text, lambda: text.encode() in ANSI.sub(b"", self.output[since:]))

    def close(self):
        try:
            if not self.exited():
                os.killpg(self.pid, signal.SIGTERM)
                deadline = time.monotonic() + 3
                while not self.exited() and time.monotonic() < deadline:
                    self.read()
        finally:
            try:
                if not self.exited():
                    os.killpg(self.pid, signal.SIGKILL)
                    os.waitpid(self.pid, 0)
            finally:
                os.close(self.fd)


def run(args):
    artifacts = args.artifacts.resolve()
    artifacts.mkdir(parents=True, exist_ok=True)
    artifacts = Path(tempfile.mkdtemp(prefix="run-", dir=artifacts))
    herdr = str(args.herdr.resolve(strict=True))
    binary = str(args.binary.resolve(strict=True))
    root = Path(tempfile.mkdtemp(prefix="hctx-", dir="/tmp")).resolve()
    name = "ci-" + uuid.uuid4().hex[:12]
    terminal = None
    failed = False
    pane_pid = None
    try:
        for directory in ("home", "config", "work", "tmp"):
            (root / directory).mkdir()
        (root / "config/config.toml").write_text(
            'onboarding = false\n[terminal]\ndefault_shell = "/bin/sh"\nnew_cwd = "current"\n'
            '[ui.sound]\nenabled = false\n', encoding="utf-8"
        )
        env = {
            "PATH": "/usr/bin:/bin", "HOME": str(root / "home"),
            "XDG_CONFIG_HOME": str(root / "home/.config"),
            "HERDR_CONFIG_PATH": str(root / "config/config.toml"),
            "TMPDIR": str(root / "tmp"), "TERM": "xterm-256color",
            "LANG": "en_US.UTF-8", "SHELL": "/bin/sh", "PS1": "hctx-shell> ",
            "HERDR_DISABLE_SOUND": "1",
        }

        def cli(*command):
            result = subprocess.run([herdr, *command], env=env, cwd=root / "work",
                                    capture_output=True, text=True, timeout=10)
            with (artifacts / "commands.log").open("a", encoding="utf-8") as log:
                log.write(f"{command!r}: exit {result.returncode}\n{result.stdout}{result.stderr}\n")
            result.check_returncode()
            return result.stdout

        version = cli("--version").strip()
        (artifacts / "environment.json").write_text(json.dumps({
            "herdr": version, "os": platform.platform(), "arch": platform.machine(),
            "python": platform.python_version(), "root": str(root), "session": name,
            "terminal": "xterm-256color, 160x40",
        }, indent=2) + "\n", encoding="utf-8")
        if version != "herdr 0.6.5":
            raise AssertionError(f"Expected herdr 0.6.5, got {version!r}")

        def sessions():
            raw = cli("session", "list", "--json")
            (artifacts / "sessions.json").write_text(raw, encoding="utf-8")
            entries = json.loads(raw)["sessions"]
            for entry in entries:
                for field in ("session_dir", "socket_path"):
                    if not Path(entry[field]).resolve().is_relative_to(root):
                        raise AssertionError(f"Non-isolated {field}: {entry[field]}")
            return entries

        def session_state():
            return next((entry for entry in sessions() if entry["name"] == name), None)

        def is_running():
            state = session_state()
            return state is not None and state["running"]

        def is_stopped():
            state = session_state()
            return state is not None and not state["running"]

        sessions()
        with (artifacts / "pty.log").open("wb") as log:
            terminal = Terminal([binary, "--herdr-bin", herdr, "--interval", "500ms"],
                                env, root / "work", log)
            try:
                terminal.expect("Loaded 1 session(s).")
                start = len(terminal.output)
                terminal.send(b"n")
                terminal.expect("Enter to create and attach.", start)
                terminal.send(name.encode() + b"\r")
                terminal.wait("running session", is_running)
                terminal.expect("No workspaces yet", start)
                # Herdr starts a new session empty; its normal shortcut opens a shell workspace.
                terminal.send(b"\x02N")
                terminal.expect("hctx-shell>", start)

                for visit in (1, 2):
                    marker = f"HCTX_{uuid.uuid4().hex}"
                    start = len(terminal.output)
                    # The expected marker is absent from the typed command's echo.
                    command = (f"printf '%s\\n' \"$$\" > '{root}/work/pane.pid'; "
                               f"printf '%s%s\\n' '{marker[:12]}' '{marker[12:]}'\r")
                    terminal.send(command.encode())
                    terminal.expect(marker, start)
                    terminal.wait("shell PID", lambda: (root / "work/pane.pid").exists())
                    current_pid = int((root / "work/pane.pid").read_text().strip())
                    if pane_pid is not None and current_pid != pane_pid:
                        raise AssertionError("Reattach did not preserve the shell process")
                    pane_pid = current_pid
                    if args.fail_after_attach and visit == 1:
                        raise AssertionError("Injected failure after attach")
                    start = len(terminal.output)
                    terminal.send(b"\x02q")
                    terminal.expect("herdrctx", start)
                    terminal.expect(name, start)
                    terminal.wait("session surviving detach", is_running)
                    if visit == 1:
                        start = len(terminal.output)
                        terminal.send(b"/" + name.encode() + b"\r")
                        terminal.expect(name, start)
                        terminal.send(b"\r")
                        terminal.expect(marker, start)

                start = len(terminal.output)
                terminal.send(b"s")
                terminal.expect(f'Stop session "{name}"?', start)
                if not is_running():
                    raise AssertionError("Session stopped before confirmation")
                terminal.send(b"\r")
                terminal.wait("stopped session", is_stopped)
                terminal.wait("shell process exit", lambda: not process_running(pane_pid))
                terminal.expect("stopped", start)
                start = len(terminal.output)
                terminal.send(b"d")
                terminal.expect(f'Delete session "{name}"?', start)
                if not is_stopped():
                    raise AssertionError("Session deleted before confirmation")
                terminal.send(b"\r")
                terminal.wait("deleted session", lambda: session_state() is None)
                terminal.expect("No sessions match", start)
                terminal.send(b"q")
                terminal.wait("normal TUI exit", terminal.exited)
                if terminal.status != 0:
                    raise AssertionError(f"TUI exit status: {terminal.status}")
                print("PASS: create, attach, detach, reattach, stop, delete, quit", flush=True)
            finally:
                terminal.close()
                terminal = None
    except BaseException:
        failed = True
        (artifacts / "failure.txt").write_text(traceback.format_exc(), encoding="utf-8")
        raise
    finally:
        try:
            if failed:
                try:
                    snapshot = artifacts / "sessions.json"
                    if snapshot.exists():
                        shutil.copyfile(snapshot, artifacts / "sessions-before-cleanup.json")
                    for source in root.rglob("*"):
                        if source.is_file():
                            target = artifacts / "herdr-logs" / source.relative_to(root)
                            target.parent.mkdir(parents=True, exist_ok=True)
                            shutil.copyfile(source, target)
                except OSError as error:
                    print(f"Could not copy all diagnostic files: {error}", file=sys.stderr)
            if "sessions" in locals():
                # The CLI environment and session paths were verified as test-owned.
                state = session_state()
                if state is not None:
                    if state["running"]:
                        cli("session", "stop", name, "--json")
                    cli("session", "delete", name, "--json")
                if session_state() is not None:
                    raise AssertionError("Test session remains after cleanup")
                if pane_pid is not None:
                    deadline = time.monotonic() + 5
                    while process_running(pane_pid) and time.monotonic() < deadline:
                        time.sleep(0.1)
                    if process_running(pane_pid):
                        raise AssertionError(f"Test shell process {pane_pid} remains after cleanup")
            (artifacts / "cleanup.json").write_text(json.dumps({
                "session": name, "session_removed": True, "pane_pid": pane_pid,
                "shell_exited": pane_pid is None or not process_running(pane_pid),
            }, indent=2) + "\n", encoding="utf-8")
        except BaseException:
            (artifacts / "cleanup-error.txt").write_text(traceback.format_exc(), encoding="utf-8")
            raise
        finally:
            shutil.rmtree(root)
            print(f"Artifacts: {artifacts}", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, default=Path("bin/herdrctx"))
    parser.add_argument("--herdr", type=Path, default=Path("bin/herdr-ci"))
    parser.add_argument("--artifacts", type=Path, default=Path("dist/integration"))
    parser.add_argument("--fail-after-attach", action="store_true",
                        help="exercise failure logging and cleanup with a live session")
    args = parser.parse_args()
    signal.signal(signal.SIGTERM, lambda *_: sys.exit("Integration test terminated"))
    run(args)


if __name__ == "__main__":
    main()
