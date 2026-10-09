#!/usr/bin/env python3
"""Run only the MySQL driver smoke test against a new private, networkless mysqld.
No system service management, sudo, real credentials, shared sockets or data dirs.
Artifacts stay under maskriver/.local for diagnosis; server always stops on exit.
"""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time


def main():
    root = Path(__file__).resolve().parents[1]
    local = root / ".local"
    local.mkdir(exist_ok=True)
    for executable in ("mysqld", "mysqladmin", "go"):
        if shutil.which(executable) is None:
            raise SystemExit(f"BLOCKED: {executable} unavailable; no services changed")
    run = Path(tempfile.mkdtemp(prefix="mysql-m1a-", dir=local))
    run.chmod(0o700)
    (run / "maskriver-fixture-only").write_text("synthetic-only\n")
    data = run / "data"  # mysqld --initialize creates this fresh directory.
    tmp = run / "tmp"
    tmp.mkdir(mode=0o700)
    socket = run / "mysql.sock"
    if len(str(socket).encode()) > 100:
        raise SystemExit("BLOCKED: Unix socket path too long")
    base = ["mysqld", "--no-defaults", f"--datadir={data}",
            f"--tmpdir={tmp}", "--skip-log-bin", "--innodb-buffer-pool-size=32M"]
    server = None
    print(f"Disposable artifacts: {run}", flush=True)
    with (run / "initialize.log").open("w") as log:
        result = subprocess.run(base + ["--initialize-insecure"], stdout=log,
                                stderr=subprocess.STDOUT, timeout=90, cwd=root)
    if result.returncode:
        raise SystemExit("BLOCKED: isolated mysqld initialization failed; see initialize.log")
    try:
        with (run / "server.log").open("w") as log:
            server = subprocess.Popen(base + ["--skip-networking", "--mysqlx=OFF",
                f"--socket={socket}", f"--pid-file={run / 'mysql.pid'}",
                "--secure-file-priv=NULL", "--local-infile=OFF"],
                stdout=log, stderr=subprocess.STDOUT, cwd=root)
            ready = False
            for _ in range(120):
                if server.poll() is not None:
                    break
                if socket.exists():
                    ping = subprocess.run(["mysqladmin", "--no-defaults", "--protocol=socket",
                        f"--socket={socket}", "--user=root", "ping"],
                        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=3)
                    if ping.returncode == 0:
                        ready = True
                        break
                time.sleep(0.25)
            if not ready:
                raise SystemExit("BLOCKED: isolated MySQL not ready; see server.log")
            env = os.environ.copy()
            env["MASKRIVER_TEST_MYSQL_SOCKET"] = str(socket)
            result = subprocess.run(["go", "test", "-count=1", "-v", "./internal/testenv",
                "-run", "^TestMySQLFixture$"], cwd=root, env=env, timeout=180)
            if result.returncode:
                raise SystemExit(result.returncode)
    finally:
        if server is not None and server.poll() is None:
            server.terminate()
            try:
                server.wait(timeout=30)
            except subprocess.TimeoutExpired:
                server.kill()
                server.wait(timeout=10)
        print("Isolated mysqld stopped; no persistent service configured.", flush=True)


if __name__ == "__main__":
    main()
