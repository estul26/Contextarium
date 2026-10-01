#!/usr/bin/env python3
"""Disposable, loopback-only M2/M1/M0 binary checks. This is not an R3 test.

Requires three already-built binaries. Never opens a caller's runtime database.
All processes and data are owned by this invocation; logs remain in memory.
"""
import argparse
import contextlib
import http.client
import json
import os
from pathlib import Path
import shutil
import signal
import socket
import sqlite3
import subprocess
import tempfile
import time


def request(port, method, path, body=None, key=None, headers=None):
    conn = http.client.HTTPConnection("127.0.0.1", port, timeout=5)
    fields = {"Content-Type": "application/json"}
    if key:
        fields["Idempotency-Key"] = key
    fields.update(headers or {})
    try:
        conn.request(method, path, body=body.encode("utf-8") if isinstance(body, str) else body, headers=fields)
        reply = conn.getresponse()
        raw = reply.read()
        return reply.status, json.loads(raw) if raw else None, dict(reply.getheaders())
    finally:
        conn.close()


def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def assert_private(logs, database):
    for forbidden in (str(database), "9007199254740993", "synthetic-private-marker", "Bearer ", "INSERT INTO"):
        assert forbidden not in logs, "runtime log leaked private input"


@contextlib.contextmanager
def running(binary, database):
    port = free_port()
    env = dict(os.environ, CONTEXTARIUM_LISTEN_ADDR=f"127.0.0.1:{port}", CONTEXTARIUM_DB_PATH=str(database))
    process = subprocess.Popen([str(binary)], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    try:
        deadline = time.monotonic() + 12
        while time.monotonic() < deadline:
            assert process.poll() is None, "binary exited before readiness"
            try:
                if request(port, "GET", "/readyz")[0] == 200:
                    break
            except (OSError, http.client.HTTPException):
                time.sleep(0.02)
        else:
            raise AssertionError("binary readiness timeout")
        yield port
    finally:
        if process.poll() is None:
            process.send_signal(signal.SIGTERM)
        try:
            out, err = process.communicate(timeout=8)
        except subprocess.TimeoutExpired:
            process.kill()  # Cleanup only this invocation's child; not durability evidence.
            process.communicate()
            raise AssertionError("graceful shutdown timed out")
        assert process.returncode == 0, "binary did not shut down gracefully"
        assert_private((out + err).decode(), database)


def mutation(port, method, path, body, key, expected):
    status, result, headers = request(port, method, path, body, key)
    assert status == expected, f"unexpected HTTP status {status}"
    assert headers.get("Cache-Control") == "no-store"
    assert result["meta"]["request_id"] == headers["X-Request-Id"]
    return result


def stores(database):
    # Separate read connection + one deferred transaction for all four stores.
    with contextlib.closing(sqlite3.connect(database)) as db:
        db.execute("BEGIN")
        value = tuple(tuple(db.execute(f"SELECT * FROM {table} ORDER BY 1,2")) for table in
                      ("records", "record_revisions", "mutation_audit", "idempotency_keys"))
        assert db.execute("PRAGMA integrity_check").fetchone() == ("ok",)
        assert db.execute("PRAGMA foreign_key_check").fetchall() == []
        db.rollback()
        return value


def copy_closed(source, destination):
    # sqlite3's connection context manager does not close the connection. Every
    # observer above uses closing(); refuse to copy an outstanding WAL anyway.
    wal = Path(str(source) + "-wal")
    assert not wal.exists() or wal.stat().st_size == 0, "source still has WAL content"
    shutil.copy2(source, destination)


def refuse(binary, database):
    before = stores(database)
    port = free_port()
    env = dict(os.environ, CONTEXTARIUM_LISTEN_ADDR=f"127.0.0.1:{port}", CONTEXTARIUM_DB_PATH=str(database))
    result = subprocess.run([str(binary)], env=env, capture_output=True, timeout=12)
    assert result.returncode != 0
    logs = (result.stdout + result.stderr).decode()
    assert "application ready" not in logs
    assert_private(logs, database)
    assert stores(database) == before, "old binary modified M2 stores"
    with socket.socket() as sock:
        assert sock.connect_ex(("127.0.0.1", port)) != 0, "old binary opened listener"


def check(binary, m1, m0):
    with tempfile.TemporaryDirectory(prefix="contextarium-m2-binary-") as directory:
        root = Path(directory)
        database = root / "synthetic.db"
        # Genuine M1 binary creates active + archived data, with historical replay
        # snapshots different from the state that M2 will adopt.
        with running(m1, database) as port:
            subject_body = '{"kind":"project","display_name":"Synthetic project"}'
            subject = mutation(port, "POST", "/api/v1/subjects", subject_body, "subject", 201)
            schema_body = '{"schema_id":"example.exact","schema_version":1,"definition":{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"},"migration_policy":"explicit"}'
            schema = mutation(port, "POST", "/api/v1/schemas", schema_body, "schema", 201)
            data = '{"n":9007199254740993,"d":0.30,"e":1e2,"z":-0,"unknown":["雪",true],"low":1e-308,"high":1e308,"large":99999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999}'
            body = '{"subject_id":' + json.dumps(subject["data"]["id"]) + ',"namespace":"example.records","schema_id":"example.exact","schema_version":1,"data":' + data + '}'
            created = mutation(port, "POST", "/api/v1/records", body, "legacy-create", 201)
            other = mutation(port, "POST", "/api/v1/records", body, "other", 201)
            path = "/api/v1/records/" + created["data"]["id"]
            patch = '{"key":"synthetic-private-marker","status":"archived"}'
            old_patch = mutation(port, "PATCH", path, patch, "legacy-patch", 200)
            adopted = mutation(port, "PATCH", path, '{"sensitivity":"restricted"}', "later-m1", 200)
        # A closed database copy is an explicit rollback baseline, not a live-file backup.
        baseline = root / "closed-m1.db"
        copy_closed(database, baseline)
        with contextlib.closing(sqlite3.connect(database)) as db:
            old_records = db.execute("SELECT * FROM records ORDER BY id").fetchall()
            old_keys = db.execute("SELECT * FROM idempotency_keys ORDER BY scope,key").fetchall()
            old_ledger = db.execute("SELECT * FROM schema_migrations ORDER BY version").fetchall()
        with running(binary, database) as port:
            current = request(port, "GET", path)[1]["data"]
            assert current == dict(adopted["data"], revision=1)
            revision = request(port, "GET", path + "/revisions/1")[1]["data"]
            assert revision["snapshot"] == adopted["data"]
            assert revision["operation"] == "m1_adoption"
            assert revision["actor"] == {"kind": "development_test", "id": "local-migration-003"}
            assert revision["recorded_at"] != revision["snapshot"]["updated_at"]
            with contextlib.closing(sqlite3.connect(database)) as db:
                assert [row[:-1] for row in db.execute("SELECT * FROM records ORDER BY id")] == old_records
                assert [row[:-1] for row in db.execute("SELECT * FROM idempotency_keys ORDER BY scope,key")] == old_keys
                assert db.execute("SELECT * FROM schema_migrations WHERE version<3 ORDER BY version").fetchall() == old_ledger
                assert db.execute("SELECT count(*) FROM record_revisions").fetchone() == (2,)
                assert all(row[0] == data for row in db.execute("SELECT data FROM record_revisions"))
            before = stores(database)
            for route, verb, key, payload, original, status in (("/api/v1/records", "POST", "legacy-create", body, created, 201), (path, "PATCH", "legacy-patch", patch, old_patch, 200)):
                replay = mutation(port, verb, route, payload, key, status)
                assert replay["data"] == original["data"] and replay["meta"]["result_contract"] == "m1"
            assert stores(database) == before
            assert mutation(port, "POST", "/api/v1/subjects", subject_body, "subject", 201)["data"] == subject["data"]
            assert mutation(port, "POST", "/api/v1/schemas", schema_body, "schema", 201)["data"] == schema["data"]
            new_patch = '{"base_revision":1,"status":"active"}'
            changed = mutation(port, "PATCH", path, new_patch, "m2-patch", 200)
            restore_body = '{"base_revision":2}'
            restored = mutation(port, "POST", path + "/restore/1", restore_body, "m2-restore", 200)
            assert restored["data"]["revision"] == 3 and restored["data"]["status"] == "archived"
            fresh = mutation(port, "POST", "/api/v1/records", body, "m2-create", 201)
            fresh_path = "/api/v1/records/" + fresh["data"]["id"]
            mutation(port, "PATCH", fresh_path, '{"base_revision":1,"key":null}', "later-fresh", 200)
            mutation(port, "PATCH", path, '{"base_revision":3,"key":null}', "later-m2", 200)
            assert request(port, "PATCH", path, patch, "unknown")[0] == 400
            assert request(port, "PATCH", path, '{"base_revision":4,"status":"archived"}', "legacy-patch")[0] == 409
            for suffix, method in (("/revisions", "GET"), ("/revisions/1", "GET"), ("/restore/1", "POST")):
                for headers in ({"Host": "hostile.example"}, {"Origin": "https://hostile.example"}, {"Sec-Fetch-Site": "cross-site"}):
                    assert request(port, method, path + suffix, restore_body if method == "POST" else None, "security", headers)[0] == 403
            probe_before = stores(database)
            for route in ("/healthz", "/readyz"):
                assert request(port, "GET", route)[0] == 200
                assert request(port, "HEAD", route)[0] == 200
            assert stores(database) == probe_before
        # Restart exercises successful replay with stale bases and fresh request IDs.
        before = stores(database)
        with running(binary, database) as port:
            for route, verb, key, payload, original, status, contract in (
                ("/api/v1/records", "POST", "legacy-create", body, created, 201, "m1"),
                (path, "PATCH", "legacy-patch", patch, old_patch, 200, "m1"),
                ("/api/v1/records", "POST", "m2-create", body, fresh, 201, "m2"),
                (path, "PATCH", "m2-patch", new_patch, changed, 200, "m2"),
                (path + "/restore/1", "POST", "m2-restore", restore_body, restored, 200, "m2"),
            ):
                replay = mutation(port, verb, route, payload, key, status)
                assert replay["data"] == original["data"] and replay["meta"]["result_contract"] == contract
                assert replay["meta"]["request_id"] != original["meta"]["request_id"]
            assert stores(database) == before
        evidence = root / "closed-m2-evidence.db"
        copy_closed(database, evidence)
        refuse(m1, database)
        refuse(m0, database)
        rollback = root / "explicit-m1-rollback.db"
        copy_closed(baseline, rollback)
        with running(m1, rollback) as port:
            assert request(port, "GET", path)[1]["data"] == adopted["data"]
            assert request(port, "GET", fresh_path)[0] == 404  # Later M2 changes are omitted.
        assert stores(evidence) == before  # M2 evidence is preserved independently.
        # Fresh startup remains functional; unsafe binding fails before creating a DB.
        with running(binary, root / "fresh.db") as port:
            assert request(port, "GET", "/api/v1/records")[1]["data"] == []
        unsafe = root / "must-not-exist.db"
        env = dict(os.environ, CONTEXTARIUM_LISTEN_ADDR="0.0.0.0:0", CONTEXTARIUM_DB_PATH=str(unsafe))
        result = subprocess.run([str(binary)], env=env, capture_output=True, timeout=5)
        assert result.returncode != 0 and not unsafe.exists()
    print("PASS: compiled M1 adoption, M1/M2 replay after restart, exact numbers, revision/restore APIs, loopback/Host/Origin, read-only probes, SIGTERM shutdown, M0/M1 refusal, explicit closed-copy rollback. R3 NOT RUN.")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("binary", "m1-binary", "m0-binary"):
        parser.add_argument("--" + name, type=lambda value: Path(value).resolve(), required=True)
    args = parser.parse_args()
    check(args.binary, args.m1_binary, args.m0_binary)
