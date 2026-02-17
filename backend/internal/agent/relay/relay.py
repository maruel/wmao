#!/usr/bin/env python3
# Persistent relay for coding agent processes inside caic containers.
#
# Modes:
#   serve-attach --dir <path> -- <cmd...>   Start relay daemon + attach as first client.
#   attach [--offset N]                     Reconnect to a running relay daemon.
#   read-plan [path]                        Read a plan file from the container.
#
# The relay daemon owns the subprocess stdin/stdout, logs all I/O to
# output.jsonl, and accepts one client at a time via a Unix socket.
#
# Shutdown protocol — null-byte sentinel:
#   The Go backend (Session.Close) writes a \x00 byte to stdin *before*
#   closing it. The attach_client forwards this through the socket to the
#   daemon, which closes proc.stdin, letting the agent exit gracefully.
#
#   Crucially, stdin EOF alone does NOT trigger shutdown. This is what
#   distinguishes the two flows below.
#
# Flow 1 — One task is terminated (user clicks "terminate"):
#   1. Server calls Runner.Cleanup → Session.Close
#   2. Session.Close writes \x00 then closes stdin
#   3. attach_client forwards \x00 through the socket, then disconnects
#   4. Relay daemon receives \x00, closes proc.stdin
#   5. Agent emits final ResultMessage and exits (code=0)
#   6. Server waits up to 10s for exit, then kills the container
#
# Flow 2 — Backend restarts (upgrade, crash):
#   1. SSH connections are severed, attach_client sees stdin EOF
#   2. attach_client disconnects from the socket (no \x00 sent)
#   3. Relay daemon stays alive, subprocess keeps running
#   4. On restart, server discovers the container via adoptOne()
#   5. Server reads output.jsonl to restore conversation state
#   6. Server calls relay.py attach --offset N to reconnect
#   7. Task resumes seamlessly with zero message loss

import json
import logging
import os
import socket
import subprocess
import sys
import threading
import time

RELAY_DIR = os.environ.get("CAIC_RELAY_DIR", "/tmp/caic-relay")
SOCK_PATH = os.path.join(RELAY_DIR, "relay.sock")
OUTPUT_PATH = os.path.join(RELAY_DIR, "output.jsonl")
PID_PATH = os.path.join(RELAY_DIR, "pid")

# Max size of a single read from subprocess stdout.
BUF_SIZE = 65536


def serve(cmd_args, work_dir):
    """Start the relay server as a daemon, then attach as the first client.

    Architecture:
      Parent process → waits for socket → attach_client() (bridges stdio)
      Child process (daemon):
        1. Starts subprocess (claude/gemini) with piped stdin/stdout.
        2. reader_thread: subprocess stdout → output.jsonl + connected client.
        3. accept_thread: accepts client connections on Unix socket.
           - On connect: replays output.jsonl from offset, then forwards live.
           - client_reader: client stdin → subprocess stdin.
        4. When subprocess exits:
           - reader_thread closes output file and disconnects client.
           - Socket and PID file are cleaned up.

    Failure modes handled:
      - SSH drops: client disconnects, subprocess keeps running. Next
        attach reconnects from the offset where the client left off.
      - Subprocess crash: reader_thread exits, client sees EOF.
        Socket is cleaned up so IsRelayRunning returns false.
      - Graceful shutdown: client sends null byte → relay closes proc.stdin.
    """
    os.makedirs(RELAY_DIR, exist_ok=True)

    # Clean up stale socket.
    try:
        os.unlink(SOCK_PATH)
    except FileNotFoundError:
        pass

    # Fork to become a daemon.
    pid = os.fork()
    if pid > 0:
        # Parent: wait for socket to appear, then attach.
        _wait_for_socket(30)
        attach_client(offset=0)
        return

    # Child: become session leader so we survive SSH disconnects.
    os.setsid()

    # Set up logging to relay.log. This replaces the old stderr redirect so
    # that key lifecycle events are always recorded for diagnostics.
    log_path = os.path.join(RELAY_DIR, "relay.log")
    logging.basicConfig(
        filename=log_path,
        filemode="w",
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(message)s",
        datefmt="%Y-%m-%dT%H:%M:%S",
    )
    # Also capture stray stderr writes (e.g. from tracebacks).
    log_fd = os.open(log_path, os.O_WRONLY | os.O_APPEND)
    os.dup2(log_fd, 2)
    os.close(log_fd)

    # Write PID file.
    with open(PID_PATH, "w") as f:
        f.write(str(os.getpid()))

    logging.info("relay daemon started pid=%d cmd=%s cwd=%s", os.getpid(), cmd_args, work_dir)
    _start_time = time.monotonic()

    # Start the subprocess in the working directory that contains the git
    # repository so the harness (claude, gemini) picks up the right project.
    proc = subprocess.Popen(
        cmd_args,
        cwd=work_dir,
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
    )
    logging.info("subprocess started pid=%d", proc.pid)

    # Open output log (append-only).
    output_file = open(OUTPUT_PATH, "ab", buffering=0)

    # Track connected client (at most one at a time).
    client_lock = threading.Lock()
    client_conn = [None]  # mutable ref in list
    client_id = [0]  # monotonic client counter
    stdin_closed = [False]  # True once proc.stdin has been closed

    def set_client(conn, reason=""):
        with client_lock:
            old = client_conn[0]
            client_conn[0] = conn
            if old is not None:
                logging.info("client #%d disconnected reason=%s", client_id[0], reason)
                try:
                    old.close()
                except OSError:
                    pass

    def send_to_client(data):
        with client_lock:
            c = client_conn[0]
            if c is None:
                return
            try:
                c.sendall(data)
            except (BrokenPipeError, ConnectionResetError, OSError):
                client_conn[0] = None

    # Thread: read subprocess stdout → log + forward to client.
    def reader_thread():
        try:
            while True:
                data = proc.stdout.read1(BUF_SIZE)
                if not data:
                    break
                output_file.write(data)
                output_file.flush()
                send_to_client(data)
        except (OSError, ValueError) as e:
            logging.warning("reader_thread error: %s", e)
        finally:
            sz = output_file.tell() if not output_file.closed else -1
            output_file.close()
            # Process exited — close client.
            set_client(None, "subprocess_eof")
            logging.info("reader_thread exited output_bytes=%d", sz)

    t = threading.Thread(target=reader_thread, daemon=True)
    t.start()

    # Listen on Unix socket for client connections.
    srv = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    srv.bind(SOCK_PATH)
    srv.listen(1)

    # Thread: accept client connections.
    def accept_thread():
        while True:
            try:
                conn, _ = srv.accept()
            except OSError:
                break

            # Read handshake: {"offset": N}\n
            try:
                hdr = _read_line(conn)
                hs = json.loads(hdr)
                offset = hs.get("offset", 0)
            except (json.JSONDecodeError, OSError, ValueError):
                conn.close()
                continue

            # Replay output from offset.
            try:
                with open(OUTPUT_PATH, "rb") as f:
                    f.seek(offset)
                    while True:
                        chunk = f.read(BUF_SIZE)
                        if not chunk:
                            break
                        conn.sendall(chunk)
            except (OSError, BrokenPipeError):
                conn.close()
                continue

            client_id[0] += 1
            cid = client_id[0]
            set_client(conn, "replaced")
            logging.info(
                "client #%d connected offset=%d stdin_closed=%s proc_alive=%s",
                cid,
                offset,
                stdin_closed[0],
                proc.poll() is None,
            )

            # Thread: read client stdin → subprocess stdin + log.
            def client_reader(c, cid=cid):
                close_stdin = False
                try:
                    while True:
                        data = c.recv(BUF_SIZE)
                        if not data:
                            logging.info("client #%d recv returned empty (EOF/disconnect)", cid)
                            break
                        # A null byte signals the client wants proc.stdin closed
                        # (graceful termination). Strip it and set the flag.
                        if b"\x00" in data:
                            data = data.replace(b"\x00", b"")
                            close_stdin = True
                            if data:
                                proc.stdin.write(data)
                                proc.stdin.flush()
                                output_file.write(data)
                                output_file.flush()
                            break
                        proc.stdin.write(data)
                        proc.stdin.flush()
                        output_file.write(data)
                        output_file.flush()
                except (OSError, BrokenPipeError, ValueError) as e:
                    logging.info("client #%d reader error: %s", cid, e)
                if close_stdin:
                    if stdin_closed[0]:
                        logging.warning("client #%d requested stdin close but stdin already closed", cid)
                    else:
                        stdin_closed[0] = True
                        logging.info("client #%d requested stdin close", cid)
                        try:
                            proc.stdin.close()
                        except OSError:
                            pass

            ct = threading.Thread(target=client_reader, args=(conn,), daemon=True)
            ct.start()

    at = threading.Thread(target=accept_thread, daemon=True)
    at.start()

    # Wait for subprocess to exit.
    proc.wait()
    elapsed = time.monotonic() - _start_time
    logging.info("subprocess exited code=%d stdin_closed=%s elapsed=%.0fs", proc.returncode, stdin_closed[0], elapsed)
    t.join(timeout=5)

    # Clean up.
    srv.close()
    try:
        os.unlink(SOCK_PATH)
    except FileNotFoundError:
        pass
    try:
        os.unlink(PID_PATH)
    except FileNotFoundError:
        pass


def attach_client(offset):
    """Connect to relay via Unix socket and bridge to stdio."""
    conn = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    conn.connect(SOCK_PATH)

    # Send handshake.
    hs = json.dumps({"offset": offset}) + "\n"
    conn.sendall(hs.encode())

    # Thread: relay socket → stdout.
    def relay_to_stdout():
        try:
            while True:
                data = conn.recv(BUF_SIZE)
                if not data:
                    break
                sys.stdout.buffer.write(data)
                sys.stdout.buffer.flush()
        except (OSError, BrokenPipeError, ValueError):
            pass
        finally:
            # When relay closes, signal EOF to our parent.
            try:
                sys.stdout.close()
            except OSError:
                pass

    t = threading.Thread(target=relay_to_stdout, daemon=True)
    t.start()

    # Main thread: stdin → socket.
    # The null byte sentinel for graceful shutdown is written by the Go
    # backend *before* closing stdin, so it arrives through the normal data
    # path. We must NOT inject a synthetic null byte on stdin EOF because
    # EOF also happens on SSH drops / backend restarts where the container
    # should keep running.
    try:
        while True:
            data = sys.stdin.buffer.read1(BUF_SIZE)
            if not data:
                break
            conn.sendall(data)
    except (OSError, BrokenPipeError, ValueError, KeyboardInterrupt):
        pass
    finally:
        conn.close()
        t.join(timeout=5)


def _wait_for_socket(timeout):
    """Block until the relay socket appears."""
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if os.path.exists(SOCK_PATH):
            # Try connecting to verify it's ready.
            try:
                s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
                s.connect(SOCK_PATH)
                s.close()
                return
            except OSError:
                pass
        time.sleep(0.05)
    print("relay: timed out waiting for socket", file=sys.stderr)
    sys.exit(1)


def _read_line(conn):
    """Read bytes from conn until newline."""
    buf = bytearray()
    while True:
        b = conn.recv(1)
        if not b or b == b"\n":
            break
        buf.extend(b)
    return buf.decode()


def read_plan(path):
    """Print the content of a plan file.

    If path is given, read that file directly. Otherwise find the most recently
    modified .md file in ~/.claude/plans/.
    """
    if path:
        with open(path) as f:
            sys.stdout.write(f.read())
        return
    plans_dir = os.path.expanduser("~/.claude/plans")
    if not os.path.isdir(plans_dir):
        sys.exit(1)
    files = [os.path.join(plans_dir, f) for f in os.listdir(plans_dir) if f.endswith(".md")]
    if not files:
        sys.exit(1)
    latest = max(files, key=os.path.getmtime)
    with open(latest) as f:
        sys.stdout.write(f.read())


def main():
    if len(sys.argv) < 2:
        print("usage: relay.py serve-attach --dir <path> -- <cmd...>", file=sys.stderr)
        print("       relay.py attach [--offset N]", file=sys.stderr)
        print("       relay.py read-plan [path]", file=sys.stderr)
        sys.exit(1)

    mode = sys.argv[1]

    if mode == "serve-attach":
        # Parse required --dir flag.
        rest = sys.argv[2:]
        if len(rest) < 2 or rest[0] != "--dir":
            print("relay.py serve-attach: --dir <path> is required", file=sys.stderr)
            sys.exit(1)
        work_dir = rest[1]
        rest = rest[2:]
        # Find "--" separator.
        try:
            sep = rest.index("--")
        except ValueError:
            print("relay.py serve-attach: missing '--' separator", file=sys.stderr)
            sys.exit(1)
        cmd_args = rest[sep + 1 :]
        if not cmd_args:
            print("relay.py serve-attach: no command after '--'", file=sys.stderr)
            sys.exit(1)
        serve(cmd_args, work_dir)

    elif mode == "attach":
        offset = 0
        if "--offset" in sys.argv:
            idx = sys.argv.index("--offset")
            if idx + 1 < len(sys.argv):
                offset = int(sys.argv[idx + 1])
        attach_client(offset)

    elif mode == "read-plan":
        read_plan(sys.argv[2] if len(sys.argv) > 2 else None)

    else:
        print(f"relay.py: unknown mode {mode!r}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
