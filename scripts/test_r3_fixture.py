"""Fake-only fixture regressions. Prepared for separate approval; NOT RUN.

Ordinary Go CI does not invoke these tests. Loading the controller here never
calls main; all HTTP, socket, process and wait dependencies are replaced.
"""
import contextlib
import importlib.util
import io
import json
from pathlib import Path
import signal
import subprocess
import tempfile
import unittest
from unittest import mock


SENTINEL = 'PRIVATE_SENTINEL_DO_NOT_LOG'


class FakeSocket:
    def __enter__(self): return self
    def __exit__(self, *args): return False
    def bind(self, address): self.address = address
    def getsockname(self): return ('127.0.0.1', 43123)


class FakeResponse:
    def __init__(self, status=200, data=None, raw=None):
        self.status = status
        self.raw = raw if raw is not None else json.dumps(data or {}).encode()
        self.read_sizes = []

    def read(self, size):
        self.read_sizes.append(size)
        return self.raw[:size]


class FakeHTTP:
    def __init__(self):
        self.calls = []; self.connections = []; self.overrides = {}
        self.close_error = None

    def __call__(self, host, port, timeout):
        owner = self
        class Connection:
            def request(self, method, url, body, headers):
                self.url = url
                owner.calls.append((method, url, body, headers.copy()))

            def getresponse(self):
                answer = owner.overrides.get(self.url)
                if isinstance(answer, BaseException): raise answer
                if answer is not None: return answer
                if self.url == '/api/v1/subjects':
                    return FakeResponse(201, {'data':{'id':'sub_00000000000000000000000000000001'}})
                if self.url == '/api/v1/records':
                    return FakeResponse(201, {'data':{'id':'rec_00000000000000000000000000000001'}})
                return FakeResponse(200, {'data':{}})

            def close(self):
                if owner.close_error is not None: raise owner.close_error
        conn = Connection()
        self.connections.append((host, port, timeout, conn))
        return conn


class FakeProcess:
    def __init__(self):
        self.returncode = None; self.close_code = 0
        self.signals = []; self.communicate_timeouts = []
        self.errors = []; self.signal_error = None
        self.output = b''; self.stderr_output = b''
        self.returned_output = (None, None)

    def launch(self, command, *, env, stdout, stderr):
        self.command = command; self.env = env
        stdout.write(self.output); stderr.write(self.stderr_output)
        stdout.flush(); stderr.flush()
        return self

    def poll(self): return self.returncode

    def send_signal(self, value):
        if self.signal_error is not None: raise self.signal_error
        self.signals.append(value)

    def kill(self):
        self.signals.append(signal.SIGKILL)
        self.returncode = -signal.SIGKILL

    def communicate(self, *, timeout):
        self.communicate_timeouts.append(timeout)
        if self.errors: raise self.errors.pop(0)
        if self.returncode is None: self.returncode = self.close_code
        return self.returned_output


class FixtureTests(unittest.TestCase):
    def setUp(self):
        # Only executes if this test suite is separately authorized and run.
        source = Path(__file__).with_name('r3-check.py')
        spec = importlib.util.spec_from_file_location('r3_fixture_test_controller', source)
        self.controller = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.controller)
        self.http = FakeHTTP(); self.process = FakeProcess()
        self.stack = contextlib.ExitStack()
        self.addCleanup(self.stack.close)
        self.root = Path(self.stack.enter_context(tempfile.TemporaryDirectory(prefix='contextarium-fixture-unit-')))
        (self.root/self.controller.MARKER).write_text('synthetic test ownership\n')
        self.output = io.StringIO()
        self.stack.enter_context(contextlib.redirect_stdout(self.output))
        self.stack.enter_context(mock.patch.object(self.controller.http.client, 'HTTPConnection', self.http))
        self.stack.enter_context(mock.patch.object(self.controller.socket, 'socket', return_value=FakeSocket()))
        self.launch = self.stack.enter_context(mock.patch.object(self.controller.subprocess, 'Popen', side_effect=self.process.launch))
        self.stack.enter_context(mock.patch.object(self.controller.time, 'sleep'))

    def call(self, empty=False):
        return self.controller.m1_fixture(Path('synthetic-m1-binary'), self.root, empty=empty)

    def records(self):
        return [json.loads(line) for line in self.output.getvalue().splitlines()]

    def closing(self):
        return next(row for row in self.records() if row['kind']=='m1-fixture-close')

    def failure(self):
        return next(row for row in self.records() if row['kind']=='m1-fixture-failure')

    def test_http_client_resolution_and_intended_populated_requests(self):
        legacy = self.call()
        self.assertEqual(len(self.http.connections), 6)
        self.assertTrue(all(item[:3]==('127.0.0.1',43123,2) for item in self.http.connections))
        self.assertEqual([item[0] for item in self.http.calls], ['GET','POST','POST','POST','POST','PATCH'])
        self.assertEqual([r['Key'] for r in legacy], ['legacy-create','legacy-update'])
        self.assertIn('9007199254740993', legacy[0]['Body'])
        self.assertIn('"d":0.30,"e":1e2,"z":-0', legacy[0]['Body'])
        self.assertEqual(self.http.calls[-1][2], b'{"status":"archived"}')
        self.assertEqual(self.http.calls[-1][3]['Idempotency-Key'], 'legacy-update')
        self.assertEqual(self.process.signals, [signal.SIGTERM])
        self.assertEqual(self.closing()['result'], 'PASS')

    def test_primary_failure_survives_cleanup_timeout(self):
        primary = AttributeError(SENTINEL)
        self.http.overrides['/api/v1/subjects'] = primary
        self.process.errors = [subprocess.TimeoutExpired('synthetic-worker',10,output=SENTINEL.encode(),stderr=SENTINEL.encode())]
        with self.assertRaises(AttributeError) as got: self.call()
        self.assertIs(got.exception, primary)
        self.assertEqual(self.failure()['stage'], 'subject')
        close = self.closing()
        self.assertTrue(close['primary_preserved'])
        self.assertIn('fixture-close-timeout', [e['error_code'] for e in close['cleanup_errors']])
        self.assertEqual(self.process.signals, [signal.SIGTERM,signal.SIGKILL])
        self.assertEqual(self.process.communicate_timeouts, [10,5])
        self.assertNotIn(SENTINEL, self.output.getvalue())

    def test_primary_failure_survives_http_close_failure(self):
        primary = ValueError(SENTINEL)
        self.http.overrides['/readyz'] = primary
        self.http.close_error = OSError(SENTINEL)
        with self.assertRaises(ValueError) as got: self.call()
        self.assertIs(got.exception, primary)
        self.assertEqual(self.failure()['stage'], 'readiness')
        self.assertIn('fixture-http-close',[e['error_code'] for e in self.closing()['cleanup_errors']])

    def test_cleanup_only_timeout_fails(self):
        self.process.errors = [subprocess.TimeoutExpired('synthetic-worker',10)]
        with self.assertRaises(self.controller.FixtureError) as got: self.call(empty=True)
        self.assertEqual(got.exception.code, 'fixture-close-timeout')
        self.assertFalse(self.closing()['primary_preserved'])
        self.assertEqual(self.closing()['result'], 'FAIL')

    def test_cleanup_only_signal_failure_fails(self):
        self.process.signal_error = OSError(SENTINEL)
        with self.assertRaises(self.controller.FixtureError) as got: self.call(empty=True)
        self.assertEqual(got.exception.code, 'fixture-signal-failed')
        self.assertFalse(self.closing()['controller_termination_performed'])

    def test_readiness_timeout_is_bounded_and_reports_stage(self):
        self.http.overrides['/readyz'] = TimeoutError(SENTINEL)
        with mock.patch.object(self.controller.time, 'monotonic', side_effect=[0,11]):
            with self.assertRaises(self.controller.FixtureError) as got: self.call()
        self.assertEqual(got.exception.code, 'fixture-readiness-timeout')
        self.assertEqual(self.failure()['stage'], 'readiness')
        self.assertEqual(len(self.http.calls), 1)
        self.assertLess(len(self.output.getvalue()), 12000)
        self.assertNotIn(SENTINEL, self.output.getvalue())

    def test_early_exit_is_natural_and_not_controller_termination(self):
        self.process.returncode = 7
        self.process.stderr_output = (json.dumps({'msg':'application failed','level':'ERROR','private':SENTINEL})+'\n').encode()
        with self.assertRaises(self.controller.FixtureError) as got: self.call()
        self.assertEqual(got.exception.code, 'fixture-child-exited')
        self.assertEqual(self.http.calls, [])
        close = self.closing()
        self.assertEqual(close['natural_exit'], {'exit_code':7,'signal':None})
        self.assertFalse(close['controller_termination_performed'])
        self.assertEqual(close['outputs']['stderr']['records'], [{'code':'application-failed','level':'ERROR'}])
        self.assertEqual(close['outputs']['stderr']['bytes'], len(self.process.stderr_output))

    def test_empty_fixture_clean_close_is_checked(self):
        self.assertEqual(self.call(empty=True), [])
        self.assertEqual([c[1] for c in self.http.calls], ['/readyz'])
        self.assertEqual(self.process.communicate_timeouts, [10])
        self.assertEqual(self.closing()['observed_exit'], {'exit_code':0,'signal':None})
        self.assertEqual(self.closing()['result'], 'PASS')

    def test_empty_fixture_wal_postcondition_is_not_skipped(self):
        (self.root/'store.db-wal').write_bytes(b'synthetic')
        with self.assertRaises(self.controller.FixtureError) as got: self.call(empty=True)
        self.assertEqual(got.exception.code, 'fixture-wal-remains')
        self.assertEqual(self.process.communicate_timeouts, [10])

    def test_populated_fixture_wal_postcondition_is_not_skipped(self):
        (self.root/'store.db-wal').write_bytes(b'synthetic')
        with self.assertRaises(self.controller.FixtureError) as got: self.call()
        self.assertEqual(got.exception.code, 'fixture-wal-remains')
        self.assertEqual(len(self.http.calls), 6)
        self.assertEqual(self.process.communicate_timeouts, [10])

    def test_empty_fixture_nonzero_close_fails(self):
        self.process.close_code = 3
        with self.assertRaises(self.controller.FixtureError) as got: self.call(empty=True)
        self.assertEqual(got.exception.code, 'fixture-exit-nonzero')

    def test_populated_fixture_nonzero_close_fails(self):
        self.process.close_code = 3
        with self.assertRaises(self.controller.FixtureError) as got: self.call()
        self.assertEqual(got.exception.code, 'fixture-exit-nonzero')

    def test_http_status_and_allowlisted_api_code_without_body(self):
        response = FakeResponse(422, {'error':{'code':'SCHEMA_VALIDATION_FAILED','message':SENTINEL}})
        self.http.overrides['/api/v1/schemas'] = response
        with self.assertRaises(self.controller.FixtureError) as got: self.call()
        self.assertEqual(got.exception.code, 'fixture-http-status')
        self.assertEqual(self.failure()['stage'], 'schema')
        self.assertEqual(self.failure()['http']['status'], 422)
        self.assertEqual(self.failure()['http']['api_error_code'], 'SCHEMA_VALIDATION_FAILED')
        self.assertEqual(response.read_sizes, [128*1024+1])
        self.assertNotIn(SENTINEL, self.output.getvalue())

    def test_sensitive_output_is_bounded_and_not_published(self):
        line = json.dumps({'msg':'application ready','level':'INFO','private':SENTINEL})+'\n'
        self.process.output = self.process.stderr_output = (line*2000+SENTINEL+'\n').encode()
        self.process.returned_output = (self.process.output,self.process.stderr_output)
        response = FakeResponse(400, {'error':{'code':SENTINEL,'message':SENTINEL}})
        self.http.overrides['/api/v1/subjects'] = response
        try: self.call()
        except self.controller.FixtureError as error: self.controller.report_controller_failure(error)
        else: self.fail('HTTP rejection missing')
        public = self.output.getvalue()
        self.assertNotIn(SENTINEL, public)
        self.assertNotIn(str(self.root), public)
        self.assertLess(len(public), 20000)
        self.assertEqual(self.failure()['http']['api_error_code'], 'redacted')
        close = self.closing()
        for name in ('stdout','stderr'):
            capture = close['outputs'][name]
            self.assertEqual(capture['sha256'],self.controller.digest(self.process.output))
            self.assertEqual(capture['bytes'],len(self.process.output))
            self.assertLessEqual(capture['tail_bytes'],65536)
            self.assertLessEqual(len(capture['records']),8)
            self.assertTrue(capture['tail_truncated'])
            self.assertEqual(close['communication'][0][name]['bytes'],len(self.process.output))
        final = self.records()[-1]
        self.assertEqual(final['kind'],'FINAL')
        self.assertEqual(final['stage'],'subject')
        self.assertEqual(final['error_code'],'fixture-http-status')
        self.assertTrue(final['location'])
        self.assertTrue(final['error_id'].startswith('fixture-http-status:'))

    def test_launch_failure_is_reported_without_private_path(self):
        primary = OSError(SENTINEL)
        self.launch.side_effect = primary
        with self.assertRaises(OSError) as got: self.call()
        self.assertIs(got.exception, primary)
        self.assertEqual(self.failure()['stage'], 'launch')
        self.assertNotIn(SENTINEL, self.output.getvalue())

    def test_close_reporting_failure_cannot_replace_primary(self):
        primary = ValueError(SENTINEL)
        self.http.overrides['/api/v1/subjects'] = primary
        real_log = self.controller.log
        def broken_log(**fields):
            if fields['kind']=='m1-fixture-close': raise OSError(SENTINEL)
            real_log(**fields)
        with mock.patch.object(self.controller,'log',side_effect=broken_log):
            with self.assertRaises(ValueError) as got: self.call()
        self.assertIs(got.exception, primary)


if __name__ == '__main__':
    unittest.main()
