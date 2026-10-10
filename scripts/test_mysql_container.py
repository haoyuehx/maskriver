#!/usr/bin/env python3
"""Run synthetic MySQL fixture in a disposable, network-disabled Linux container.

Requires an already authorized Docker daemon; never installs or starts one.
No arbitrary DSN, production credentials, host database or persistent volume.
"""
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import time
import uuid

IMAGE = "mysql:8.0.46"
ROOT = Path(__file__).resolve().parents[1]


def main():
    if os.name != "posix" or not hasattr(os, "getuid"):
        print("SKIP / UNVERIFIED: private Unix socket harness requires Linux; use Ubuntu CI")
        return 2
    if not shutil.which("docker"):
        print("SKIP / UNVERIFIED: Docker unavailable; use Ubuntu CI")
        return 2
    subprocess.run(["docker", "info"], check=True, stdout=subprocess.DEVNULL)
    local = ROOT / ".local"
    local.mkdir(exist_ok=True)
    directory = Path(tempfile.mkdtemp(prefix="mysql-m1a-", dir=local))
    directory.chmod(0o700)
    (directory / "maskriver-fixture-only").write_text("synthetic-only\n")
    socket = directory / "mysql.sock"
    if len(str(socket).encode()) > 100:
        print("UNVERIFIED: checkout path too long for Unix socket; use a shorter clone path")
        shutil.rmtree(directory)
        return 2
    name = "maskriver-fixture-" + uuid.uuid4().hex
    result = 1
    try:
        subprocess.run([
            "docker", "run", "--detach", "--name", name,
            "--network", "none", "--cap-drop", "ALL",
            "--security-opt", "no-new-privileges", "--user", f"{os.getuid()}:{os.getgid()}",
            "--mount", f"type=bind,src={directory},dst={directory}",
            "--entrypoint", "bash", IMAGE, "-ec",
            'mysqld --no-defaults --datadir="$1/data" --initialize-insecure; '
            'exec mysqld --no-defaults --datadir="$1/data" --socket="$1/mysql.sock" '
            '--pid-file="$1/mysql.pid" --skip-networking --mysqlx=OFF '
            '--local-infile=OFF --secure-file-priv=NULL',
            "maskriver", str(directory),
        ], check=True, stdout=subprocess.DEVNULL)
        # Record immutable image identity actually used; a version tag alone is not a digest.
        subprocess.run(["docker", "image", "inspect", IMAGE,
                        "--format", "{{json .RepoDigests}}"], check=True)
        deadline = time.monotonic() + 180
        while time.monotonic() < deadline:
            ready = subprocess.run([
                "docker", "exec", name, "mysqladmin", "--no-defaults",
                f"--socket={socket}", "--user=root", "ping",
            ], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            if ready.returncode == 0:
                break
            state = subprocess.check_output([
                "docker", "inspect", "--format", "{{.State.Running}}", name,
            ], text=True).strip()
            if state != "true":
                raise RuntimeError("isolated MySQL exited before readiness")
            time.sleep(2)
        else:
            raise RuntimeError("isolated MySQL readiness timeout")
        env = os.environ.copy()
        env["MASKRIVER_TEST_MYSQL_SOCKET"] = str(socket)
        # Only environment fixture exists today. Adapter integration remains #5/#10.
        result = subprocess.run([
            "go", "test", "-count=1", "-v", "./internal/testenv",
            "-run", "^TestMySQLFixture$",
        ], cwd=ROOT, env=env).returncode
    finally:
        with (directory / "container.log").open("w") as log:
            subprocess.run(["docker", "logs", name], stdout=log, stderr=subprocess.STDOUT)
        cleanup = subprocess.run(["docker", "rm", "--force", "--volumes", name],
                                 stdout=subprocess.DEVNULL)
        if cleanup.returncode != 0:
            raise RuntimeError("container cleanup failed; device owner must inspect")
        print("Disposable MySQL container removed; no host service was used")
        if result == 0:
            shutil.rmtree(directory)
        else:
            print("FAILED / UNVERIFIED: synthetic diagnostics retained under ignored .local")
    return result


if __name__ == "__main__":
    sys.exit(main())
