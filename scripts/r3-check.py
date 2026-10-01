#!/usr/bin/env python3
"""Bounded, runner-only R3 controller. No production switches or artifact upload."""
import argparse
import copy
import hashlib
import http.client
import json
import os
from pathlib import Path
import random
import selectors
import shutil
import signal
import socket
import sqlite3
import subprocess
import tempfile
import time

CANDIDATE = 'b6f63a7564977555faffe6f9ca6b1a9c22910d43'
SEEDS = (17, 29, 101)
TABLES = ('subjects', 'schemas', 'records', 'record_revisions', 'mutation_audit', 'idempotency_keys', 'schema_migrations', 'sqlite_schema')
SNAPSHOT = ('subject_id','namespace','schema_id','schema_version','data','key','sensitivity','provenance','status','created_at','updated_at')
MARKER = '.contextarium-r3-owned'
DEADLINE = time.monotonic() + 3300

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
    return subprocess.check_output([str(a) for a in args], timeout=timeout, stderr=subprocess.PIPE)

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

def execute(worker, root, case, request, initial, target=0, fault='none', sector=4096, probe=False, native=False):
    budget(root)
    live = owned(root, 'live') if probe else materialize(root, 'live', initial)
    config = root / 'input.json'
    write_input(config, request)
    trace = root / 'trace.jsonl'
    if trace.exists(): trace.unlink() # Controller-owned file in the marked run root.
    command = [str(worker), '-root', str(live), '-mode', 'probe' if probe else ('native' if native else 'run'), '-trace', str(trace), '-target', str(target), '-fault', fault, '-sector', str(sector)]
    if not probe: command += ['-input',str(config)]
    ack = []; messages = []; hit = None
    # No credentials or Actions token are inherited by the instrumented process.
    env = {k:v for k,v in os.environ.items() if k in ('PATH','LANG','TZ','TMPDIR')}
    with open(root / 'worker-stderr.txt', 'wb') as stderr, open(root / 'acks.jsonl','wb') as ledger:
        child = subprocess.Popen(command, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=stderr, env=env)
        selector = selectors.DefaultSelector();selector.register(child.stdout, selectors.EVENT_READ)
        buffer = b''; finished = False; deadline = time.monotonic()+45
        try:
            while not finished:
                require(time.monotonic() < deadline, 'child response/barrier timeout')
                if not selector.select(1): continue
                part = os.read(child.stdout.fileno(),65536)
                require(part, 'child exited without result/barrier')
                buffer += part
                while b'\n' in buffer:
                    line, buffer = buffer.split(b'\n',1)
                    msg = json.loads(line);messages.append(msg)
                    if msg['kind'] == 'target':
                        require(hit is None and msg['seq'] == target and msg['mode'] == fault, 'wrong/repeated target')
                        hit=msg
                        if fault.startswith('cut-'): finished=True;break
                    if msg['kind'] == 'response' and msg['ok']:
                        # An acknowledged result is recorded only after complete receipt.
                        ack.append(msg)
                        ack_request={'Operation':'create','Key':msg['key'],'Body':msg['body_text']} if 'body_text' in msg else request
                        entry={'case':case,'request_sha256':digest(encoded(ack_request)),'response':msg,'response_sha256':digest(encoded(msg))}
                        ledger.write(encoded(entry)+b'\n');ledger.flush();os.fsync(ledger.fileno())
                    if msg['kind'] == 'done': finished=True;break
            child.kill();child.wait(timeout=5)
            require(child.returncode == -signal.SIGKILL, 'test child did not die without cleanup')
        finally:
            if child.poll() is None: child.kill();child.wait(timeout=5)
            child.stdin.close();child.stdout.close();selector.close()
    require((target == 0) == (hit is None), 'requested fault not reached')
    if native:
        image=image_files(live);remove_owned(root,live)
        return {'image':image,'ack':ack,'messages':messages}
    events, all_events=read_trace(trace)
    if hit:
        require(target in all_events, 'target missing from trace')
        chosen=all_events[target]
        require(chosen['op'] in ('write','sync','truncate'), 'fault outside approved operation')
    model=Model(initial)
    for event in events: model.consume(event)
    # Native file writes not intercepted by the VFS must fail, never be silently ignored.
    require(image_files(live) == model.live, 'live file/model mismatch: untraced I/O or broken model')
    result={'events':events,'all':all_events,'model':model,'ack':ack,'messages':messages,'trace_sha256':digest(trace.read_bytes()),'ack_sha256':digest((root/'acks.jsonl').read_bytes())}
    remove_owned(root,live)
    return result

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

def expect_reject(name, callback):
    try: callback()
    except (AssertionError, subprocess.CalledProcessError, KeyError, ValueError, StopIteration):
        log(kind='negative-control',name=name,result='PASS');return
    raise AssertionError('negative control failed to reject: '+name)

def model_validation():
    def event(op,name='store.db',**kwargs):return dict(dict(op=op,name=name,rc=0,applied=1,flags=0,offset=0,hex=''),**kwargs)
    def check(model_class):
        m=model_class({})
        m.consume(event('open'))
        m.consume(event('write',hex='616263646566',applied=6))
        require(m.live['store.db']==b'abcdef' and 'store.db' not in m.crash('discard',17),'volatile visibility/create')
        m.consume(event('sync'))
        require(m.crash('discard',17)['store.db']==b'abcdef','successful sync persistence')
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
    expect_reject('incorrect-sync-semantics',lambda:check(BrokenSync))
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
    failed=execute(worker,root,'probe-failed-sync',{}, {},target=firstsync['seq'],fault='ioerr',probe=True)
    require(any(m.get('kind')=='response' and not m['ok'] for m in failed['messages']),'failed sync was reported successful')
    require('probe.db' not in failed['model'].crash('discard',17),'native failed-sync behavior')
    require(any(e['op']=='truncate' and e['offset']==4 for e in result['events']),'truncate not intercepted')
    log(kind='validation',name='native-VFS-visibility-sync-truncate-namespace',result='PASS')

def m1_fixture(binary,root,empty=False):
    path=root/'store.db'
    with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
    env={k:v for k,v in os.environ.items() if k in ('PATH','LANG','TZ')}
    env.update(CONTEXTARIUM_LISTEN_ADDR=f'127.0.0.1:{port}',CONTEXTARIUM_DB_PATH=str(path))
    child=subprocess.Popen([str(binary)],env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
    def http(method,url,body=None,key=None):
        conn=http.client.HTTPConnection('127.0.0.1',port,timeout=2)
        headers={'Content-Type':'application/json'}
        if key:headers['Idempotency-Key']=key
        try:
            conn.request(method,url,body.encode() if body else None,headers);r=conn.getresponse();raw=r.read()
            require(r.status in (200,201),'M1 fixture HTTP failure')
            return json.loads(raw) if raw else None
        finally:conn.close()
    try:
        until=time.monotonic()+10
        while True:
            try:http('GET','/readyz');break
            except (OSError,AssertionError):
                require(time.monotonic()<until and child.poll() is None,'M1 readiness');time.sleep(.02)
        if empty: return []
        sub=http('POST','/api/v1/subjects','{"kind":"project","display_name":"Synthetic R3 M1"}','subject')['data']['id']
        http('POST','/api/v1/schemas','{"schema_id":"example.r3","schema_version":1,"definition":{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"},"migration_policy":"explicit"}','schema')
        body='{"subject_id":'+json.dumps(sub)+',"namespace":"example.r3","schema_id":"example.r3","schema_version":1,"data":{"n":9007199254740993,"d":0.30,"e":1e2,"z":-0,"unknown":["雪",true]}}'
        old=http('POST','/api/v1/records',body,'legacy-create')['data']
        http('POST','/api/v1/records',body,'legacy-other')
        http('PATCH','/api/v1/records/'+old['id'],'{"status":"archived"}','legacy-update')
        legacy=[{'Operation':'create','Key':'legacy-create','Body':body},{'Operation':'patch','ID':old['id'],'Key':'legacy-update','Body':'{"status":"archived"}'}]
    finally:
        if child.poll() is None:child.send_signal(signal.SIGTERM)
        child.communicate(timeout=10)
        require(child.returncode==0,'M1 fixture graceful close')
    require(not (root/'store.db-wal').exists(),'M1 fixture not closed')
    return legacy

def select_targets(events,operation):
    groups={}
    for e in events:
        if e['op'] not in ('write','sync','truncate') or e['phase']=='open':continue
        meaning=e['op']
        if e['op']=='write' and e['role']=='wal':
            data=bytes.fromhex(e['hex'])
            meaning='wal-frame'
            if e['offset']==0:meaning='wal-header-reset'
            elif e['length']==24 and int.from_bytes(data[4:8],'big')!=0:meaning='wal-commit-marker'
        key=(e['phase'],e['role'],meaning)
        groups.setdefault(key,[]).append(e)
    result=[]
    for key,items in groups.items():
        for i in sorted({0,len(items)//2,len(items)-1}):result.append((key,items[i]))
    require(result,'no targets discovered')
    require(any(k[1]=='wal' and k[2]=='sync' for k in groups),'WAL sync not observed')
    if operation in ('checkpoint','autocheckpoint'):
        require(any(k[1]=='database' and k[2]=='write' for k in groups),'checkpoint database writes not observed')
        require(any(k[1]=='database' and k[2]=='sync' for k in groups),'checkpoint database sync not observed')
    if operation=='autocheckpoint':require(sum(e['op']=='write' and e['role']=='wal' and e['offset']==0 for e in events)>=2,'WAL reset after automatic checkpoint missing')
    if operation=='checkpoint':require(any(k[1]=='wal' and k[2]=='truncate' for k in groups),'WAL truncation missing')
    require(any(k[2]=='wal-commit-marker' for k in groups),'commit marker not identified')
    return sorted(result,key=lambda v:v[1]['seq'])

def negative_oracles(worker,root,before,after,request,ack,image,m2schema):
    expect_reject('lost-acknowledged-mutation',lambda:oracle(before,before,request,ack,m2schema))
    for name,statement in (
        ('broken-audit-linkage','DROP TRIGGER mutation_audit_no_delete; DELETE FROM mutation_audit WHERE rowid=(SELECT max(rowid) FROM mutation_audit);'),
        ('broken-revision-linkage','DROP TRIGGER record_revisions_no_delete; DELETE FROM record_revisions WHERE rowid=(SELECT max(rowid) FROM record_revisions);'),
        ('broken-idempotency-linkage',"UPDATE idempotency_keys SET response='{}' WHERE key='r3-change';"),
    ):
        path=materialize(root,'negative',image)
        # Explicit negative-control fixture only; production guards never change.
        with sqlite3.connect(path/'store.db') as db:db.executescript(statement)
        def rejected():
            state=inspect(worker,path)
            oracle(before,state,request,ack,m2schema)
        expect_reject(name,rejected)
        remove_owned(root,path)
    log(kind='validation',name='oracle-negative-controls',result='PASS')

def main():
    parser=argparse.ArgumentParser();parser.add_argument('--worker',type=Path,required=True);parser.add_argument('--m1-binary',type=Path,required=True);args=parser.parse_args()
    require(os.environ.get('GITHUB_ACTIONS')=='true' and os.environ.get('GITHUB_REPOSITORY')=='estul26/Contextarium' and os.environ.get('RUNNER_OS')=='Linux','R3 execution restricted to authorized GitHub Linux job')
    require(os.environ.get('GITHUB_RUN_ATTEMPT')=='1','R3 automatic reruns are not authorized')
    worker=args.worker.resolve();m1=args.m1_binary.resolve()
    root=Path(tempfile.mkdtemp(prefix='contextarium-r3-',dir=os.environ['RUNNER_TEMP']));(root/MARKER).write_text('owned\n')
    require(shutil.disk_usage(root).free>=8*1024**3,'initial scratch/free reserve unavailable')
    log(kind='manifest',candidate=CANDIDATE,harness=run('git','rev-parse','HEAD').decode().strip(),seeds=SEEDS,model='write-back-v1; byte-range tears; ordered namespace/truncate epochs; successful file-sync barriers',artifact_upload=False)
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
    cases=0;recoveries=0;coverage=set();manifest=hashlib.sha256()
    for request in requests:
        op=request['Operation'];seedstate=oldstate if op=='migration' else before;seedimage=oldimage if op=='migration' else initial
        if op=='empty-m1':seedstate,seedimage=emptystate,emptyimage
        if op=='fresh':
            seedimage={'store.db':b''};empty=materialize(root,'empty',seedimage);seedstate=inspect(worker,empty);remove_owned(root,empty)
        for sector in (512,4096):
            base=execute(worker,root,op+'-baseline',request,seedimage,sector=sector)
            require(all(m['ok'] for m in base['messages'] if m['kind']=='response'),'no-fault baseline failed')
            settings=next(m['value'] for m in base['messages'] if m['kind']=='settings')
            require(settings['sqlite_version()']=='3.53.4' and settings['foreign_keys']==1 and settings['journal_mode']=='wal' and settings['synchronous']==2 and settings['busy_timeout']==1000 and settings['max_open_connections']==1,'candidate settings changed')
            log(kind='settings',operation=op,sector=sector,value=settings)
            p=materialize(root,'baseline',base['model'].crash('retain',17));oracle(seedstate,inspect(worker,p),request,base['ack'],schema);remove_owned(root,p)
            targets=select_targets(base['events'],op)
            log(kind='schedule',operation=op,sector=sector,selection='first-middle-last per phase/role/semantic; all listed targets mandatory',targets=[{'seq':e['seq'],'phase':e['phase'],'role':e['role'],'op':e['op'],'meaning':key[2],'offset':e['offset'],'length':e['length']} for key,e in targets])
            for key,target in targets:
                modes=['cut-before','cut-after','ioerr']
                if target['op'] in ('write','truncate'):modes+=['full']
                if target['op']=='write':modes+=['partial']
                for fault in modes:
                    case=f'{op}-s{sector}-{target["seq"]}-{fault}'
                    got=execute(worker,root,case,request,seedimage,target['seq'],fault,sector)
                    reached=got['all'][target['seq']]
                    require(all(reached[k]==target[k] for k in ('op','name','role','offset','length','phase','flags')),'fault schedule target drift')
                    schedules=[('discard',17),('retain',17)]+[('reorder-torn',seed) for seed in SEEDS]
                    results=[]
                    for schedule,seed in schedules:
                        image=got['model'].crash(schedule,seed)
                        p=materialize(root,'recovery',image)
                        # FIRST SQLite connection after reconstruction: separate process, no migration.
                        state=inspect(worker,p);outcome=oracle(seedstate,state,request,got['ack'],schema)
                        replay(worker,root,p,seedstate,request,got['ack'],schema)
                        item={'schedule':schedule,'seed':seed,'outcome':outcome,'image_sha256':digest(encoded({n:digest(b) for n,b in image.items()})),'state_sha256':digest(encoded(state))}
                        results.append(item);recoveries+=1;remove_owned(root,p)
                    report={'kind':'case','case':case,'phase':reached['phase'],'role':reached['role'],'op':reached['op'],'offset':reached['offset'],'length':reached['length'],'occurrence':target['seq'],'trace_sha256':got['trace_sha256'],'ledger_sha256':got['ack_sha256'],'acknowledged':len(got['ack']),'results':results,'result':'PASS'}
                    log(**report);manifest.update(encoded(report));cases+=1;coverage.add((op,reached['role'],key[2],fault))
    log(kind='FINAL',harness_validation='PASS',T18='PASS',T30='PASS',R3='PASS within write-back-v1 model',cases=cases,recoveries=recoveries,coverage=sorted(coverage),manifest_sha256=manifest.hexdigest(),artifact_upload=False,paid_usage_authorized=0)
    # Only the marked, test-created root is removed, after results/hashes were logged.
    require(root.parent.resolve()==Path(os.environ['RUNNER_TEMP']).resolve() and (root/MARKER).is_file(),'final cleanup ownership')
    shutil.rmtree(root)

if __name__=='__main__':
    try:main()
    except Exception as exc:
        log(kind='FINAL',result='FAIL',error_type=type(exc).__name__,reason=str(exc)[:500],T18='PARTIAL',T30='FAIL or incomplete; inspect reached cases')
        raise
