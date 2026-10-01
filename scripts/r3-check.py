#!/usr/bin/env python3
"""Bounded, runner-only R3 controller. No production switches or artifact upload."""
import argparse
import copy
from contextlib import contextmanager
import hashlib
import http.client
import json
import os
from pathlib import Path
import random
import re
import selectors
import shutil
import signal
import socket
import subprocess
import tempfile
import time

CANDIDATE = 'b6f63a7564977555faffe6f9ca6b1a9c22910d43'
SEEDS = (17, 29, 101)
TABLES = ('subjects', 'schemas', 'records', 'record_revisions', 'mutation_audit', 'idempotency_keys', 'schema_migrations', 'sqlite_schema')
SNAPSHOT = ('subject_id','namespace','schema_id','schema_version','data','key','sensitivity','provenance','status','created_at','updated_at')
MARKER = '.contextarium-r3-owned'
DEADLINE = time.monotonic() + 3300
IDENTITY = {'candidate': CANDIDATE, 'harness': 'unavailable-before-manifest'}
SETTINGS = ('sqlite_version()', 'sqlite_source_id()', 'foreign_keys', 'journal_mode',
            'synchronous', 'busy_timeout', 'wal_autocheckpoint', 'page_size',
            'mmap_size', 'compile_options', 'max_open_connections')
# Raw stderr/protocol/trace remain private scratch. Only this bounded allowlist
# reaches job logs; notably it contains no error strings, paths, SQL or page hex.
LABELS = set(('startup open fixture inspection close settings model-validation checkpoint-prepare '
              'create patch metadata archive unarchive noop restore migration fresh empty-m1 '
              'checkpoint autocheckpoint batch start complete failed pre post database wal journal '
              'write sync truncate delete operation-failed sql-no-rows sqlite-error '
              'path-boundary file-not-allowed metadata-write trace-write trace-flush protocol-write '
              'barrier-input-eof unexpected-barrier-release unknown-fault native-partial-write-failed '
              'native-write-failed unexpected-mapped-pointer phase-length phase-label '
              'decision-input-eof decision-reply-invalid').split())
LABELS.update('mutation-'+op for op in ('create','patch','metadata','archive','unarchive','noop','restore'))
DIAGNOSTIC_KINDS = {'worker-progress', 'worker-error', 'worker-fatal', 'vfs-operation', 'vfs-fatal'}
FIXTURE_STAGES = {'launch','readiness','subject','schema','record','update','close'}
FIXTURE_CODES = {'fixture-http-status','fixture-response-too-large','fixture-http-close',
                 'fixture-child-exited','fixture-readiness-timeout','fixture-close-timeout',
                 'fixture-kill-timeout','fixture-exit-nonzero','fixture-wal-remains',
                 'fixture-signal-failed','fixture-kill-failed','fixture-poll-failed',
                 'fixture-communicate-failed','fixture-cleanup-failed','fixture-capture-failed',
                 'fixture-stream-close','fixture-log-failed'}
M1_API_CODES = {'INVALID_ARGUMENT','NOT_FOUND','CONFLICT','SCHEMA_VALIDATION_FAILED',
                'UNAVAILABLE','INTERNAL_ERROR','FORBIDDEN','METHOD_NOT_ALLOWED',
                'PAYLOAD_TOO_LARGE','UNSUPPORTED_MEDIA_TYPE'}
M1_LOG_CODES = {'configuration rejected':'configuration-rejected',
                'database initialization failed':'database-initialization-failed',
                'database close failed':'database-close-failed',
                'listener initialization failed':'listener-initialization-failed',
                'application ready':'application-ready',
                'HTTP serving or shutdown failed':'http-serving-or-shutdown-failed',
                'application stopped':'application-stopped','application failed':'application-failed'}


class FixtureError(AssertionError):
    def __init__(self, code):
        assert code in FIXTURE_CODES
        self.code = code
        super().__init__(code)


def safe_failure(exc, stage='controller', code=None):
    """Public identifiers contain only fixed codes and this source's locations."""
    if code is None:
        code = 'controller-unexpected-error'
        for cls, label in ((FixtureError, 'fixture-error'), (subprocess.TimeoutExpired, 'controller-timeout'),
                           (TimeoutError, 'controller-timeout'), (json.JSONDecodeError, 'controller-malformed-json'),
                           (AttributeError, 'controller-attribute-error'), (KeyError, 'controller-missing-field'),
                           (StopIteration, 'controller-failed-lookup'), (AssertionError, 'controller-assertion'),
                           (OSError, 'controller-os-error'), (ValueError, 'controller-invalid-value')):
            if isinstance(exc, cls):
                code = exc.code if cls is FixtureError else label
                break
    else:
        assert code in FIXTURE_CODES
    known_types = {'FixtureError','AssertionError','AttributeError','KeyError','StopIteration',
                   'JSONDecodeError','TimeoutExpired','TimeoutError','CalledProcessError',
                   'ConnectionRefusedError','ConnectionResetError','BrokenPipeError','OSError',
                   'FileNotFoundError','PermissionError','ProcessLookupError','ValueError',
                   'TypeError','RuntimeError','HTTPException','RemoteDisconnected','IncompleteRead',
                   'KeyboardInterrupt','SystemExit'}
    locations = []
    tb = exc.__traceback__
    while tb is not None:
        frame = tb.tb_frame
        if frame.f_code.co_filename == __file__:
            name = frame.f_code.co_name
            # Never publish filenames, source text, exception messages or locals.
            if re.fullmatch(r'[A-Za-z_][A-Za-z_0-9]{0,63}', name):
                locations.append({'function':name,'line':tb.tb_lineno})
            if name == 'm1_fixture' and stage == 'controller':
                phase = frame.f_locals.get('failure_stage') or frame.f_locals.get('stage')
                if phase in FIXTURE_STAGES: stage = phase
        tb = tb.tb_next
    locations = locations[-4:]
    point = next((item for item in reversed(locations) if item['function'] not in ('require','__init__')),
                 {'function':'unknown','line':0})
    return {'stage':stage if stage in FIXTURE_STAGES | {'controller'} else 'redacted',
            'error_type':type(exc).__name__ if type(exc).__name__ in known_types else 'OtherError',
            'error_code':code, 'error_id':f'{code}:{point["function"]}:{point["line"]}',
            'location':locations}


def report_controller_failure(exc):
    log(kind='FINAL', **IDENTITY, result='FAIL', **safe_failure(exc),
        reason='safe code and source locations only; raw exception text withheld',
        T18='PARTIAL',T30='BLOCKED; incomplete harness evidence',R3='OPEN')


def m1_log_tail(raw):
    tail = raw[-65536:]; records = []; suppressed = 0
    for line in tail.splitlines():
        try: row = json.loads(line)
        except (ValueError, UnicodeError): row = None
        message = row.get('msg') if isinstance(row, dict) else None
        if not isinstance(message, str) or message not in M1_LOG_CODES:
            suppressed += 1
            continue
        # Even recognized lines may contain private extra fields; discard them.
        records.append({'code':M1_LOG_CODES[message],
                        'level':row.get('level') if row.get('level') in ('DEBUG','INFO','WARN','ERROR') else 'redacted'})
    return {'tail_bytes':len(tail),'tail_sha256':digest(tail),
            'records':records[-8:],'suppressed_tail_lines':suppressed}


def m1_output_bytes(raw):
    raw = raw or b''
    if isinstance(raw, str): raw = raw.encode('utf-8', errors='replace')
    return {'bytes':len(raw),'sha256':digest(raw),'tail_truncated':len(raw)>65536,**m1_log_tail(raw)}


def m1_output_file(stream, complete):
    stream.flush(); size = stream.seek(0, os.SEEK_END); stream.seek(0)
    checksum = hashlib.sha256(); remaining = size
    # Hash this finite size snapshot even if a child could not be terminated.
    while remaining:
        block = stream.read(min(65536,remaining))
        if not block:raise FixtureError('fixture-capture-failed')
        checksum.update(block); remaining -= len(block)
    stream.seek(max(0,size-65536)); tail = stream.read(min(size,65536))
    return {'bytes':size,'sha256':checksum.hexdigest(),'complete':complete,
            'tail_truncated':size>65536,**m1_log_tail(tail)}


def close_m1_child(child):
    """Collect process failures; the caller also guards diagnostic failures."""
    result = {'controller_termination_performed':False,'signals':[],
              'natural_exit':exit_status(None),'observed_exit':exit_status(None),
              'cleanup_errors':[],'communication':[]}
    def failed(action, exc, code):
        result['cleanup_errors'].append({'action':action,**safe_failure(exc,'close',code)})
    def poll():
        try: return child.poll()
        except BaseException as exc:
            failed('poll',exc,'fixture-poll-failed');return None
    result['natural_exit'] = exit_status(poll())
    if result['natural_exit'] == exit_status(None):
        try:
            child.send_signal(signal.SIGTERM)
            result['controller_termination_performed'] = True; result['signals'].append('SIGTERM')
        except BaseException as exc: failed('terminate',exc,'fixture-signal-failed')
    def communicate(seconds, timeout_code):
        try:
            output, stderr = child.communicate(timeout=seconds)
        except subprocess.TimeoutExpired as exc:
            result['communication'].append({'complete':False,'stdout':m1_output_bytes(exc.output),
                                            'stderr':m1_output_bytes(exc.stderr)})
            failed('communicate',exc,timeout_code);return False
        except BaseException as exc:
            failed('communicate',exc,'fixture-communicate-failed');return False
        result['communication'].append({'complete':True,'stdout':m1_output_bytes(output),
                                        'stderr':m1_output_bytes(stderr)})
        return True
    complete = communicate(10,'fixture-close-timeout')
    if not complete and poll() is None:
        try:
            child.kill()
            result['controller_termination_performed'] = True; result['signals'].append('SIGKILL')
        except BaseException as exc: failed('kill',exc,'fixture-kill-failed')
        communicate(5,'fixture-kill-timeout')
    result['observed_exit'] = exit_status(poll())
    return result


def safe_diagnostic(row):
    if not isinstance(row, dict) or not isinstance(row.get('kind'), str) or row['kind'] not in DIAGNOSTIC_KINDS:
        return None
    safe = {'kind': row['kind']}
    for key in ('phase', 'stage', 'op', 'role', 'reason'):
        if key in row:
            safe[key] = row[key] if isinstance(row[key], str) and row[key] in LABELS else 'redacted'
    if 'query' in row:
        safe['query'] = row['query'] if isinstance(row['query'], str) and row['query'] in (*SETTINGS, '') else 'redacted'
    for key in ('seq', 'offset', 'length', 'flags', 'rc', 'applied', 'code', 'extended_code'):
        if type(row.get(key)) is int and abs(row[key]) < 2**63:
            safe[key] = row[key]
    return safe


def stderr_summary(raw, total=None):
    # Read at most the last 64 KiB, publish at most eight allowlisted records.
    raw = raw[-65536:]
    records = []; suppressed = 0; last_pre = None; last_post = None
    progress = {}
    for line in raw.splitlines():
        try: row = safe_diagnostic(json.loads(line))
        except (ValueError, UnicodeError): row = None
        if row is None:
            suppressed += 1
            continue
        if row['kind'] == 'vfs-operation':
            if row.get('stage') == 'pre': last_pre = row
            else: last_post = row
        else:
            records.append(row)
            track_progress(progress, row)
    return dict(stderr_bytes=total if total is not None else len(raw),
                stderr_tail_sha256=digest(raw), stderr_tail=records[-8:],
                stderr_tail_truncated=(total or 0)>65536,
                suppressed_tail_lines=suppressed, last_vfs_pre=last_pre,
                last_vfs_post=last_post, **progress)


def track_progress(progress, row):
    if row.get('kind') != 'worker-progress': return
    progress['last_phase'] = row.get('phase')
    if row.get('query'): progress['last_settings_query'] = row['query']
    if row.get('stage') == 'complete':
        if row.get('query'): progress['last_completed_settings_query'] = row['query']
        else: progress['last_completed_phase'] = row.get('phase')


def exit_status(code):
    return {'exit_code': code if code is not None and code >= 0 else None,
            'signal': -code if code is not None and code < 0 else None}


def log_worker_failure(stage, exc, code, terminated, stdout, stderr, progress=None, **extra):
    # Exception text is deliberately excluded: OS/JSON errors can quote paths/data.
    log(kind='worker-failure', **IDENTITY, failure_stage=stage,
        error_type=type(exc).__name__, natural_exit=exit_status(code) if not terminated else exit_status(None),
        controller_termination_performed=terminated, observed_exit=exit_status(code),
        protocol=stdout, diagnostics=stderr, progress=progress or {}, **extra)


def cleanup_child(child, selector=None):
    try: return cleanup_child_actions(child, selector)
    except BaseException as exc:
        return {'controller_termination_performed':None, 'observed_exit':exit_status(None),
                'cleanup_errors':[{'action':'cleanup','error_type':type(exc).__name__}]}


def cleanup_child_actions(child, selector=None):
    # Always secondary to the original failure. No exception text in job logs.
    result = {'controller_termination_performed': False, 'cleanup_errors': [],
              'exit_before_cleanup': exit_status(child.poll())}
    actions = []
    if child.poll() is None:
        def terminate():
            child.kill()
            result['controller_termination_performed'] = True
        actions += [('kill', terminate), ('wait', lambda: child.wait(timeout=5))]
    actions += [(name, stream.close) for name, stream in (('stdin',child.stdin),('stdout',child.stdout),('stderr',child.stderr)) if stream]
    if selector: actions.append(('selector', selector.close))
    for name, action in actions:
        try: action()
        except BaseException as exc: result['cleanup_errors'].append({'action':name,'error_type':type(exc).__name__})
    result['observed_exit'] = exit_status(child.poll())
    return result


@contextmanager
def evidence_file(path):
    stream = path.open('wb'); primary = None
    try:
        yield stream
    except BaseException as exc:
        primary = exc
        raise
    finally:
        try: stream.close()
        except BaseException as exc:
            try: log(kind='evidence-close-failed', **IDENTITY, error_type=type(exc).__name__)
            except BaseException: pass
            if primary is None: raise

def require(value, message):
    if not value:
        raise AssertionError(message)

def digest(data):
    return hashlib.sha256(data).hexdigest()

def encoded(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=False).encode()

def precise(raw):
    return json.loads(raw, parse_int=lambda s: ('number', s), parse_float=lambda s: ('number', s))

def canonical(value):
    if isinstance(value,tuple) and value[0]=='number': return value[1]
    if isinstance(value,dict): return '{'+','.join(json.dumps(k,ensure_ascii=False)+':'+canonical(value[k]) for k in sorted(value))+'}'
    if isinstance(value,list): return '['+','.join(canonical(v) for v in value)+']'
    return json.dumps(value,ensure_ascii=False,separators=(',',':'))

def log(**fields):
    print(json.dumps(fields, sort_keys=True, separators=(',', ':')), flush=True)

def setting_value(query, value):
    # Bounded public representation, never arbitrary worker strings.
    if query == 'mmap_size' and value == {'supported':False,'reason':'vfs-control-notfound-no-row'}:
        return value
    if type(value) is int and 0 <= value < 2**63: return value
    if query == 'journal_mode' and value == 'wal': return value
    if query == 'sqlite_version()' and isinstance(value,str) and re.fullmatch(r'[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}',value): return value
    if query == 'sqlite_source_id()' and isinstance(value,str) and re.fullmatch(r'[0-9 :-]{19} [a-f0-9]{64}',value): return value
    if query == 'compile_options' and isinstance(value,list):
        return [x if isinstance(x,str) and len(x)<=128 and re.fullmatch(r'[A-Z0-9_]+(?:=-?[0-9]+)?',x) else 'redacted' for x in value[:256]]
    return 'redacted'


def log_setting(msg, case):
    require(msg.get('query') in SETTINGS and msg.get('status') in ('started','complete'), 'malformed settings report')
    fields = {'kind':'settings-query','case':case,'query':msg['query'],'status':msg['status']}
    if 'value' in msg: fields['value'] = setting_value(msg['query'],msg['value'])
    log(**fields)


def check_settings(settings):
    # Keep the existing required checks, with explicit optional mmap reporting.
    require(settings['sqlite_version()']=='3.53.4' and settings['foreign_keys']==1 and
            settings['journal_mode']=='wal' and settings['synchronous']==2 and
            settings['busy_timeout']==1000 and settings['max_open_connections']==1,
            'candidate settings changed')
    require(settings['mmap_size'] == 0 or settings['mmap_size'] ==
            {'supported':False,'reason':'vfs-control-notfound-no-row'}, 'unexpected mmap setting')


def check_mmap_coverage(messages, settings):
    reports = [m for m in messages if m['kind']=='vfs-coverage']
    require(len(reports)==1, 'missing/repeated no-mmap coverage report')
    report = reports[0]
    require(report.get('fetch_policy')=='always-null-no-native-delegation' and
            type(report.get('reads')) is int and report['reads']>0 and
            type(report.get('fetches')) is int and report['fetches']>=0 and
            report.get('null_fetches')==report['fetches'] and
            type(report.get('mmap_control_rejections')) is int and report['mmap_control_rejections']>=0,
            'no-mmap interception evidence missing')
    if isinstance(settings['mmap_size'],dict):
        require(report['mmap_control_rejections']>0,'unsupported mmap diagnostic without intercepted control')
    log(kind='no-mmap-coverage', **{k:report[k] for k in ('fetch_policy','reads','fetches','null_fetches','mmap_control_rejections')})

def owned(parent, name):
    path = parent / name
    path.mkdir(mode=0o700)
    (path / MARKER).write_text('Contextarium R3 synthetic resources only\n')
    return path

def remove_owned(root, path):
    require(path.parent.resolve() == root.resolve() and path != root and (path / MARKER).is_file(), 'cleanup ownership/containment')
    shutil.rmtree(path)

def budget(root):
    require(time.monotonic() < DEADLINE, 'R3 time budget exhausted; coverage incomplete')
    require(shutil.disk_usage(root).free >= 2 * 1024**3, 'free-space reserve reached')
    total = sum(p.stat().st_size for p in root.rglob('*') if p.is_file())
    require(total < 4 * 1024**3, 'scratch bound reached')

def run(*args, timeout=30):
    # Fixture/inspection/retry failures need the same pre-cleanup evidence as
    # streaming workers. These small helpers have no barrier protocol.
    env = {k:v for k,v in os.environ.items() if k in ('PATH','LANG','TZ','TMPDIR')}
    child = subprocess.Popen([str(a) for a in args], stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env)
    output = b''; stderr = b''; primary = None
    try:
        output, stderr = child.communicate(timeout=timeout)
        require(child.returncode == 0, 'helper worker failed')
        return output
    except BaseException as exc:
        primary = exc
        if isinstance(exc, subprocess.TimeoutExpired):
            output, stderr = exc.output or b'', exc.stderr or b''
        try:
            log_worker_failure('helper-timeout' if isinstance(exc, subprocess.TimeoutExpired) else 'helper-exit',
                               exc, child.poll(), False,
                               {'received_bytes':len(output),'sha256':digest(output),'complete':False},
                               stderr_summary(stderr, len(stderr)))
        except BaseException: pass # Evidence failure cannot replace the original failure.
        raise
    finally:
        cleanup = cleanup_child(child)
        if cleanup['controller_termination_performed'] or cleanup['cleanup_errors']:
            try: log(kind='worker-cleanup', **IDENTITY, **cleanup)
            except BaseException: pass
        if cleanup['cleanup_errors'] and primary is None:
            raise AssertionError('helper cleanup failed; see bounded diagnostic')

def read_trace(path):
    pending = {}
    events = []
    for line in path.read_text().splitlines():
        row = json.loads(line)
        if row['stage'] == 'pre':
            require(row['seq'] == len(pending) + 1, 'trace sequence gap')
            pending[row['seq']] = row
        else:
            require(row['seq'] in pending and 'rc' not in pending[row['seq']], 'unexpected post trace')
            pending[row['seq']].update(row)
            events.append(pending[row['seq']])
    return events, pending

class Model:
    """Write-back cache model reconstructed independently from successful VFS calls.

    Namespace create is durable with file sync; syncDir deletion is durable at
    return. Unsynced writes may persist early. Successful sync persists exactly
    this file's visible contents/length; it never flushes another file. Namespace
    and truncation operations order write epochs. Torn writes change only bytes
    addressed by that write (no collateral-sector corruption is claimed).
    """
    def __init__(self, initial):
        self.live = dict(initial)
        self.durable = dict(initial)
        self.pending = []

    @staticmethod
    def apply(image, op):
        name, kind = op['name'], op['op']
        if kind == 'open':
            image.setdefault(name, b'')
        elif kind == 'delete':
            image.pop(name, None)
        elif kind == 'truncate':
            current = image.get(name, b'')
            size = op['offset']
            image[name] = current[:size] + b'\0' * max(0, size-len(current))
        elif kind == 'write':
            data = bytes.fromhex(op['hex'])[:op['applied']]
            current = image.get(name, b'')
            off = op['offset']
            current += b'\0' * max(0, off+len(data)-len(current))
            image[name] = current[:off]+data+current[off+len(data):]
        else:
            raise AssertionError('unknown modeled mutation')

    def consume(self, event):
        name, kind, rc = event['name'], event['op'], event['rc']
        require(rc in (0, 10, 13, 1802, 5898), 'unexpected native SQLite I/O failure')
        if kind == 'sync':
            if rc == 0:
                self.durable[name] = self.live[name]
                self.pending = [op for op in self.pending if op['name'] != name]
            return
        if not event['applied']:
            return
        self.apply(self.live, event)
        self.pending.append(event)
        if kind == 'delete' and event['flags']:
            self.durable.pop(name, None)
            self.pending = [op for op in self.pending if op['name'] != name]

    def crash(self, schedule, seed):
        image = dict(self.durable)
        if schedule == 'discard':
            return image
        if schedule == 'retain':
            for event in self.pending:
                self.apply(image, event)
            return image
        require(schedule == 'reorder-torn', 'unknown persistence schedule')
        rng = random.Random(seed)
        batch = []
        def flush():
            rng.shuffle(batch)
            for event in batch:
                choice = rng.randrange(3)
                if choice == 0:
                    continue
                op = dict(event)
                if choice == 1 and op['applied'] > 1:
                    # Byte-prefix tear of a later write, not loss of an earlier sync.
                    op['applied'] = rng.randrange(1, op['applied'])
                self.apply(image, op)
            batch.clear()
        for event in self.pending:
            if event['op'] == 'write':
                batch.append(event)
            else:
                flush()
                # Preserve namespace/length ordering; truncation cuts are separately targeted.
                self.apply(image, event)
        flush()
        return image

def image_files(path):
    return {p.name: p.read_bytes() for p in path.iterdir() if p.name in ('store.db','store.db-wal','store.db-journal','probe.db')}

def materialize(root, name, image):
    path = owned(root, name)
    for filename, data in image.items():
        require(filename in ('store.db','store.db-wal','store.db-journal','probe.db'), 'unexpected image path')
        (path / filename).write_bytes(data)
    # database/sql requires an empty regular main file for a pre-initialization image.
    if 'store.db' not in image:
        (path / 'store.db').touch(mode=0o600)
    return path

def write_input(path, request):
    meta = {k:v for k,v in request.items() if k != 'Body'}
    path.write_text(json.dumps(meta)[:-1] + ',"Body":' + request.get('Body','{}') + '}')

def inspect(worker, path):
    return json.loads(run(worker, '-root', path, '-mode', 'inspect'))

# Discovery keeps the approved first/middle/last samples. Sequence numbers only
# identify trace rows. Full descriptors remain evidence; only the explicitly
# projected selector and Nth member of that exact selector group identify a target.
TARGET_FIELDS = ('phase','name','role','op','meaning','offset','length','flags','wal_page_size')
TARGET_POLICY = 'stable-selector-group-nth-v3'
FAULT_MODES = ('cut-before','cut-after','ioerr','full','partial')
REQUIRED_OPERATIONS = ('create','patch','metadata','archive','unarchive','noop','restore',
                       'checkpoint','autocheckpoint','migration','fresh','empty-m1')
MATRIX_ORDER = ('checkpoint','autocheckpoint','restore','migration','fresh','empty-m1',
                'noop','create','patch','metadata','archive','unarchive')
RECOVERY_SCHEDULES = (('discard',17),('retain',17),('reorder-torn',17),
                      ('reorder-torn',29),('reorder-torn',101))


def wal_header_page_size(data):
    # Pinned SQLite WAL header: big-endian fields, checksum order from magic.
    if len(data)<32:return 0
    words=[int.from_bytes(data[i:i+4],'big') for i in range(0,32,4)]
    magic,version,size=words[:3]
    if magic not in (0x377f0682,0x377f0683) or version!=3007000 or not valid_page_size(size):return 0
    order='big' if magic&1 else 'little';a=b=0
    for i in range(0,24,8):
        a=(a+int.from_bytes(data[i:i+4],order)+b)&0xffffffff
        b=(b+int.from_bytes(data[i+4:i+8],order)+a)&0xffffffff
    return size if [a,b]==words[6:8] else 0


def valid_page_size(size):
    return type(size) is int and 512<=size<=65536 and size&(size-1)==0


def wal_write_classification(data, offset, header):
    size=wal_header_page_size(header)
    require(not header or size,'invalid private WAL geometry header')
    n=len(data)
    if offset<0 or n<=0:return 'wal-raw',size
    if offset<32:
        pending=wal_header_page_size(data) if offset==0 else 0
        if offset==0 and n==32 and pending:return 'wal-header-reset',pending
        return ('wal-header-fragment' if offset+n<=32 and (offset!=0 or n<32) else 'wal-raw'),size
    if not size:return 'wal-unknown',0
    within=(offset-32)%(size+24)
    if within>=24:return ('wal-page-data' if n<=size+24-within else 'wal-raw'),size
    if within!=0 or n!=24:return ('wal-frame-fragment' if n<=24-within else 'wal-raw'),size
    if int.from_bytes(data[:4],'big')==0 or data[8:16]!=header[16:24]:return 'wal-raw',size
    return ('wal-commit-marker' if int.from_bytes(data[4:8],'big') else 'wal-frame'),size


def event_descriptor(event):
    op=event['op'];meaning=op;size=0
    if event['role']=='wal':
        # Geometry bytes stay in the private trace. Revalidate the header here,
        # independently of the C semantic label and public page-size integer.
        header=bytes.fromhex(event['wal_header'])
        require(len(header) in (0,32),'invalid private WAL header length')
        size=wal_header_page_size(header)
        require(not header or size,'invalid private WAL geometry header')
        if op=='write':
            data=bytes.fromhex(event['hex'])
            require(len(data)==event['length'],'target trace write length')
            meaning,size=wal_write_classification(data,event['offset'],header)
    return candidate_descriptor(dict(event,meaning=meaning,wal_page_size=size))


def candidate_descriptor(event):
    # Fixed labels and integers only; never retain an arbitrary worker string.
    require(event['phase'] in LABELS and event['name'] in
            ('store.db','store.db-wal','store.db-journal','probe.db'),'invalid target labels')
    require(event['role'] in ('database','wal','journal') and
            event['op'] in ('write','sync','truncate'),'invalid target operation')
    role='wal' if event['name'].endswith('-wal') else ('journal' if event['name'].endswith('-journal') else 'database')
    require(event['role']==role,'target role/name mismatch')
    meanings=('wal-header-reset','wal-frame','wal-commit-marker','wal-page-data',
              'wal-frame-fragment','wal-header-fragment','wal-unknown','wal-raw') if event['op']=='write' and role=='wal' else (event['op'],)
    require(event['meaning'] in meanings,'invalid target semantic')
    for name in ('offset','length','flags','wal_page_size'):
        require(type(event[name]) is int and 0<=event[name]<2**63,'invalid target range')
    size=event['wal_page_size'];offset=event['offset'];n=event['length'];meaning=event['meaning']
    require((size==0 or valid_page_size(size)) and (role=='wal' or size==0),'invalid WAL page size context')
    if event['op']=='write':
        require(n>0,'empty target write')
        if role=='wal':
            if meaning=='wal-header-reset':require(offset==0 and n==32 and size>0,'invalid WAL reset geometry')
            elif meaning=='wal-header-fragment':require(offset<32 and offset+n<=32 and (offset!=0 or n<32),'invalid WAL header fragment')
            elif meaning=='wal-unknown':require(size==0 and offset>=32,'invalid unknown WAL geometry')
            elif meaning!='wal-raw':
                require(size>0 and offset>=32,'missing WAL frame geometry')
                within=(offset-32)%(size+24)
                if meaning in ('wal-frame','wal-commit-marker'):
                    require(within==0 and n==24,'invalid WAL frame-header geometry')
                elif meaning=='wal-frame-fragment':require(within<24 and n<=24-within and (within!=0 or n!=24),'invalid WAL frame fragment')
                else:require(within>=24 and n<=size+24-within,'invalid WAL page-data geometry')
    return {k:event[k] for k in TARGET_FIELDS}


def target_selector(descriptor):
    """Only positive WAL append offsets are geometry, never every offset.

    Exact length/flags, validated page size and phase/file/role/op/meaning remain identity.
    Unknown/mixed/partial-header writes stay strict raw targets. Header-reset
    offset zero, truncate size, sync offset/flags and database or
    journal write offsets stay exact. The latter have no reviewed semantic page
    identity with which to safely replace their physical address.
    """
    descriptor=candidate_descriptor(descriptor)
    selector=dict(descriptor)
    append=(descriptor['role']=='wal' and descriptor['op']=='write' and
            descriptor['meaning'] in ('wal-frame','wal-commit-marker','wal-page-data'))
    selector['offset_policy']='positive-wal-append' if append else 'exact'
    if append:del selector['offset']  # Its positive range was checked above.
    return selector


def target_group(descriptor):
    """Canonical selector items are the sole discovery AND live group key.

    Positive WAL append offset is excluded only by target_selector; all other
    selector fields, including exact length/flags and strict offsets, remain.
    """
    return tuple(sorted(target_selector(descriptor).items()))


def make_target(items, index, samples):
    require(items and type(index) is int and 0<=index<len(items),'invalid target position')
    descriptor=event_descriptor(items[index]);group=target_group(descriptor)
    require(all(target_group(event_descriptor(e))==group for e in items),'mixed target selector group')
    return {'descriptor':descriptor, 'selector':target_selector(descriptor), 'samples':samples,
            'group_position':index+1,'group_count':len(items),'baseline_seq':items[index]['seq']}


def target_summary(plan):
    return {k:plan[k] for k in ('descriptor','selector','samples','group_position','group_count','baseline_seq')} | {
        'identity_policy':TARGET_POLICY,'wal_classifier':'wal-format-boundaries-v1'}


def target_evidence(plan, event):
    descriptor=event_descriptor(event)
    require(target_selector(descriptor)==plan['selector'],'fault schedule selector drift')
    return {'planned':target_summary(plan),'observed':dict(descriptor,seq=event['seq']),
            'geometry_changed':['offset'] if descriptor['offset']!=plan['descriptor']['offset'] else []}


class TargetMatcher:
    """Select the Nth live member of ONE complete stable-selector class.

    Other selector shapes never advance this group or authorize injection.
    Members within this group are intentionally indistinguishable: adding or
    removing one changes positions and cannot reveal a hidden page/transaction
    identity. Missing N fails. Discovery count/first-middle-last labels and full
    baseline geometry are evidence, not requirements on the live trace suffix.
    No prefix comparison, search ahead, retry or physical-address substitution.
    """
    def __init__(self, plan):
        self.plan=plan;self.group=target_group(plan['descriptor']);self.seen=0
        self.last_seq=0;self.last_observed=[];self.selected_seq=None
        self.selected_descriptor=None;self.geometry_change=None
        self.alternates={};self.alternate_events=0;self.unlisted_alternate_events=0
        self.trace_verified=False;self.decision_sent=False;self.reached=False;self.ack_before_fault=0
        require(target_selector(plan['descriptor'])==plan['selector'] and
                type(plan['group_position']) is int and type(plan['group_count']) is int and
                1<=plan['group_position']<=plan['group_count'] and
                type(plan['baseline_seq']) is int and plan['baseline_seq']>0 and
                plan['samples'] and all(s in ('first','middle','last') for s in plan['samples']),
                'invalid target group plan')

    def observe(self, event):
        require(self.selected_seq is None,'repeated target decision')
        descriptor=candidate_descriptor(event)
        self.geometry_change=None
        require(type(event['seq']) is int and event['seq']>self.last_seq,'decision sequence not increasing')
        self.last_seq=event['seq']
        observed=dict(descriptor,seq=event['seq'])
        self.last_observed=(self.last_observed+[observed])[-8:]
        group=target_group(descriptor)
        if group!=self.group:
            # Relevant alternatives share phase OR file. Bound shape storage;
            # overflow is explicit, never silently presented as a full census.
            if (descriptor['phase']==self.plan['descriptor']['phase'] or
                    descriptor['name']==self.plan['descriptor']['name']):
                self.alternate_events+=1
                if group in self.alternates:
                    self.alternates[group]['count']+=1
                    self.alternates[group]['last_observed']=observed
                elif len(self.alternates)<8:
                    self.alternates[group]={'selector':target_selector(descriptor),'count':1,
                                            'last_observed':observed}
                else:self.unlisted_alternate_events+=1
            return False
        self.seen+=1
        if self.seen!=self.plan['group_position']:return False
        baseline=dict(self.plan['descriptor'],seq=self.plan['baseline_seq'])
        if descriptor['offset']!=baseline['offset']:
            self.geometry_change={'group_position':self.seen,'baseline':baseline,'observed':observed}
        self.selected_seq=event['seq']
        self.selected_descriptor=descriptor
        return True

    def require_reached(self, hit):
        self.reached=(self.trace_verified and self.decision_sent and hit is not None and
                      hit['seq']==self.selected_seq)
        require(self.reached,'required descriptor target not reached')

    def diagnostics(self):
        return {'planned':target_summary(self.plan),'matching_live_group_members':self.seen,
                'relevant_alternate_scope':'same-phase-or-file','relevant_alternate_events':self.alternate_events,
                'alternate_selectors':list(self.alternates.values()),
                'unlisted_alternate_events':self.unlisted_alternate_events,
                'last_observed':self.last_observed,'selected_seq':self.selected_seq,
                'selected_observed':dict(self.selected_descriptor,seq=self.selected_seq) if self.selected_descriptor else None,
                'last_geometry_change':self.geometry_change,
                'trace_verified_before_decision':self.trace_verified,
                'decision_sent':self.decision_sent,'target_reached':self.reached,
                'acknowledged_before_fault':self.ack_before_fault}


def verify_pending_trace(trace, event):
    # The VFS flushed its private pre-row before blocking. Read only a bounded
    # tail; a row larger than the bound fails rather than authorizing a fault.
    with trace.open('rb') as source:
        source.seek(0,os.SEEK_END);size=source.tell();source.seek(max(0,size-256*1024))
        tail=source.read(256*1024)
    require(tail.endswith(b'\n'),'pending trace row incomplete')
    pending=json.loads(tail.splitlines()[-1])
    require(pending['stage']=='pre' and pending['seq']==event['seq'],'pending trace identity mismatch')
    require(event_descriptor(pending)==candidate_descriptor(event),'pending trace descriptor mismatch')


def decide_io(stream, trace, matcher, event, acknowledgements):
    inject=matcher.observe(event)  # Only this exact selector group and Nth position may inject.
    if matcher.geometry_change is not None:
        # Safe metadata for the selected member, before any reply. This
        # is a selector match, not proof of private-trace verification/injection.
        log(kind='target-geometry',identity_policy=TARGET_POLICY,
            target_baseline_seq=matcher.plan['baseline_seq'],
            stage='selector-match-before-trace-verification',**matcher.geometry_change)
    if inject:
        verify_pending_trace(trace,event)
        matcher.trace_verified=True
        # Only complete successful mutation responses already fsynced in the
        # external ledger count. A WAL marker is never an acknowledgement.
        matcher.ack_before_fault=sum('result_text' in a for a in acknowledgements)
    stream.write((('i' if inject else 'c')+' '+str(event['seq'])+'\n').encode())
    stream.flush()
    if inject:matcher.decision_sent=True


def record_acknowledgement(ledger, acknowledgements, case, request, message):
    require(message['kind']=='response' and message['ok'] is True,'invalid acknowledgement')
    if 'result_text' in message:
        require(isinstance(message['result_text'],str) and message['result_text'], 'incomplete mutation response')
        json.loads(message['result_text'])
    ack_request={'Operation':'create','Key':message['key'],'Body':message['body_text']} if 'body_text' in message else request
    entry={'case':case,'request_sha256':digest(encoded(ack_request)),'response':message,'response_sha256':digest(encoded(message))}
    ledger.write(encoded(entry)+b'\n');ledger.flush();os.fsync(ledger.fileno())
    acknowledgements.append(message)  # Never visible to the target decision first.


def fault_modes(target):
    op=target['descriptor']['op']
    return ['cut-before','cut-after','ioerr']+(['full'] if op in ('write','truncate') else [])+(['partial'] if op=='write' else [])


class AcceptanceCoverage:
    """Final PASS requires every planned family/sector/target/mode and post-ack
    checkpoint boundaries, not merely a positive count from an early family."""
    def __init__(self):
        self.planned={};self.completed=set();self.post_ack=set()

    def plan(self, operation, sector, targets):
        key=(operation,sector)
        require(key not in self.planned and targets,'missing/repeated family plan')
        self.planned[key]={(p['baseline_seq'],mode) for _,p in targets for mode in fault_modes(p)}

    def complete(self, operation, sector, plan, fault, results, ack_before_fault):
        key=(operation,sector);item=(plan['baseline_seq'],fault)
        require(key in self.planned and item in self.planned[key],'unplanned completed case')
        identity=(*key,*item)
        require(identity not in self.completed,'duplicate completed case')
        require([(r['schedule'],r['seed']) for r in results]==list(RECOVERY_SCHEDULES), 'incomplete recovery schedules')
        require(all(r['outcome'] in ('present','absent') for r in results),'invalid recovered outcome')
        self.completed.add(identity)
        if ack_before_fault>0:
            d=plan['descriptor'];self.post_ack.add((operation,sector,d['role'],d['meaning']))

    def require_complete(self):
        families={(op,sector) for op in REQUIRED_OPERATIONS for sector in (512,4096)}
        require(set(self.planned)==families,'required operation/sector family absent')
        expected={(*key,*item) for key,items in self.planned.items() for item in items}
        require(self.completed==expected,'required target/fault cases incomplete')
        required_ack=set()
        for sector in (512,4096):
            for op in ('checkpoint','autocheckpoint'):
                required_ack.update((op,sector,'database',meaning) for meaning in ('write','sync'))
            required_ack.add(('checkpoint',sector,'wal','truncate'))
            required_ack.add(('autocheckpoint',sector,'wal','wal-header-reset'))
        require(required_ack<=self.post_ack,'new acknowledged-effect recovery coverage missing')


def execute(worker, root, case, request, initial, target=None, fault='none', sector=4096, probe=False, native=False, diagnostic=False):
    if diagnostic:
        require(target is None and fault == 'none' and not probe and request['Operation'] == 'create',
                'diagnostic entry forbids targets, faults, probes and non-create requests')
    require((target is None and fault=='none') or (isinstance(target,dict) and fault in FAULT_MODES), 'invalid target/fault pair')
    if target is not None:require(fault in fault_modes(target),'fault mode incompatible with target operation')
    matcher=TargetMatcher(target) if target is not None else None
    budget(root)
    live = owned(root, 'live') if probe else materialize(root, 'live', initial)
    config = root / 'input.json'
    write_input(config, request)
    trace = root / 'trace.jsonl'
    if trace.exists(): trace.unlink() # Controller-owned file in the marked run root.
    command = [str(worker), '-root', str(live), '-mode', 'probe' if probe else ('native' if native else 'run'), '-trace', str(trace), '-fault', fault, '-sector', str(sector)]
    if not probe: command += ['-input',str(config)]
    if diagnostic: command += ['-diagnostic-only']
    ack = []; messages = []; hit = None; progress = {}
    # No credentials or Actions token are inherited by the instrumented process.
    env = {k:v for k,v in os.environ.items() if k in ('PATH','LANG','TZ','TMPDIR')}
    stderr_path = root / 'worker-stderr.txt'
    with evidence_file(stderr_path) as stderr, evidence_file(root / 'acks.jsonl') as ledger:
        child = subprocess.Popen(command, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=stderr, env=env)
        selector = selectors.DefaultSelector();selector.register(child.stdout, selectors.EVENT_READ)
        buffer = b''; finished = False; done_received = False; deadline = time.monotonic()+45
        received = 0; lines = 0; candidates = 0; bad_line = None; terminated = False
        stage = 'protocol-wait'; primary = None
        try:
            while not finished:
                stage = 'protocol-timeout'
                require(time.monotonic() < deadline, 'child response/barrier timeout')
                if not selector.select(1): continue
                stage = 'protocol-read'
                part = os.read(child.stdout.fileno(),65536)
                if not part:
                    # EOF can precede waitpid visibility. Briefly collect natural
                    # status without signalling, then report before any cleanup.
                    try: child.wait(timeout=.25)
                    except subprocess.TimeoutExpired: pass
                    stage = 'protocol-eof'
                    require(False, 'child exited without result/barrier')
                received += len(part); buffer += part
                stage = 'protocol-size'
                require(len(buffer) <= 8*1024**2 and received <= 32*1024**2, 'protocol bound exceeded')
                while b'\n' in buffer:
                    line, buffer = buffer.split(b'\n',1)
                    stage = 'protocol-json'; bad_line = {'bytes':len(line),'sha256':digest(line)}
                    msg = json.loads(line)
                    require(isinstance(msg,dict) and msg.get('kind') in
                            {'worker-progress','worker-error','worker-fatal','setting','settings','io-candidate','target','response','state','vfs-coverage','done'},
                            'malformed protocol envelope')
                    bad_line = None
                    if msg['kind']=='io-candidate':
                        stage='pre-injection-descriptor-check'
                        candidates+=1
                        require(candidates<=32768,'I/O decision bound exceeded')
                        require(matcher is not None,'unexpected I/O decision request')
                        decide_io(child.stdin,trace,matcher,msg,ack)
                        continue
                    lines += 1; messages.append(msg)
                    require(lines <= 2048, 'protocol record bound exceeded')
                    safe = safe_diagnostic(msg)
                    if safe: track_progress(progress, safe)
                    if msg['kind'] == 'setting':
                        stage = 'settings-report'
                        log_setting(msg, case)
                    if msg['kind'] == 'target':
                        stage = 'target-barrier'
                        require(matcher is not None and matcher.trace_verified and matcher.decision_sent and
                                hit is None and msg['seq'] == matcher.selected_seq and msg['mode'] == fault, 'wrong/repeated target')
                        hit=msg
                        if fault.startswith('cut-'): finished=True;break
                    if msg['kind'] == 'response' and msg['ok']:
                        stage = 'acknowledgement-ledger'
                        record_acknowledgement(ledger,ack,case,request,msg)
                    if msg['kind'] == 'done': finished=True;done_received=True;break
            if diagnostic:
                stage = 'diagnostic-natural-exit'
                child.wait(timeout=5)
                require(child.returncode == 0, 'no-fault diagnostic did not exit cleanly')
                # No unread protocol bytes can hide an unexpected extra response.
                buffer += child.stdout.read(8*1024**2+1)
                require(not buffer, 'extra diagnostic protocol output')
            else:
                stage = 'planned-barrier-termination'
                child.kill();terminated=True;child.wait(timeout=5)
                require(child.returncode == -signal.SIGKILL, 'test child did not die without cleanup')
            stage = 'fault-target-verification'
            if matcher:matcher.require_reached(hit)
            else:require(hit is None,'unexpected fault target')
            if native:
                image=image_files(live)
                result={'image':image,'ack':ack,'messages':messages}
            else:
                stage = 'trace-validation'
                events, all_events=read_trace(trace)
                if hit:
                    require(hit['seq'] in all_events, 'target missing from trace')
                    chosen=all_events[hit['seq']]
                    require(event_descriptor(chosen)==matcher.selected_descriptor,'verified target trace changed')
                    require(chosen['op'] in ('write','sync','truncate'), 'fault outside approved operation')
                model=Model(initial)
                for event in events: model.consume(event)
                # Also retained in diagnostic-only mode; no crash() schedules run.
                require(image_files(live) == model.live, 'live file/model mismatch: untraced I/O or broken model')
                result={'events':events,'all':all_events,'model':model,'ack':ack,'messages':messages,'trace_sha256':digest(trace.read_bytes()),'ack_sha256':digest((root/'acks.jsonl').read_bytes()),
                        'target_seq':hit['seq'] if hit else None,'ack_before_fault':matcher.ack_before_fault if matcher else 0}
            stage = 'scratch-cleanup'
            remove_owned(root,live)
            return result
        except BaseException as exc:
            primary = exc
            # Persist safe evidence to the job log before killing/closing anything.
            try:
                with stderr_path.open('rb') as source:
                    total = source.seek(0, os.SEEK_END); source.seek(max(0,total-65536))
                    summary = stderr_summary(source.read(65536), total)
                log_worker_failure(stage, exc, child.poll(), terminated,
                                   {'received_bytes':received,'complete_records':lines,'done_received':done_received,
                                    'partial_bytes':len(buffer),'partial_sha256':digest(buffer),'malformed_record':bad_line},
                                   summary, progress, case=case,
                                   acknowledged=len(ack), target_reached=hit is not None,
                                   targeting=matcher.diagnostics() if matcher else None,decision_records=candidates)
            except BaseException as diagnostic_error:
                # Do not replace the primary failure even if evidence collection fails.
                try: log(kind='diagnostic-collection-failed', **IDENTITY, failure_stage=stage,
                         error_type=type(diagnostic_error).__name__, primary_error_type=type(exc).__name__)
                except BaseException: pass
            raise
        finally:
            cleanup = cleanup_child(child, selector)
            if primary is not None or cleanup['cleanup_errors']:
                try: log(kind='worker-cleanup', **IDENTITY, case=case, **cleanup)
                except BaseException: pass
            if cleanup['cleanup_errors'] and primary is None:
                raise AssertionError('worker cleanup failed; see bounded diagnostic')

def record_snapshot(v):
    return {'id':v['record_id'], **{k:v[k] for k in SNAPSHOT}}

def record_json(v, number):
    obj=dict(v)
    for field in ('data','provenance'): obj[field]=precise(obj[field])
    obj['schema_version']=('number',str(obj['schema_version']))
    obj['revision']=('number',str(number))
    return obj

def consistency(state):
    versions=[x['version'] for x in state['schema_migrations']]
    require(versions in ([],[1,2],[1,2,3]),'partial migration ledger')
    if versions != [1,2,3]:
        require(not state['record_revisions'] and not state['mutation_audit'],'M2 rows without complete ledger')
        return
    revisions={(v['record_id'],v['revision_number']):v for v in state['record_revisions']}
    audits={a['event_id']:a for a in state['mutation_audit']}
    require(len(revisions)==len(state['record_revisions'])==len(audits)==len(state['mutation_audit']),'duplicate/missing event/revision')
    records={r['id']:r for r in state['records']}
    schemas={(s['schema_id'],s['schema_version']) for s in state['schemas']}
    subjects={s['id'] for s in state['subjects']}
    for r in records.values():
        chain=sorted(v['revision_number'] for v in revisions.values() if v['record_id']==r['id'])
        require(chain==list(range(1,r['revision']+1)),'noncontiguous history')
        require(record_snapshot(revisions[(r['id'],r['revision'])])=={k:v for k,v in r.items() if k!='revision'},'head/snapshot mismatch')
    for key,v in revisions.items():
        require(v['record_id'] in records and v['subject_id'] in subjects and (v['schema_id'],v['schema_version']) in schemas,'revision identity/schema')
        a=audits.get(v['audit_event_id']);require(a is not None,'missing audit')
        require(a['resource_id']==v['record_id'] and a['revision_number']==v['revision_number'],'event/revision pair')
        for left,right in [('timestamp','recorded_at'),('actor_id','actor_id'),('attribution_kind','attribution_kind'),('request_id','request_id'),('subject_id','subject_id'),('namespace','namespace'),('base_revision','base_revision'),('source_revision','source_revision')]:
            require(a[left]==v[right],'audit linkage '+left)
        require(a['resource_type']=='record' and a['result']=='accepted' and a['proposal_id'] is None,'audit envelope')
        n=v['revision_number']
        if n==1:
            require(v['base_revision'] is None and v['operation'] in ('create','m1_adoption'),'first revision')
        else:
            require(v['base_revision']==n-1 and v['operation'] in ('update','restore'),'base linkage')
            old=revisions[(v['record_id'],n-1)]
            for field in ('subject_id','namespace','schema_id','schema_version','created_at'):
                require(v[field]==old[field],'immutable record identity')
        action={'create':'record.created','m1_adoption':'record.history_adopted','restore':'revision.restored'}.get(v['operation'],'record.updated')
        if v['operation']=='update':
            old=revisions[(v['record_id'],n-1)]
            if old['status']!=v['status']: action='record.archived' if v['status']=='archived' else 'record.unarchived'
        require(a['action']==action,'audit action')
        require(a['reason']==('m1_history_boundary' if v['operation']=='m1_adoption' else None),'audit reason')
        if v['operation']=='restore':
            require(1<=v['source_revision']<n,'restore source')
            prior=revisions[(v['record_id'],v['source_revision'])]
            require(all(v[k]==prior[k] for k in SNAPSHOT if k!='updated_at'),'restore content')
        else: require(v['source_revision'] is None,'unexpected restore source')
        if v['operation']!='m1_adoption':require(v['recorded_at']==v['updated_at'],'action time')
    for k in state['idempotency_keys']:
        if k['response_contract']!='m2' or 'records' not in k['scope']:continue
        r=precise(k['response']);rid=r['id'];n=int(r['revision'][1])
        require((rid,n) in revisions,'idempotency revision absent')
        require(r==record_json(record_snapshot(revisions[(rid,n)]),n),'replay/snapshot mismatch or number token changed')

def subset(old,new,label):
    require(all(row in new for row in old), 'previously durable '+label+' changed/lost')

def oracle(before, after, request, acks, m2schema):
    consistency(after)
    op=request['Operation']
    if op in ('migration','fresh','empty-m1'):
        if after==before:
            require(not acks,'acknowledged adoption lost');return 'absent'
        require([r['version'] for r in after['schema_migrations']]==[1,2,3],'partial upgrade')
        require(after['sqlite_schema']==m2schema,'partial/changed migration DDL')
        for table in ('subjects','schemas'):require(before[table]==after[table],'adoption changed '+table)
        subset(before['schema_migrations'],after['schema_migrations'],'ledger')
        require(after['records']==[dict(r,revision=1) for r in before['records']],'adoption record bytes')
        require(after['idempotency_keys']==[dict(k,response_contract='m1') for k in before['idempotency_keys']],'legacy replay bytes')
        require(len(after['record_revisions'])==len(before['records']),'invented history')
        for v in after['record_revisions']:
            require(v['operation']=='m1_adoption' and v['actor_id']=='local-migration-003','adoption attribution')
        return 'present'
    for table in ('subjects','schemas','schema_migrations','sqlite_schema'):require(before[table]==after[table],'unrelated '+table+' changed')
    for table in ('record_revisions','mutation_audit','idempotency_keys'):subset(before[table],after[table],table)
    newkeys=[k for k in after['idempotency_keys'] if k not in before['idempotency_keys']]
    expected=[request]
    if op in ('checkpoint','autocheckpoint'):
        expected=[]
        for i in range(1 if op=='checkpoint' else 40):
            expected.append({'Operation':'create','Key':f'checkpoint-{i:03d}','Body':checkpoint_body(before)})
    bykey={r['Key']:r for r in expected}
    require(len(newkeys)<=len(expected),'extra replay keys')
    require(len(after['record_revisions'])==len(before['record_revisions'])+len(newkeys),'partial revision effect')
    require(len(after['mutation_audit'])==len(before['mutation_audit'])+len(newkeys),'partial audit effect')
    require(len(after['records'])==len(before['records'])+(len(newkeys) if op in ('create','checkpoint','autocheckpoint') else 0),'partial current effect')
    newids=set()
    for key in newkeys:
        require(key['key'] in bykey,'unexpected accepted request')
        req=bykey[key['key']];body=precise(req['Body']);saved=precise(key['response']);rid=saved['id'];newids.add(rid)
        scope='POST records' if req['Operation']=='create' else ('POST records/'+req['ID']+'/restore' if req['Operation']=='restore' else 'PATCH records/'+req['ID'])
        fingerprint={'revision':('number',str(req['Target'])),'body':body} if req['Operation']=='restore' else body
        require(key['scope']==scope and key['request_hash']==digest(canonical(fingerprint).encode()),'scope/fingerprint changed')
        r=next(r for r in after['records'] if r['id']==rid)
        v=next(v for v in after['record_revisions'] if v['record_id']==rid and v['revision_number']==r['revision'])
        require(v['actor_id']=='r3-worker' and v['attribution_kind']=='development_test' and v['request_id']=='req_00000000000000000000000000000300','new action attribution')
        require(key['response_contract']=='m2','new response contract')
        if req['Operation']=='create':
            require(r['revision']==1 and r['created_at']==r['updated_at'],'create head/time')
            require(r['subject_id']==body['subject_id'] and r['namespace']==body['namespace'] and r['schema_id']==body['schema_id'] and ('number',str(r['schema_version']))==body['schema_version'],'create schema/identity')
            require(precise(r['data'])==body['data'] and r['key'] is None and r['provenance']=='null' and r['sensitivity']=='private' and r['status']=='active','create content/tokens')
        else:
            old=next(r for r in before['records'] if r['id']==req['ID'])
            want=dict(old)
            if req['Operation']=='restore':
                oldv=next(v for v in before['record_revisions'] if v['record_id']==req['ID'] and v['revision_number']==req['Target'])
                want=record_snapshot(oldv)
            require(r['id']==old['id'] and r['revision']==old['revision']+1,'mutation head')
            for field in ('subject_id','namespace','schema_id','schema_version','created_at'):require(r[field]==old[field],'identity changed')
            for field in ('data','key','sensitivity','provenance','status'):
                actual=precise(r[field]) if field in ('data','provenance') else r[field]
                expected_value=precise(want[field]) if field in ('data','provenance') else want[field]
                if req['Operation']!='restore' and field in body:expected_value=body[field]
                require(actual==expected_value,'mutation field/token mismatch '+field)
    for old in before['records']:
        if old['id'] not in newids:require(old in after['records'],'unrelated current record changed')
    for ack in acks:
        if 'result_text' not in ack:continue
        require(any(precise(k['response'])==precise(ack['result_text']) for k in after['idempotency_keys']),'acknowledged mutation lost/changed')
    if not newkeys:require(after==before,'partial state without replay')
    return 'present' if newkeys else 'absent'

def checkpoint_body(before):
    sub=before['subjects'][0]['id']
    return '{"subject_id":'+json.dumps(sub)+',"namespace":"example.r3","schema_id":"example.r3","schema_version":1,"data":{"n":9007199254740993,"d":0.30,"e":1e2,"z":-0,"payload":'+json.dumps('c'*48000)+'}}'

def replay(worker, root, path, before, request, acks, m2schema):
    op=request['Operation']
    requests=[request]
    if op in ('checkpoint','autocheckpoint'):
        requests=[{'Operation':'create','Key':k['key'],'Body':checkpoint_body(before)} for k in inspect(worker,path)['idempotency_keys'] if k['key'].startswith('checkpoint-')]
        if not requests: return
    if op=='migration':requests=request.get('Legacy',[]) or [request]
    if op in ('fresh','empty-m1'):requests=[request]
    # Batch input embeds exact raw JSON request bodies without float conversion.
    config=root/'retry.json'
    parts=[]
    for req in requests:
        meta={k:v for k,v in req.items() if k not in ('Body','Legacy')}
        parts.append(json.dumps(meta)[:-1]+',"Body":'+req.get('Body','{}')+'}')
    config.write_text('{"Operation":"batch","Body":{},"Batch":['+','.join(parts)+']}')
    old=inspect(worker,path)
    responses=[json.loads(line) for line in run(worker,'-root',path,'-mode','retry','-input',config).splitlines()]
    require(len(responses)==len(requests) and all(r['ok'] for r in responses),'same-key retry failed')
    after=inspect(worker,path);consistency(after)
    if op in ('migration','fresh','empty-m1'):
        oracle(before,after,request,[],m2schema)
    elif old!=before or op in ('checkpoint','autocheckpoint'):
        require(after==old,'successful replay changed stores')
    else:
        oracle(before,after,request,responses,m2schema)
    for req,response in zip(requests,responses):
        original=next((k for k in old['idempotency_keys'] if k['key']==req['Key']),None) if 'Key' in req else None
        if original:require(precise(original['response'])==precise(response['result_text']),'replay changed original result')
    again=[json.loads(line) for line in run(worker,'-root',path,'-mode','retry','-input',config).splitlines()]
    require(responses==again and inspect(worker,path)==after,'second replay changed result/stores')

def expect_reject(name, reason, callback):
    try: callback()
    except AssertionError as exc:
        # A timeout, worker failure, parser bug, or different invariant failure
        # must never count as detection of the deliberately incorrect outcome.
        require(str(exc)==reason,'negative control rejected for an unexpected reason')
        log(kind='negative-control',name=name,result='PASS',rejection=reason);return
    raise AssertionError('negative control failed to reject: '+name)

def model_validation():
    def event(op,name='store.db',**kwargs):return dict(dict(op=op,name=name,rc=0,applied=1,flags=0,offset=0,hex=''),**kwargs)
    def check(model_class):
        m=model_class({})
        m.consume(event('open'))
        m.consume(event('write',hex='616263646566',applied=6))
        require(m.live['store.db']==b'abcdef' and 'store.db' not in m.crash('discard',17),'volatile visibility/create')
        m.consume(event('sync'))
        require(m.crash('discard',17).get('store.db')==b'abcdef','successful sync persistence')
        m.consume(event('write',offset=2,hex='58595a',applied=3))
        require(m.live['store.db']==b'abXYZf','overwrite visibility')
        m.consume(event('sync',rc=10,applied=0))
        require(m.crash('discard',17)['store.db']==b'abcdef','failed sync must not claim durability')
        m.consume(event('truncate',offset=4))
        require(m.live['store.db']==b'abXY','truncate size')
        require(m.crash('retain',17)['store.db']==b'abXY','crash reconstruction')
        m.consume(event('sync'))
        require(m.crash('discard',17)['store.db']==b'abXY','synced truncate')
        m.consume(event('open',name='probe.db'))
        m.consume(event('write',name='probe.db',hex='ff',applied=1))
        require('probe.db' not in m.crash('discard',17),'one-file sync flushed another file')
        m.consume(event('delete',flags=1))
        require('store.db' not in m.crash('discard',17),'syncDir delete')
        for seed in SEEDS:require(m.crash('reorder-torn',seed)==m.crash('reorder-torn',seed),'nondeterministic seed')
    check(Model)
    class BrokenSync(Model):
        def consume(self,e):
            if e['op']=='sync': return
            super().consume(e)
    expect_reject('incorrect-sync-semantics','successful sync persistence',lambda:check(BrokenSync))
    log(kind='validation',name='independent-storage-model',result='PASS')

def raw_validation(worker,root):
    result=execute(worker,root,'probe',{}, {}, probe=True)
    require(any(m.get('kind')=='response' and m['ok'] for m in result['messages']),'native VFS probe failed')
    require(result['model'].live=={} and result['model'].durable=={},'native namespace/delete model')
    firstsync=next(e for e in result['events'] if e['op']=='sync')
    partial=Model({})
    for event in result['events']:
        partial.consume(event)
        if event['seq']==firstsync['seq']:break
    require(partial.crash('discard',17)=={'probe.db':b'abcdef'},'native synced bytes reconstruction')
    failed=execute(worker,root,'probe-failed-sync',{}, {},target=make_target([firstsync],0,['first']),fault='ioerr',probe=True)
    require(any(m.get('kind')=='response' and not m['ok'] for m in failed['messages']),'failed sync was reported successful')
    require('probe.db' not in failed['model'].crash('discard',17),'native failed-sync behavior')
    require(any(e['op']=='truncate' and e['offset']==4 for e in result['events']),'truncate not intercepted')
    log(kind='validation',name='native-VFS-visibility-sync-truncate-namespace',result='PASS')

def m1_fixture(binary,root,empty=False):
    path=root/'store.db'; child=None; streams={}; legacy=[]
    stage='launch'; failure_stage=None; primary=None; secondary=[]; http_info={}
    fixture='empty' if empty else 'populated'
    def mark(phase):
        nonlocal stage
        stage=phase; http_info.clear()
        log(kind='m1-fixture-progress',fixture=fixture,stage=stage)
    def request_http(method,url,body=None,key=None):
        # The helper must not shadow the imported http package.
        conn=None; request_error=None; http_info.clear()
        try:
            conn=http.client.HTTPConnection('127.0.0.1',port,timeout=2)
            headers={'Content-Type':'application/json'}
            if key:headers['Idempotency-Key']=key
            conn.request(method,url,body.encode() if body else None,headers)
            response=conn.getresponse()
            http_info['status']=response.status if type(response.status) is int and 100<=response.status<=599 else None
            raw=response.read(128*1024+1)
            http_info.update(response_bytes=len(raw),response_sha256=digest(raw),response_truncated=len(raw)>128*1024)
            if len(raw)>128*1024:raise FixtureError('fixture-response-too-large')
            try: parsed=json.loads(raw) if raw else None
            except (ValueError,UnicodeError):
                if response.status not in (200,201):raise FixtureError('fixture-http-status') from None
                raise
            error=parsed.get('error') if isinstance(parsed,dict) else None
            code=error.get('code') if isinstance(error,dict) else None
            if code is not None:
                http_info['api_error_code']=code if isinstance(code,str) and code in M1_API_CODES else 'redacted'
            if response.status not in (200,201):raise FixtureError('fixture-http-status')
            return parsed
        except BaseException as exc:
            request_error=exc
            raise
        finally:
            if conn is not None:
                try:conn.close()
                except BaseException as exc:
                    secondary.append({'action':'http-close',**safe_failure(exc,stage,'fixture-http-close')})
                    if request_error is None:raise FixtureError('fixture-http-close') from exc
    try:
        mark('launch')
        # File-backed child output avoids a full pipe blocking readiness. These
        # exclusive files belong to the caller's marked fixture directory only.
        for name in ('stdout','stderr'):streams[name]=(root/('m1-'+name+'.txt')).open('x+b')
        with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
        env={k:v for k,v in os.environ.items() if k in ('PATH','LANG','TZ')}
        env.update(CONTEXTARIUM_LISTEN_ADDR=f'127.0.0.1:{port}',CONTEXTARIUM_DB_PATH=str(path))
        child=subprocess.Popen([str(binary)],env=env,stdout=streams['stdout'],stderr=streams['stderr'])
        mark('readiness')
        until=time.monotonic()+10
        while True:
            if child.poll() is not None:raise FixtureError('fixture-child-exited')
            try:request_http('GET','/readyz');break
            except (OSError,FixtureError) as exc:
                if secondary:raise  # A failed HTTP close is not a readiness retry.
                if isinstance(exc,FixtureError) and exc.code!='fixture-http-status':raise
                if child.poll() is not None:raise FixtureError('fixture-child-exited') from exc
                if time.monotonic()>=until:raise FixtureError('fixture-readiness-timeout') from exc
                time.sleep(.02)
        if not empty:
            mark('subject')
            sub=request_http('POST','/api/v1/subjects','{"kind":"project","display_name":"Synthetic R3 M1"}','subject')['data']['id']
            mark('schema')
            request_http('POST','/api/v1/schemas','{"schema_id":"example.r3","schema_version":1,"definition":{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"},"migration_policy":"explicit"}','schema')
            body='{"subject_id":'+json.dumps(sub)+',"namespace":"example.r3","schema_id":"example.r3","schema_version":1,"data":{"n":9007199254740993,"d":0.30,"e":1e2,"z":-0,"unknown":["雪",true]}}'
            mark('record')
            old=request_http('POST','/api/v1/records',body,'legacy-create')['data']
            request_http('POST','/api/v1/records',body,'legacy-other')
            mark('update')
            request_http('PATCH','/api/v1/records/'+old['id'],'{"status":"archived"}','legacy-update')
            legacy=[{'Operation':'create','Key':'legacy-create','Body':body},{'Operation':'patch','ID':old['id'],'Key':'legacy-update','Body':'{"status":"archived"}'}]
    except BaseException as exc:
        primary=exc; failure_stage=stage
        try:log(kind='m1-fixture-failure',fixture=fixture,**safe_failure(exc,stage),http=dict(http_info))
        except BaseException as diagnostic_error:
            secondary.append({'action':'failure-log',**safe_failure(diagnostic_error,stage,'fixture-log-failed')})
        raise
    finally:
        stage='close'; cleanup={}; outputs={}
        if child is not None:
            try:
                cleanup=close_m1_child(child)
                secondary.extend(cleanup.pop('cleanup_errors'))
                if child.returncode!=0:raise FixtureError('fixture-exit-nonzero')
            except BaseException as exc:
                code=exc.code if isinstance(exc,FixtureError) else 'fixture-cleanup-failed'
                secondary.append({'action':'process-close',**safe_failure(exc,'close',code)})
            try:
                if (root/'store.db-wal').exists():raise FixtureError('fixture-wal-remains')
            except BaseException as exc:
                code=exc.code if isinstance(exc,FixtureError) else 'fixture-cleanup-failed'
                secondary.append({'action':'wal-postcondition',**safe_failure(exc,'close',code)})
        for name,stream in streams.items():
            try:outputs[name]=m1_output_file(stream,child is not None and child.returncode is not None)
            except BaseException as exc:
                secondary.append({'action':'capture-'+name,**safe_failure(exc,'close','fixture-capture-failed')})
            try:stream.close()
            except BaseException as exc:
                secondary.append({'action':'close-'+name,**safe_failure(exc,'close','fixture-stream-close')})
        try:
            log(kind='m1-fixture-close',fixture=fixture,stage='close',primary_preserved=primary is not None,
                **cleanup,outputs=outputs,cleanup_errors=secondary,
                result='FAIL' if primary is not None or secondary else 'PASS')
        except BaseException as exc:
            secondary.append({'action':'close-log',**safe_failure(exc,'close','fixture-log-failed')})
        # A pending bare raise keeps its original exception/traceback. A close or
        # WAL failure without a primary is itself fatal for both fixture kinds.
        if primary is None and secondary:raise FixtureError(secondary[0]['error_code'])
    return legacy

def select_targets(events,operation):
    groups={}
    for e in events:
        if e['op'] not in ('write','sync','truncate') or e['phase']=='open':continue
        key=target_group(event_descriptor(e))
        groups.setdefault(key,[]).append(e)
    result=[]
    for key,items in groups.items():
        positions={'first':0,'middle':len(items)//2,'last':len(items)-1}
        for i in sorted(set(positions.values())):
            result.append((key,make_target(items,i,[name for name,j in positions.items() if i==j])))
    require(result,'no targets discovered')
    descriptors=[p['descriptor'] for _,p in result]
    require(any(d['role']=='wal' and d['meaning']=='sync' for d in descriptors),'WAL sync not observed')
    if operation in ('checkpoint','autocheckpoint'):
        require(any(d['role']=='database' and d['meaning']=='write' for d in descriptors),'checkpoint database writes not observed')
        require(any(d['role']=='database' and d['meaning']=='sync' for d in descriptors),'checkpoint database sync not observed')
    if operation=='autocheckpoint':require(sum(e['op']=='write' and e['role']=='wal' and e['offset']==0 for e in events)>=2,'WAL reset after automatic checkpoint missing')
    if operation=='checkpoint':require(any(d['role']=='wal' and d['meaning']=='truncate' for d in descriptors),'WAL truncation missing')
    require(any(d['meaning']=='wal-commit-marker' for d in descriptors),'commit marker not identified')
    return sorted(result,key=lambda v:v[1]['baseline_seq'])

def matrix_requests(requests):
    # Order only: keep prerequisite create controls above this call and retain
    # every required family. No historical run contributes to fresh accounting.
    by_operation={request['Operation']:request for request in requests}
    require(len(by_operation)==len(requests) and set(by_operation)==set(MATRIX_ORDER)==set(REQUIRED_OPERATIONS),
            'matrix operation families missing or duplicated')
    return [by_operation[operation] for operation in MATRIX_ORDER]


def negative_oracles(worker,root,before,after,request,ack,image,m2schema):
    # Inspect one fresh positive image first. Only copies of its observed oracle
    # input are corrupted; a failed inspector cannot masquerade as rejection.
    path=materialize(root,'negative-positive-control',image)
    observed=inspect(worker,path)
    require(observed==after,'negative-control positive inspection changed state')
    require(oracle(before,observed,request,ack,m2schema)=='present','negative-control positive oracle failed')
    remove_owned(root,path)
    expect_reject('lost-acknowledged-mutation','acknowledged mutation lost/changed',
                  lambda:oracle(before,before,request,ack,m2schema))
    for name,reason in (
        ('broken-audit-linkage','event/revision pair'),
        ('broken-revision-linkage','head/snapshot mismatch'),
        ('broken-idempotency-linkage','idempotency revision absent'),
    ):
        bad=copy.deepcopy(observed)
        if name=='broken-audit-linkage':
            bad['mutation_audit'][-1]['revision_number']+=100
        elif name=='broken-revision-linkage':
            current=bad['records'][0]
            head=next(v for v in bad['record_revisions'] if v['record_id']==current['id'] and v['revision_number']==current['revision'])
            head['data']='{"deliberately":"broken"}'
        else:
            key=next(k for k in bad['idempotency_keys'] if k['key']==request['Key'])
            response=precise(key['response']);response['revision']=('number','999999999')
            key['response']=canonical(response)
        expect_reject(name,reason,lambda:oracle(before,bad,request,ack,m2schema))
    require(oracle(before,observed,request,ack,m2schema)=='present','negative controls modified positive state')
    log(kind='validation',name='oracle-negative-controls',result='PASS')

def no_fault_diagnostic(worker, root):
    # This entry has no path to model_validation/raw_validation, negative_oracles,
    # replay, target discovery, or Model.crash. No injected fault or crash kill.
    fixture=owned(root,'fixture');run(worker,'-root',fixture,'-mode','fixture')
    before=inspect(worker,fixture);consistency(before)
    initial=image_files(fixture);schema=before['sqlite_schema']
    request={'Operation':'create','Key':'r3-diagnostic-create','Body':checkpoint_body(before)}
    for native, name in ((True,'diagnostic-native'),(False,'diagnostic-instrumented')):
        result=execute(worker,root,name,request,initial,native=native,diagnostic=True)
        settings=[m['value'] for m in result['messages'] if m['kind']=='settings']
        require(len(settings)==1,'diagnostic settings missing/repeated')
        check_settings(settings[0])
        responses=[m for m in result['messages'] if m['kind']=='response']
        states=[m['value'] for m in result['messages'] if m['kind']=='state']
        require(len(responses)==len(result['ack'])==len(states)==1 and responses[0]['ok'],
                'diagnostic requires exactly one accepted mutation and consistent inspection')
        oracle(before,states[0],request,result['ack'],schema)
        if not native: check_mmap_coverage(result['messages'],settings[0])
        log(kind='no-fault-diagnostic-control', name=name, result='PASS',
            state_sha256=digest(encoded(states[0])), response_sha256=digest(encoded(responses[0])),
            trace_sha256=result.get('trace_sha256'), ledger_sha256=digest((root/'acks.jsonl').read_bytes()))
    # Diagnostic success cannot close an acceptance case or unlock the matrix.
    log(kind='FINAL', **IDENTITY, mode='no-fault-diagnostic', diagnostic='PASS',
        fault_schedules='NOT RUN', negative_controls='NOT RUN', T18='PARTIAL', T30='BLOCKED', R3='OPEN', artifact_upload=False)
    remove_owned(root,fixture)
    require(root.parent.resolve()==Path(os.environ['RUNNER_TEMP']).resolve() and (root/MARKER).is_file(),'final cleanup ownership')
    shutil.rmtree(root)


def main():
    parser=argparse.ArgumentParser()
    parser.add_argument('--mode',choices=('acceptance','no-fault-diagnostic'),default='acceptance')
    parser.add_argument('--worker',type=Path,required=True)
    parser.add_argument('--m1-binary',type=Path)
    args=parser.parse_args()
    if args.mode=='acceptance' and args.m1_binary is None: parser.error('acceptance requires --m1-binary')
    require(os.environ.get('GITHUB_ACTIONS')=='true' and os.environ.get('GITHUB_REPOSITORY')=='estul26/Contextarium' and os.environ.get('RUNNER_OS')=='Linux','R3 execution restricted to authorized GitHub Linux job')
    require(os.environ.get('GITHUB_RUN_ATTEMPT')=='1','R3 automatic reruns are not authorized')
    require(os.environ.get('RUNNER_ENVIRONMENT')=='github-hosted','requires approved standard hosted runner')
    worker=args.worker.resolve()
    sources=('internal/r3/worker/main.go','internal/r3/vfs/vfs.c','internal/r3/vfs/vfs.go','scripts/r3-check.py')
    IDENTITY.update(harness=run('git','rev-parse','HEAD').decode().strip(),
                    source_sha256={name:digest(Path(name).read_bytes()) for name in sources},
                    worker_sha256=digest(worker.read_bytes()))
    root=Path(tempfile.mkdtemp(prefix='contextarium-r3-',dir=os.environ['RUNNER_TEMP']));(root/MARKER).write_text('owned\n')
    require(shutil.disk_usage(root).free>=8*1024**3,'initial scratch/free reserve unavailable')
    log(kind='manifest', **IDENTITY, mode=args.mode, seeds=SEEDS if args.mode=='acceptance' else [],model='write-back-v1; byte-range tears; ordered namespace/truncate epochs; successful file-sync barriers',artifact_upload=False)
    if args.mode=='no-fault-diagnostic':
        no_fault_diagnostic(worker,root)
        return
    m1=args.m1_binary.resolve()
    # No fault acceptance runs occur until every validation gate has passed.
    model_validation();raw_validation(worker,root)
    fixture=owned(root,'fixture');run(worker,'-root',fixture,'-mode','fixture')
    before=inspect(worker,fixture);consistency(before);initial=image_files(fixture);schema=before['sqlite_schema']
    archived=next(r for r in before['records'] if r['status']=='archived');active=next(r for r in before['records'] if r['status']=='active')
    requests=[
        {'Operation':'create','Key':'r3-change','Body':checkpoint_body(before)},
        {'Operation':'patch','ID':archived['id'],'Key':'r3-change','Body':'{"base_revision":2,"data":{"n":9007199254740993,"d":0.30,"e":1e2,"z":-0,"changed":true}}'},
        {'Operation':'metadata','ID':archived['id'],'Key':'r3-change','Body':'{"base_revision":2,"key":"changed","sensitivity":"restricted","provenance":{"source":"synthetic-r3"}}'},
        {'Operation':'archive','ID':active['id'],'Key':'r3-change','Body':'{"base_revision":2,"status":"archived"}'},
        {'Operation':'unarchive','ID':archived['id'],'Key':'r3-change','Body':'{"base_revision":2,"status":"active"}'},
        {'Operation':'noop','ID':archived['id'],'Key':'r3-change','Body':'{"base_revision":2,"status":"archived"}'},
        {'Operation':'restore','ID':archived['id'],'Key':'r3-change','Target':1,'Body':'{"base_revision":2}'},
        {'Operation':'checkpoint','Body':'{}'}, {'Operation':'autocheckpoint','Body':'{}'},
    ]
    native=execute(worker,root,'validation-native',requests[0],initial,native=True)
    p=materialize(root,'native-validation',native['image']);oracle(before,inspect(worker,p),requests[0],native['ack'],schema);remove_owned(root,p)
    log(kind='validation',name='actual-application-native-VFS-control',result='PASS')
    baseline=execute(worker,root,'validation-create',requests[0],initial)
    image=baseline['model'].crash('retain',17);p=materialize(root,'validation-recovery',image);after=inspect(worker,p)
    oracle(before,after,requests[0],baseline['ack'],schema);replay(worker,root,p,before,requests[0],baseline['ack'],schema);remove_owned(root,p)
    negative_oracles(worker,root,before,after,requests[0],baseline['ack'],image,schema)
    log(kind='gate',harness_validation='PASS',acceptance_matrix='STARTING')
    old=owned(root,'m1');legacy=m1_fixture(m1,old);oldstate=inspect(worker,old);oldimage=image_files(old)
    emptyold=owned(root,'empty-m1');m1_fixture(m1,emptyold,empty=True);emptystate=inspect(worker,emptyold);emptyimage=image_files(emptyold)
    requests += [{'Operation':'migration','Body':'{}','Legacy':legacy},{'Operation':'fresh','Body':'{}'},{'Operation':'empty-m1','Body':'{}'}]
    cases=0;recoveries=0;coverage=set();manifest=hashlib.sha256();acceptance=AcceptanceCoverage()
    for request in matrix_requests(requests):
        op=request['Operation'];seedstate=oldstate if op=='migration' else before;seedimage=oldimage if op=='migration' else initial
        if op=='empty-m1':seedstate,seedimage=emptystate,emptyimage
        if op=='fresh':
            seedimage={'store.db':b''};empty=materialize(root,'empty',seedimage);seedstate=inspect(worker,empty);remove_owned(root,empty)
        for sector in (512,4096):
            base=execute(worker,root,op+'-baseline',request,seedimage,sector=sector)
            require(all(m['ok'] for m in base['messages'] if m['kind']=='response'),'no-fault baseline failed')
            settings=next(m['value'] for m in base['messages'] if m['kind']=='settings')
            check_settings(settings)
            check_mmap_coverage(base['messages'],settings)
            log(kind='settings',operation=op,sector=sector,value={q:setting_value(q,v) for q,v in settings.items() if q in SETTINGS})
            p=materialize(root,'baseline',base['model'].crash('retain',17));oracle(seedstate,inspect(worker,p),request,base['ack'],schema);remove_owned(root,p)
            targets=select_targets(base['events'],op)
            acceptance.plan(op,sector,targets)
            log(kind='schedule',operation=op,sector=sector,
                selection='discovery first-middle-last per stable selector group; Nth live member before injection; exact live private trace; no substitution',
                targets=[target_summary(p) for _,p in targets])
            for key,target in targets:
                for fault in fault_modes(target):
                    case=f'{op}-s{sector}-b{target["baseline_seq"]}-{fault}'
                    got=execute(worker,root,case,request,seedimage,target,fault,sector)
                    reached=got['all'][got['target_seq']]
                    evidence=target_evidence(target,reached)
                    schedules=RECOVERY_SCHEDULES
                    results=[]
                    for schedule,seed in schedules:
                        image=got['model'].crash(schedule,seed)
                        p=materialize(root,'recovery',image)
                        # FIRST SQLite connection after reconstruction: separate process, no migration.
                        state=inspect(worker,p);outcome=oracle(seedstate,state,request,got['ack'],schema)
                        replay(worker,root,p,seedstate,request,got['ack'],schema)
                        item={'schedule':schedule,'seed':seed,'outcome':outcome,'image_sha256':digest(encoded({n:digest(b) for n,b in image.items()})),'state_sha256':digest(encoded(state))}
                        results.append(item);recoveries+=1;remove_owned(root,p)
                    report={'kind':'case','case':case,'phase':reached['phase'],'role':reached['role'],'op':reached['op'],'offset':reached['offset'],'length':reached['length'],'occurrence':reached['seq'],**evidence,'acknowledged_before_fault':got['ack_before_fault'],'trace_sha256':got['trace_sha256'],'ledger_sha256':got['ack_sha256'],'acknowledged':len(got['ack']),'results':results,'result':'PASS'}
                    acceptance.complete(op,sector,target,fault,results,got['ack_before_fault'])
                    log(**report);manifest.update(encoded(report));cases+=1;coverage.add((op,reached['role'],target['descriptor']['meaning'],fault))
    acceptance.require_complete()
    log(kind='FINAL',harness_validation='PASS',T18='PASS',T30='PASS',R3='PASS within write-back-v1 model',cases=cases,recoveries=recoveries,coverage=sorted(coverage),manifest_sha256=manifest.hexdigest(),artifact_upload=False,paid_usage_authorized=0)
    # Only the marked, test-created root is removed, after results/hashes were logged.
    require(root.parent.resolve()==Path(os.environ['RUNNER_TEMP']).resolve() and (root/MARKER).is_file(),'final cleanup ownership')
    shutil.rmtree(root)

if __name__=='__main__':
    try:main()
    except Exception as exc:
        try:report_controller_failure(exc)
        except BaseException:pass  # A broken log sink must not expose a raw traceback.
        raise SystemExit(1) from None
