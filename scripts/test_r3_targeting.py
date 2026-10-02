"""Source-review-only fake-trace targeting regressions; NOT RUN.

No SQLite, worker, fixture binary, HTTP or subprocess is executed by these tests.
Ordinary CI does not run this file. Separate execution approval is required.
"""
import ast
import importlib.util
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

from r3_wal_test_vectors import wal_header, frame_header, frame_offset, page_offset


class TargetingTests(unittest.TestCase):
    def setUp(self):
        source=Path(__file__).with_name('r3-check.py')
        spec=importlib.util.spec_from_file_location('r3_targeting_test_controller',source)
        self.c=importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.c)
        logger=mock.patch.object(self.c,'log');self.log=logger.start()
        self.addCleanup(logger.stop)
        self.directory=tempfile.TemporaryDirectory(prefix='contextarium-target-unit-')
        self.addCleanup(self.directory.cleanup)
        self.root=Path(self.directory.name)
        (self.root/self.c.MARKER).write_text('synthetic test ownership\n')
        self.trace=self.root/'synthetic-trace.jsonl'
        self.sink=io.BytesIO()

    def event(self, seq=47, *, phase='mutation-noop', name='store.db-wal',
              op='write', offset=None, length=24, flags=0, commit=True, page_size=4096):
        if offset is None:offset=frame_offset(21,page_size) if length==24 else page_offset(21,page_size)
        data=bytes(length) if op=='write' else b''
        if op=='write' and name.endswith('-wal'):
            if offset==0 and length==32:data=wal_header(page_size)
            elif length==24:data=frame_header(page_size,commit=commit)
        return dict(seq=seq,stage='pre',phase=phase,name=name,
                    role='wal' if name.endswith('-wal') else ('journal' if name.endswith('-journal') else 'database'),op=op,
                    offset=offset,length=length,flags=flags,hex=data.hex(),
                    wal_header=wal_header(page_size).hex() if name.endswith('-wal') else '')

    def message(self, event):
        return dict(self.c.event_descriptor(event),seq=event['seq'],kind='io-candidate')

    def plan(self, items=None, index=0):
        items=items or [self.event()]
        positions={'first':0,'middle':len(items)//2,'last':len(items)-1}
        return self.c.make_target(items,index,[name for name,position in positions.items() if position==index])

    def deliver(self, matcher, event, acknowledgements=None):
        self.trace.write_text(json.dumps(event)+'\n')
        self.c.decide_io(self.sink,self.trace,matcher,self.message(event),acknowledgements or [])

    def test_unrelated_earlier_events_do_not_move_intended_target(self):
        matcher=self.c.TargetMatcher(self.plan())
        self.deliver(matcher,self.event(8,phase='open'))
        self.deliver(matcher,self.event(17,op='sync',offset=0,length=0,flags=2))
        self.deliver(matcher,self.event(25,offset=56,length=4096,commit=False))
        self.deliver(matcher,self.event(91))
        self.assertEqual(self.sink.getvalue(),b'c 8\nc 17\nc 25\ni 91\n')
        matcher.require_reached({'seq':91})
        self.assertEqual(matcher.plan['baseline_seq'],47)

    def assert_unrelated(self, event):
        matcher=self.c.TargetMatcher(self.plan())
        self.deliver(matcher,event)
        self.assertEqual(self.sink.getvalue(),f"c {event['seq']}\n".encode())
        with self.assertRaisesRegex(AssertionError,'required descriptor target not reached'):
            matcher.require_reached(None)

    def test_wrong_phase_never_receives_fault(self):
        self.assert_unrelated(self.event(47,phase='mutation-restore'))

    def test_wrong_role_never_receives_fault(self):
        self.assert_unrelated(self.event(47,name='store.db'))

    def test_wrong_semantic_operation_never_receives_fault(self):
        self.assert_unrelated(self.event(47,commit=False))

    def test_changed_range_is_live_sample_geometry_for_wal_database_and_journal(self):
        for name,allowed in [('store.db-wal',True),('store.db',True),('store.db-journal',True)]:
            with self.subTest(name=name):
                self.sink=io.BytesIO();original=self.event(name=name)
                matcher=self.c.TargetMatcher(self.plan([original]))
                self.deliver(matcher,dict(original,offset=94792))
                self.assertEqual(self.sink.getvalue(),b'i 47\n' if allowed else b'c 47\n')
                self.assertEqual(matcher.seen,int(allowed))
                if not allowed:
                    with self.assertRaisesRegex(AssertionError,'required descriptor target not reached'):
                        matcher.require_reached(None)

    def test_first_commit_marker_at_different_append_offset_is_selected(self):
        plan=self.plan([self.event(39,offset=70072),self.event(41,offset=74192),
                        self.event(42,offset=78312)])
        self.assertEqual(plan['samples'],['first'])
        self.assertEqual(plan['group_position'],1)
        self.assertEqual(plan['group_count'],3)
        matcher=self.c.TargetMatcher(plan)
        # Sixth-run metadata reproduced synthetically, not a rerun or claim of
        # actual fault/recovery success for that historical failed case.
        self.deliver(matcher,self.event(39,offset=70072,commit=False))
        self.deliver(matcher,self.event(44,offset=78336,length=4096,commit=False))
        self.deliver(matcher,self.event(45,offset=82432))
        matcher.require_reached({'seq':45})
        self.assertEqual(self.sink.getvalue(),b'c 39\nc 44\ni 45\n')
        self.assertEqual(matcher.seen,1)

    def test_baseline_and_live_geometry_are_retained_in_diagnostics_and_report(self):
        plan=self.plan([self.event(39,offset=70072)])
        live=self.event(45,offset=82432)
        matcher=self.c.TargetMatcher(plan);self.deliver(matcher,live)
        diagnostic=matcher.diagnostics();report=self.c.target_evidence(plan,live)
        for result in (diagnostic,report):
            self.assertEqual(result['planned']['baseline_seq'],39)
            self.assertEqual(result['planned']['descriptor']['offset'],70072)
            self.assertEqual(result['planned']['selector']['offset_policy'],'positive-wal-append')
            self.assertNotIn('offset',result['planned']['selector'])
        self.assertEqual(diagnostic['selected_observed'],report['observed'])
        self.assertEqual(report['observed']['seq'],45)
        self.assertEqual(report['observed']['offset'],82432)
        self.assertEqual(report['observed']['length'],24)
        self.assertEqual(report['observed']['flags'],0)
        self.assertEqual(report['geometry_changed'],['offset'])
        record=self.log.call_args.kwargs
        self.assertEqual(record['kind'],'target-geometry')
        self.assertEqual(record['baseline']['seq'],39)
        self.assertEqual(record['baseline']['offset'],70072)
        self.assertEqual(record['observed'],report['observed'])
        self.assertNotIn('hex',json.dumps(record))

    def test_wrong_file_with_same_role_never_receives_injection(self):
        plan=self.plan([self.event(name='store.db',length=4096)])
        matcher=self.c.TargetMatcher(plan)
        self.deliver(matcher,self.event(name='probe.db',length=4096))
        self.assertEqual(self.sink.getvalue(),b'c 47\n')
        self.assertEqual(matcher.seen,0)
        with self.assertRaisesRegex(AssertionError,'required descriptor target not reached'):
            matcher.require_reached(None)

    def test_full_page_and_fragment_are_different_sampling_buckets(self):
        plan=self.plan([self.event(length=4096,commit=False)])
        matcher=self.c.TargetMatcher(plan)
        self.deliver(matcher,self.event(offset=94816,length=2048,commit=False))
        self.assertEqual(self.sink.getvalue(),b'c 47\n')
        self.assertEqual(matcher.seen,0)
        with self.assertRaisesRegex(AssertionError,'required descriptor target not reached'):
            matcher.require_reached(None)

    def test_commit_marker_requires_24_byte_shape(self):
        matcher=self.c.TargetMatcher(self.plan());message=self.message(self.event())
        message['length']=23
        with self.assertRaisesRegex(AssertionError,'invalid WAL frame-header geometry'):
            self.c.decide_io(self.sink,self.trace,matcher,message,[])
        self.assertEqual(self.sink.getvalue(),b'')

    def test_wal_header_reset_requires_offset_zero(self):
        header=self.event(offset=0,length=32,commit=False)
        matcher=self.c.TargetMatcher(self.plan([header]));message=self.message(header)
        message['offset']=32
        with self.assertRaisesRegex(AssertionError,'invalid WAL reset geometry'):
            self.c.decide_io(self.sink,self.trace,matcher,message,[])
        self.assertEqual(self.sink.getvalue(),b'')

    def test_wal_append_cannot_claim_offset_zero(self):
        matcher=self.c.TargetMatcher(self.plan());message=self.message(self.event())
        message['offset']=0
        with self.assertRaisesRegex(AssertionError,'missing WAL frame geometry'):
            self.c.decide_io(self.sink,self.trace,matcher,message,[])
        self.assertEqual(self.sink.getvalue(),b'')

    def test_truncate_size_remains_identity(self):
        event=self.event(op='truncate',offset=4096,length=0)
        matcher=self.c.TargetMatcher(self.plan([event]))
        changed=dict(event,offset=0)
        self.assertNotEqual(self.c.target_group(self.message(event)),self.c.target_group(self.message(changed)))
        self.deliver(matcher,changed)
        self.assertEqual(self.sink.getvalue(),b'c 47\n')
        self.assertEqual(matcher.seen,0)
        with self.assertRaisesRegex(AssertionError,'required descriptor target not reached'):
            matcher.require_reached(None)

    def test_sync_flags_remain_identity(self):
        event=self.event(op='sync',offset=0,length=0,flags=2)
        matcher=self.c.TargetMatcher(self.plan([event]))
        changed=dict(event,flags=3)
        self.assertNotEqual(self.c.target_group(self.message(event)),self.c.target_group(self.message(changed)))
        self.deliver(matcher,changed)
        self.assertEqual(self.sink.getvalue(),b'c 47\n')
        self.assertEqual(matcher.seen,0)
        with self.assertRaisesRegex(AssertionError,'required descriptor target not reached'):
            matcher.require_reached(None)

    def test_append_flags_remain_identity(self):
        event=self.event()
        matcher=self.c.TargetMatcher(self.plan([event]))
        changed=dict(event,offset=94792,flags=1)
        self.assertNotEqual(self.c.target_group(self.message(event)),self.c.target_group(self.message(changed)))
        self.deliver(matcher,changed)
        self.assertEqual(self.sink.getvalue(),b'c 47\n')
        self.assertEqual(matcher.seen,0)
        with self.assertRaisesRegex(AssertionError,'required descriptor target not reached'):
            matcher.require_reached(None)

    def test_mixed_selector_group_plan_is_rejected(self):
        header=self.event(10,offset=4152,commit=False)
        page=self.event(47,length=4096,commit=False)
        with self.assertRaisesRegex(AssertionError,'mixed target coverage bucket'):
            self.plan([header,page],1)
        self.assertEqual(self.sink.getvalue(),b'')

    def test_append_group_accepts_geometry_but_enforces_nth_position(self):
        items=[self.event(10,offset=4152),self.event(47)]
        matcher=self.c.TargetMatcher(self.plan(items,1))
        self.deliver(matcher,self.event(20,offset=8272))
        self.assertEqual(self.sink.getvalue(),b'c 20\n')
        self.assertIsNone(matcher.selected_seq)
        self.deliver(matcher,self.event(80,offset=94792))
        self.assertEqual(self.sink.getvalue(),b'c 20\ni 80\n')
        self.assertEqual(matcher.seen,2)
        self.assertEqual([c.kwargs['group_position'] for c in self.log.call_args_list],[2])
        self.assertEqual([c.kwargs['baseline']['seq'] for c in self.log.call_args_list],[47])

    def test_indistinguishable_append_members_resolve_only_by_group_position(self):
        plan=self.plan([self.event(10,offset=4152),self.event(47)],1)
        matcher=self.c.TargetMatcher(plan)
        # There is deliberately no hidden transaction/page identity. An extra
        # identical selector counts as a member; the explicit second one wins.
        self.deliver(matcher,self.event(11,offset=4152))
        self.deliver(matcher,self.event(12,offset=8272))
        self.assertEqual(self.sink.getvalue(),b'c 11\ni 12\n')
        self.assertEqual(matcher.selected_seq,12)

    def test_missing_nth_member_fails_despite_other_shapes(self):
        items=[self.event(10,length=4096),self.event(20,length=4096),self.event(30,length=4096)]
        matcher=self.c.TargetMatcher(self.plan(items,2))
        self.deliver(matcher,self.event(40,length=4096))
        self.deliver(matcher,self.event(41,length=24,commit=False))
        self.deliver(matcher,self.event(42,length=4096))
        with self.assertRaisesRegex(AssertionError,'required descriptor target not reached'):
            matcher.require_reached(None)
        self.assertEqual(matcher.seen,2)
        self.assertEqual(self.sink.getvalue(),b'c 40\nc 41\nc 42\n')
        self.assertFalse(matcher.decision_sent)

    def test_descriptor_and_private_trace_verified_before_injection_command(self):
        matcher=self.c.TargetMatcher(self.plan());event=self.event(88,offset=94792)
        self.trace.write_text(json.dumps(event)+'\n')
        ordering=[];verify=self.c.verify_pending_trace
        def checking(trace,message):
            ordering.append('verify');verify(trace,message)
        class Sink(io.BytesIO):
            def write(inner,data):
                self.assertEqual(ordering,['verify'])
                self.assertTrue(matcher.trace_verified)
                ordering.append('command');return super().write(data)
        with mock.patch.object(self.c,'verify_pending_trace',side_effect=checking):
            self.c.decide_io(Sink(),self.trace,matcher,self.message(event),[])
        self.assertEqual(ordering,['verify','command'])

    def test_trace_semantics_disagree_no_injection(self):
        matcher=self.c.TargetMatcher(self.plan())
        self.trace.write_text(json.dumps(self.event(commit=False))+'\n')
        with self.assertRaisesRegex(AssertionError,'pending trace descriptor mismatch'):
            self.c.decide_io(self.sink,self.trace,matcher,self.message(self.event()),[])
        self.assertEqual(self.sink.getvalue(),b'')

    def test_private_trace_geometry_must_match_live_message_exactly(self):
        matcher=self.c.TargetMatcher(self.plan())
        self.trace.write_text(json.dumps(self.event())+'\n')
        with self.assertRaisesRegex(AssertionError,'pending trace descriptor mismatch'):
            self.c.decide_io(self.sink,self.trace,matcher,self.message(self.event(offset=94792)),[])
        self.assertEqual(self.sink.getvalue(),b'')
        self.assertFalse(matcher.trace_verified)
        self.assertFalse(matcher.decision_sent)

    def test_malformed_private_trace_cannot_authorize_injection(self):
        matcher=self.c.TargetMatcher(self.plan());self.trace.write_text('{\n')
        with self.assertRaises(json.JSONDecodeError):
            self.c.decide_io(self.sink,self.trace,matcher,self.message(self.event(offset=94792)),[])
        self.assertEqual(self.sink.getvalue(),b'')
        self.assertFalse(matcher.decision_sent)

    def test_stale_trace_identity_no_injection(self):
        matcher=self.c.TargetMatcher(self.plan())
        self.trace.write_text(json.dumps(self.event(46))+'\n')
        with self.assertRaisesRegex(AssertionError,'pending trace identity mismatch'):
            self.c.decide_io(self.sink,self.trace,matcher,self.message(self.event()),[])
        self.assertEqual(self.sink.getvalue(),b'')

    def test_missing_target_retains_planned_and_observed_without_payload(self):
        matcher=self.c.TargetMatcher(self.plan())
        observed=self.event(44,offset=74240,length=4072,commit=False)
        msg=self.message(observed);msg['private']='PRIVATE_SENTINEL_DO_NOT_LOG'
        self.c.decide_io(self.sink,self.trace,matcher,msg,[])
        with self.assertRaisesRegex(AssertionError,'required descriptor target not reached'):
            matcher.require_reached(None)
        diagnostic=matcher.diagnostics()
        self.assertEqual(diagnostic['planned']['baseline_seq'],47)
        self.assertEqual(diagnostic['planned']['descriptor']['offset'],86552)
        self.assertEqual(diagnostic['last_observed'][-1]['seq'],44)
        self.assertEqual(diagnostic['last_observed'][-1]['offset'],74240)
        self.assertNotIn('PRIVATE_SENTINEL',json.dumps(diagnostic))
        self.assertNotIn('hex',json.dumps(diagnostic))

    def test_identical_descriptors_use_explicit_nth_group_position(self):
        first=self.event(4,op='sync',offset=0,length=0,flags=2)
        last=dict(first,seq=49)
        matcher=self.c.TargetMatcher(self.plan([first,last],1))
        self.deliver(matcher,dict(first,seq=20))
        self.deliver(matcher,dict(last,seq=80))
        self.assertEqual(self.sink.getvalue(),b'c 20\ni 80\n')
        self.assertEqual(matcher.seen,2)

    def test_duplicate_decision_after_selection_fails(self):
        matcher=self.c.TargetMatcher(self.plan());self.deliver(matcher,self.event())
        with self.assertRaisesRegex(AssertionError,'repeated target decision'):
            self.deliver(matcher,self.event(48))
        self.assertEqual(self.sink.getvalue(),b'i 47\n')

    def test_nonincreasing_live_sequence_fails(self):
        matcher=self.c.TargetMatcher(self.plan())
        self.deliver(matcher,self.event(47,phase='open'))
        with self.assertRaisesRegex(AssertionError,'decision sequence not increasing'):
            self.deliver(matcher,self.event(47))
        self.assertEqual(self.sink.getvalue(),b'c 47\n')

    def test_first_middle_last_selection_preserves_all_positions(self):
        frames=[self.event(i+10,offset=page_offset(i),length=4096,commit=False) for i in range(5)]
        sync=self.event(20,op='sync',offset=0,length=0,flags=2)
        targets=self.c.validation_bucket_targets(frames+[sync,self.event(21)],'noop')
        frame_plans=[p for _,p in targets if p['descriptor']['meaning']=='wal-page-data']
        self.assertEqual([p['baseline_seq'] for p in frame_plans],[10,12,14])
        self.assertEqual([p['samples'] for p in frame_plans],[['first'],['middle'],['last']])
        self.assertEqual([p['group_position'] for p in frame_plans],[1,3,5])
        self.assertTrue(all(p['group_count']==5 for p in frame_plans))

    def test_single_member_group_retains_all_sample_labels(self):
        targets=self.c.validation_bucket_targets([self.event(4,op='sync',offset=0,length=0),self.event()], 'noop')
        self.assertTrue(all(p['samples']==['first','middle','last'] for _,p in targets))

    def test_required_commit_group_absent_fails_discovery(self):
        with self.assertRaisesRegex(AssertionError,'commit marker not identified'):
            self.c.validation_bucket_targets([self.event(4,op='sync',offset=0,length=0)],'noop')

    def test_extra_frame_header_does_not_disturb_page_write_group(self):
        items=[self.event(10,length=4096),self.event(20,length=4096)]
        matcher=self.c.TargetMatcher(self.plan(items,1))
        for e in (self.event(31,length=24,commit=False),self.event(32,length=4096),
                  self.event(33,length=24,commit=False),self.event(34,length=4096)):
            self.deliver(matcher,e)
        self.assertEqual(self.sink.getvalue(),b'c 31\nc 32\nc 33\ni 34\n')
        self.assertEqual(matcher.seen,2)
        self.assertEqual(matcher.diagnostics()['alternate_buckets'][0]['count'],2)

    def test_extra_page_data_does_not_disturb_commit_marker_group(self):
        matcher=self.c.TargetMatcher(self.plan([self.event(10),self.event(20)],1))
        for e in (self.event(31,length=4096),self.event(32),
                  self.event(33,length=4096),self.event(34)):
            self.deliver(matcher,e)
        self.assertEqual(self.sink.getvalue(),b'c 31\nc 32\nc 33\ni 34\n')
        self.assertEqual(matcher.seen,2)

    def test_group_key_is_exactly_the_coverage_bucket_policy(self):
        items=[self.event(),self.event(offset=98912),self.event(length=4096),
               self.event(flags=1),self.event(offset=0,length=32),
               self.event(name='store.db'),self.event(name='store.db-journal'),
               self.event(op='truncate',offset=0,length=0),
               self.event(op='sync',offset=0,length=0,flags=2)]
        for e in items:
            d=self.c.event_descriptor(e)
            self.assertEqual(dict(self.c.target_group(d)),self.c.coverage_bucket(d))
        self.assertEqual(self.c.target_group(self.message(items[0])),self.c.target_group(self.message(items[1])))

    def test_discovery_coalesces_write_offsets_and_preserves_sample_labels(self):
        # Two physical offsets for database/journal writes coalesce into ten members.
        shapes=[dict(length=24,commit=False),dict(length=4096),dict(length=4096,flags=1),
                dict(name='store.db',offset=0,length=4096),dict(name='store.db',offset=4096,length=4096),
                dict(name='store.db-journal',offset=0),dict(name='store.db-journal',offset=24),
                dict(op='truncate',offset=0,length=0),dict(op='truncate',offset=4096,length=0),
                dict(offset=0,length=32),dict(op='sync',offset=0,length=0,flags=2),
                dict(op='sync',offset=0,length=0,flags=3),dict(length=24,commit=True)]
        events=[self.event(1+i*5+j,**shape) for i,shape in enumerate(shapes) for j in range(5)]
        groups={}
        for key,plan in self.c.validation_bucket_targets(events,'noop'):
            self.assertEqual(key,self.c.target_group(plan['descriptor']))
            groups.setdefault(key,[]).append(plan)
        self.assertEqual(len(groups),len(shapes)-2)
        for plans in groups.values():
            count=10 if plans[0]['descriptor']['role'] in ('database','journal') and plans[0]['descriptor']['op']=='write' else 5
            self.assertEqual([p['group_position'] for p in plans],[1,count//2+1,count])
            self.assertEqual([p['samples'] for p in plans],[['first'],['middle'],['last']])
            self.assertTrue(all(p['group_count']==count for p in plans))

    def test_sync_offset_length_and_flags_each_remain_exact(self):
        baseline=self.event(op='sync',offset=0,length=0,flags=2)
        matcher=self.c.TargetMatcher(self.plan([baseline]))
        for seq,change in enumerate(({'offset':1},{'length':1},{'flags':3}),start=1):
            self.deliver(matcher,dict(baseline,seq=seq,**change))
        self.assertEqual(matcher.seen,0)
        self.assertEqual(self.sink.getvalue(),b'c 1\nc 2\nc 3\n')
        self.deliver(matcher,dict(baseline,seq=4))
        self.assertEqual(self.sink.getvalue(),b'c 1\nc 2\nc 3\ni 4\n')

    def test_planned_group_selector_cannot_disagree_with_descriptor(self):
        plan=self.plan();plan['selector']['flags']=1
        with self.assertRaisesRegex(AssertionError,'invalid target group plan'):
            self.c.TargetMatcher(plan)
        self.assertEqual(self.sink.getvalue(),b'')

    def test_private_trace_length_and_flags_must_match_live_before_injection(self):
        for change in ({'length':2048},{'flags':1}):
            with self.subTest(change=change):
                self.sink=io.BytesIO();live=self.event(91,length=4096)
                matcher=self.c.TargetMatcher(self.plan([live]))
                self.trace.write_text(json.dumps(self.event(91,length=4096 if 'length' not in change else 2048,
                                                           flags=change.get('flags',0)))+'\n')
                with self.assertRaisesRegex(AssertionError,'pending trace descriptor mismatch'):
                    self.c.decide_io(self.sink,self.trace,matcher,self.message(live),[])
                self.assertEqual(self.sink.getvalue(),b'')
                self.assertFalse(matcher.trace_verified)
                self.assertFalse(matcher.decision_sent)

    def test_missing_target_diagnostics_report_selectors_and_bounded_counts(self):
        plan=self.plan([self.event(10,length=4096),self.event(20,length=4096)],1)
        matcher=self.c.TargetMatcher(plan)
        self.deliver(matcher,self.event(31,length=4096))
        for seq in (32,33):self.deliver(matcher,self.event(seq,commit=False))
        for seq in range(34,46):
            event=self.event(seq,length=100+seq,flags=seq,commit=False)
            msg=self.message(event);msg['secret']='PRIVATE_SENTINEL_DO_NOT_LOG'
            self.c.decide_io(self.sink,self.trace,matcher,msg,[])
        with self.assertRaisesRegex(AssertionError,'required descriptor target not reached'):
            matcher.require_reached(None)
        d=matcher.diagnostics();wire=json.dumps(d)
        self.assertEqual(d['planned']['selector']['length'],4096)
        self.assertEqual(d['planned']['group_position'],2)
        self.assertEqual(d['planned']['group_count'],2)
        self.assertEqual(d['planned']['samples'],['middle','last'])
        self.assertEqual(d['planned']['baseline_seq'],20)
        self.assertEqual(d['planned']['descriptor'],self.c.event_descriptor(self.event(20,length=4096)))
        self.assertEqual(d['matching_live_group_members'],1)
        self.assertEqual(d['relevant_alternate_events'],14)
        self.assertEqual(len(d['alternate_buckets']),8)
        self.assertEqual(d['alternate_buckets'][0]['coverage_bucket']['length'],24)
        self.assertEqual(d['alternate_buckets'][0]['count'],2)
        self.assertEqual(d['unlisted_alternate_events'],5)
        self.assertEqual(len(d['last_observed']),8)
        self.assertEqual(d['last_observed'][-1]['seq'],45)
        self.assertFalse(d['trace_verified_before_decision'])
        self.assertFalse(d['decision_sent']);self.assertFalse(d['target_reached'])
        self.assertEqual(d['acknowledged_before_fault'],0)
        self.assertNotIn('PRIVATE_SENTINEL',wire);self.assertNotIn('hex',wire)
        self.assertNotIn('prefix_sha256',wire)
        self.assertLess(len(wire),12000)

    def test_reached_flag_requires_verified_decision_and_exact_live_sequence(self):
        matcher=self.c.TargetMatcher(self.plan());self.deliver(matcher,self.event(91))
        with self.assertRaisesRegex(AssertionError,'required descriptor target not reached'):
            matcher.require_reached({'seq':90})
        self.assertFalse(matcher.diagnostics()['target_reached'])
        matcher.require_reached({'seq':91})
        self.assertTrue(matcher.diagnostics()['target_reached'])

    def test_matrix_order_keeps_every_request_unchanged(self):
        requests=[{'Operation':op,'sentinel':object()} for op in self.c.REQUIRED_OPERATIONS]
        ordered=self.c.matrix_requests(requests)
        self.assertEqual([r['Operation'] for r in ordered],
            ['checkpoint','autocheckpoint','restore','migration','fresh','empty-m1',
             'noop','create','patch','metadata','archive','unarchive'])
        self.assertEqual({id(r) for r in ordered},{id(r) for r in requests})
        self.assertEqual([r['Operation'] for r in requests],list(self.c.REQUIRED_OPERATIONS))

    def test_matrix_order_rejects_missing_or_duplicate_families(self):
        requests=[{'Operation':op} for op in self.c.REQUIRED_OPERATIONS]
        for changed in (requests[:-1],requests+[requests[0]],requests[:-1]+[requests[0]]):
            with self.subTest(count=len(changed)):
                with self.assertRaisesRegex(AssertionError,'matrix operation families missing or duplicated'):
                    self.c.matrix_requests(changed)

    def test_complete_response_is_fsynced_before_ack_can_arm_fault(self):
        acknowledgements=[];order=[]
        response={'kind':'response','ok':True,'result_text':'{"id":"synthetic"}'}
        class Ledger(io.BytesIO):
            def flush(inner):order.append('flush');super().flush()
            def fileno(inner):return 123
        def synced(fd):
            self.assertEqual(fd,123);self.assertEqual(acknowledgements,[]);order.append('sync')
        ledger=Ledger();self.addCleanup(ledger.close)
        with mock.patch.object(self.c.os,'fsync',side_effect=synced):
            self.c.record_acknowledgement(ledger,acknowledgements,'synthetic',{},response)
        matcher=self.c.TargetMatcher(self.plan())
        self.deliver(matcher,self.event(),acknowledgements)
        self.assertEqual(order,['flush','sync'])
        self.assertEqual(matcher.ack_before_fault,1)

    def test_failed_ledger_sync_does_not_create_acknowledgement(self):
        acknowledgements=[]
        ledger=mock.Mock();ledger.fileno.return_value=123
        with mock.patch.object(self.c.os,'fsync',side_effect=OSError('synthetic failure')):
            with self.assertRaises(OSError):
                self.c.record_acknowledgement(ledger,acknowledgements,'synthetic',{},
                    {'kind':'response','ok':True,'result_text':'{}'})
        self.assertEqual(acknowledgements,[])
        self.assertEqual(self.sink.getvalue(),b'')

    def test_incomplete_response_cannot_be_ledger_acknowledgement(self):
        acknowledgements=[];ledger=mock.Mock()
        with self.assertRaises(json.JSONDecodeError):
            self.c.record_acknowledgement(ledger,acknowledgements,'synthetic',{},
                {'kind':'response','ok':True,'result_text':'{'})
        ledger.write.assert_not_called();self.assertEqual(acknowledgements,[])

    def test_commit_marker_is_not_an_acknowledgement(self):
        matcher=self.c.TargetMatcher(self.plan());self.deliver(matcher,self.event(),[])
        self.assertEqual(matcher.ack_before_fault,0)

    def recovered(self):
        return [dict(schedule=s,seed=n,outcome='present') for s,n in self.c.RECOVERY_SCHEDULES]

    def representative_events(self, operation, db_count=5):
        events=[]
        def add(**fields):
            event=self.event(len(events)+1,**fields)
            event.update(rc=0,applied=event['length'] if event['op']=='write' else 1)
            events.append(event)
        add(phase='open',offset=0,length=32,commit=False)
        add(phase='open',op='sync',offset=0,length=0,flags=2)
        def transaction(phase, reset=False):
            if reset:add(phase=phase,offset=0,length=32,commit=False)
            add(phase=phase,offset=frame_offset(1),length=24,commit=False)
            add(phase=phase,offset=page_offset(1),length=4096,commit=False)
            add(phase=phase,offset=frame_offset(2),length=24)
            add(phase=phase,offset=page_offset(2),length=4096)
            add(phase=phase,op='sync',offset=0,length=0,flags=2)
        def checkpoint(phase):
            for i in range(db_count):add(phase=phase,name='store.db',offset=i*4096,length=4096)
            add(phase=phase,name='store.db',op='sync',offset=0,length=0,flags=2)
        if operation=='checkpoint':
            transaction('checkpoint-create-000');checkpoint('checkpoint')
            add(phase='checkpoint',op='truncate',offset=0,length=0)
        elif operation=='autocheckpoint':
            transaction('autocheckpoint-create-000')
            transaction('autocheckpoint-create-001');checkpoint('autocheckpoint-create-001')
            transaction('autocheckpoint-create-002',reset=True)
        else:
            phase=operation if operation in ('migration','fresh','empty-m1') else 'mutation-'+operation
            transaction(phase)
        return events

    def row_plans(self, operation, db_count=5):
        return self.c.select_representative_targets(self.representative_events(operation,db_count),operation)

    def coverage_fixture(self, acknowledged=True):
        # Fresh synthetic IDs only. This is not application/durability evidence.
        coverage=self.c.AcceptanceCoverage()
        for op in self.c.REQUIRED_OPERATIONS:
            for sector in (512,4096):
                plans=self.row_plans(op);coverage.plan(op,sector,plans)
                for _,plan in plans:
                    for fault in self.c.fault_modes(plan):
                        coverage.complete(op,sector,plan,fault,self.recovered(),int(acknowledged))
        return coverage

    def test_no_executed_families_cannot_pass(self):
        with self.assertRaisesRegex(AssertionError,'required operation/sector family absent'):
            self.c.AcceptanceCoverage().require_complete()

    def test_each_missing_required_family_or_sector_cannot_pass(self):
        for op in self.c.REQUIRED_OPERATIONS:
            for sector in (512,4096):
                with self.subTest(operation=op,sector=sector):
                    coverage=self.coverage_fixture();del coverage.planned[(op,sector)]
                    with self.assertRaisesRegex(AssertionError,'required operation/sector family absent'):
                        coverage.require_complete()

    def test_missing_required_target_combination_cannot_pass(self):
        coverage=self.coverage_fixture();coverage.completed.pop()
        with self.assertRaisesRegex(AssertionError,'required target/fault cases incomplete'):
            coverage.require_complete()

    def test_zero_acknowledged_category_coverage_cannot_pass(self):
        coverage=self.coverage_fixture();coverage.post_ack.clear()
        with self.assertRaisesRegex(AssertionError,'new acknowledged-effect recovery coverage missing'):
            coverage.require_complete()

    def test_every_checkpoint_case_needs_an_acknowledgement(self):
        for op in ('checkpoint','autocheckpoint'):
            plans=self.row_plans(op)
            for _,plan in plans:
                for mode in self.c.fault_modes(plan):
                    coverage=self.c.AcceptanceCoverage();coverage.plan(op,512,plans)
                    with self.assertRaisesRegex(AssertionError,'lacks pre-fault acknowledgement'):
                        coverage.complete(op,512,plan,mode,self.recovered(),0)
                    self.assertEqual(coverage.completed,set())

    def test_incomplete_or_extra_recovery_schedules_cannot_complete_case(self):
        plans=self.row_plans('noop');plan=plans[0][1]
        variants=[self.recovered()[:-1],self.recovered()+[dict(schedule='reorder-torn',seed=29,outcome='present')],
                  self.recovered()[::-1]]
        for recovered in variants:
            coverage=self.c.AcceptanceCoverage();coverage.plan('noop',512,plans)
            with self.assertRaisesRegex(AssertionError,'incomplete recovery schedules'):
                coverage.complete('noop',512,plan,'partial',recovered,0)
            self.assertEqual(coverage.completed,set())

    def test_complete_fresh_synthetic_ids_are_accepted(self):
        coverage=self.coverage_fixture();coverage.require_complete()
        self.assertEqual(coverage.completed,self.c.expected_case_ids())
        self.assertEqual(len(coverage.completed),78)
        self.assertEqual(len(coverage.post_ack & self.c.required_acknowledgements()),12)

    def test_duplicate_completed_id_and_unassigned_mode_are_rejected(self):
        plans=self.row_plans('noop');plan=plans[0][1]
        coverage=self.c.AcceptanceCoverage();coverage.plan('noop',512,plans)
        coverage.complete('noop',512,plan,'partial',self.recovered(),0)
        with self.assertRaisesRegex(AssertionError,'duplicate completed case'):
            coverage.complete('noop',512,plan,'partial',self.recovered(),0)
        with self.assertRaisesRegex(AssertionError,'unplanned completed case'):
            coverage.complete('noop',512,plan,'full',self.recovered(),0)

    def test_historical_sequence_based_ids_cannot_fill_representative_gate(self):
        coverage=self.coverage_fixture()
        identity=coverage.completed.pop()
        coverage.completed.add(('checkpoint',512,1477,'partial'))
        self.assertNotIn(identity,coverage.completed)
        with self.assertRaisesRegex(AssertionError,'required target/fault cases incomplete'):
            coverage.require_complete()

    def test_1477_database_offsets_form_one_bucket_and_three_samples(self):
        writes=[self.event(i+10,name='store.db',phase='checkpoint',offset=i*4096,length=4096) for i in range(1477)]
        events=[self.event(1),self.event(2,op='sync',offset=0,length=0)]+writes
        targets=self.c.validation_bucket_targets(events,'noop')
        sampled=[p for _,p in targets if p['descriptor']['role']=='database']
        self.assertEqual(len({self.c.target_group(self.message(e)) for e in writes}),1)
        self.assertEqual([p['group_position'] for p in sampled],[1,739,1477])
        self.assertEqual([p['samples'] for p in sampled],[['first'],['middle'],['last']])
        self.assertEqual([p['descriptor']['offset'] for p in sampled],[0,738*4096,1476*4096])
        self.assertTrue(all(p['group_count']==1477 for p in sampled))
        self.assertTrue(all('offset' not in p['coverage_bucket'] for p in sampled))

    def test_database_live_offset_is_evidence_not_baseline_address_search(self):
        base=self.event(8,name='store.db',phase='checkpoint',offset=4096,length=4096)
        live=dict(base,seq=91,offset=32768);plan=self.plan([base])
        matcher=self.c.TargetMatcher(plan);self.deliver(matcher,live)
        evidence=self.c.target_evidence(plan,live)
        self.assertEqual(self.sink.getvalue(),b'i 91\n')
        self.assertEqual(evidence['planned']['selector']['offset'],4096)
        self.assertEqual(evidence['observed_selector']['offset'],32768)
        self.assertEqual(evidence['observed']['offset'],32768)
        self.assertEqual(evidence['geometry_changed'],['offset'])

    def test_database_private_trace_must_match_exact_live_offset(self):
        base=self.event(8,name='store.db',phase='checkpoint',offset=4096,length=4096)
        live=dict(base,seq=91,offset=32768);matcher=self.c.TargetMatcher(self.plan([base]))
        self.trace.write_text(json.dumps(dict(base,seq=91))+'\n')
        with self.assertRaisesRegex(AssertionError,'pending trace descriptor mismatch'):
            self.c.decide_io(self.sink,self.trace,matcher,self.message(live),[])
        self.assertFalse(matcher.trace_verified);self.assertFalse(matcher.decision_sent)
        self.assertEqual(self.sink.getvalue(),b'')

    def test_database_lengths_and_flags_stay_separate_buckets(self):
        base=self.event(name='store.db',offset=0,length=4096)
        keys=[self.c.target_group(self.message(e)) for e in
              (base,dict(base,offset=8192),self.event(name='store.db',offset=0,length=2048),dict(base,flags=1))]
        self.assertEqual(keys[0],keys[1]);self.assertEqual(len(set(keys)),3)

    def test_journal_offsets_coalesce_but_exact_live_trace_is_required(self):
        base=self.event(name='store.db-journal',offset=0,length=24)
        live=dict(base,seq=90,offset=512);plan=self.plan([base])
        self.assertEqual(self.c.coverage_bucket(self.message(base)),self.c.coverage_bucket(self.message(live)))
        matcher=self.c.TargetMatcher(plan);self.trace.write_text(json.dumps(dict(base,seq=90))+'\n')
        with self.assertRaisesRegex(AssertionError,'pending trace descriptor mismatch'):
            self.c.decide_io(self.sink,self.trace,matcher,self.message(live),[])
        self.assertEqual(self.sink.getvalue(),b'')

    def test_wal_full_page_fragment_frame_commit_and_reset_are_distinct(self):
        events=[self.event(offset=0,length=32),self.event(commit=False),self.event(),
                self.event(length=4096),self.event(length=2048),
                self.event(offset=frame_offset(),length=12),self.event(offset=0,length=16),
                self.event(offset=frame_offset(),length=4097)]
        buckets=[self.c.coverage_bucket(self.message(e)) for e in events]
        self.assertEqual([b['shape'] for b in buckets],['wal-header-reset','wal-frame','wal-commit-marker',
            'wal-page-full','wal-page-fragment','wal-frame-fragment','wal-header-fragment','wal-raw'])
        self.assertEqual(len({tuple(sorted(b.items())) for b in buckets}),8)
        unknown=dict(self.event(),wal_header='')
        self.assertEqual(self.c.coverage_bucket(self.message(unknown))['shape'],'wal-unknown')

    def test_many_page_fragment_lengths_form_one_bucket(self):
        fragments=[self.event(i+10,offset=page_offset(i),length=i+1) for i in range(1000)]
        targets=self.c.validation_bucket_targets([self.event(1),self.event(2,op='sync',offset=0,length=0)]+fragments,'noop')
        plans=[p for _,p in targets if p['coverage_bucket']['shape']=='wal-page-fragment']
        self.assertEqual(len(plans),3)
        self.assertEqual([p['group_position'] for p in plans],[1,501,1000])
        self.assertEqual([p['descriptor']['length'] for p in plans],[1,501,1000])
        self.assertTrue(all('length' not in p['coverage_bucket'] for p in plans))

    def test_many_frame_fragment_lengths_form_one_bucket(self):
        events=[self.event(i,offset=frame_offset(i),length=i) for i in range(1,24)]
        plan=self.plan(events,11)
        self.assertEqual(plan['group_position'],12);self.assertEqual(plan['group_count'],23)
        self.assertEqual(len({self.c.target_group(self.message(e)) for e in events}),1)

    def test_fragment_live_length_and_offset_are_recorded_after_exact_verification(self):
        base=self.event(1,offset=page_offset(),length=512)
        live=self.event(9,offset=page_offset(3)+100,length=73)
        plan=self.plan([base]);matcher=self.c.TargetMatcher(plan);self.deliver(matcher,live)
        evidence=self.c.target_evidence(plan,live)
        self.assertEqual(self.sink.getvalue(),b'i 9\n')
        self.assertEqual(evidence['geometry_changed'],['offset','length'])
        self.assertEqual(evidence['observed']['length'],73)
        self.assertEqual(evidence['planned']['descriptor']['length'],512)

    def test_fragment_private_length_mismatch_cannot_inject(self):
        base=self.event(length=512);live=self.event(90,length=73)
        matcher=self.c.TargetMatcher(self.plan([base]));self.trace.write_text(json.dumps(self.event(90,length=74))+'\n')
        with self.assertRaisesRegex(AssertionError,'pending trace descriptor mismatch'):
            self.c.decide_io(self.sink,self.trace,matcher,self.message(live),[])
        self.assertEqual(self.sink.getvalue(),b'');self.assertFalse(matcher.decision_sent)

    def test_fragment_page_size_and_flags_remain_distinct(self):
        events=[self.event(length=80),self.event(length=90),self.event(length=80,flags=1),
                self.event(length=80,page_size=1024)]
        keys=[self.c.target_group(self.message(e)) for e in events]
        self.assertEqual(keys[0],keys[1]);self.assertEqual(len(set(keys)),3)

    def test_raw_unknown_fragments_and_page_data_cannot_satisfy_commit_target(self):
        matcher=self.c.TargetMatcher(self.plan())
        events=[self.event(1,length=4096),self.event(2,length=12,offset=frame_offset()),
                self.event(3,length=4097,offset=frame_offset()),dict(self.event(4),wal_header='')]
        for event in events:self.deliver(matcher,event)
        self.assertEqual(matcher.seen,0);self.assertNotIn(b'i ',self.sink.getvalue())
        with self.assertRaisesRegex(AssertionError,'required descriptor target not reached'):matcher.require_reached(None)
        with self.assertRaisesRegex(AssertionError,'commit marker not identified'):
            self.c.validation_bucket_targets(events+[self.event(5,op='sync',offset=0,length=0)],'noop')

    def test_nth_database_member_ignores_other_buckets_and_missing_n_fails(self):
        a=self.event(1,name='store.db',length=4096,offset=0)
        plan=self.plan([a,dict(a,seq=2,offset=4096),dict(a,seq=3,offset=8192)],2)
        matcher=self.c.TargetMatcher(plan)
        for event in (dict(a,seq=10,offset=12288),self.event(11),dict(a,seq=12,offset=16384)):
            self.deliver(matcher,event)
        self.assertEqual(matcher.seen,2);self.assertNotIn(b'i ',self.sink.getvalue())
        with self.assertRaisesRegex(AssertionError,'required descriptor target not reached'):matcher.require_reached(None)
        self.deliver(matcher,dict(a,seq=13,offset=20480))
        self.assertEqual(self.sink.getvalue(),b'c 10\nc 11\nc 12\ni 13\n')

    def test_plan_bucket_tampering_cannot_authorize_injection(self):
        plan=self.plan();plan['coverage_bucket']['shape']='wal-page-fragment'
        with self.assertRaisesRegex(AssertionError,'invalid target group plan'):self.c.TargetMatcher(plan)
        self.assertEqual(self.sink.getvalue(),b'')

    def planning_fixture(self, db_count=5, history=None):
        requests=[{'Operation':op} for op in self.c.REQUIRED_OPERATIONS]
        def discover(request,sector):
            if history is not None:history.append((request['Operation'],sector))
            return dict(targets=self.row_plans(request['Operation'],db_count),seedimage={},seedstate={})
        return requests,discover

    def test_full_plan_all_24_discoveries_precede_first_application_case(self):
        history=[];requests,discover=self.planning_fixture(history=history)
        coverage=self.c.AcceptanceCoverage();iterator=self.c.application_cases(requests,discover,coverage)
        self.assertEqual(history,[]);first=next(iterator)
        self.assertEqual(history,[(op,s) for op in self.c.MATRIX_ORDER for s in (512,4096)])
        self.assertEqual(len(coverage.planned),24)
        self.assertEqual(first[0]['operation'],'checkpoint');self.assertEqual(first[1]['row_id'],'K1')
        self.assertEqual([call.kwargs['kind'] for call in self.log.call_args_list],
                         ['application-plan','application-plan-gate','schedule'])

    def test_late_discovery_failure_yields_no_application_fault_case(self):
        history=[];requests,discover=self.planning_fixture(history=history);injections=[]
        def failing(request,sector):
            if request['Operation']=='unarchive' and sector==4096:raise TimeoutError('synthetic')
            return discover(request,sector)
        coverage=self.c.AcceptanceCoverage()
        with self.assertRaises(TimeoutError):
            for item in self.c.application_cases(requests,failing,coverage):injections.append(item)
        self.assertEqual(len(history),23);self.assertEqual(injections,[]);self.assertEqual(coverage.planned,{})
        self.log.assert_not_called()

    def test_exact_approved_assignments_not_every_compatible_mode(self):
        requests,discover=self.planning_fixture()
        cases=list(self.c.application_cases(requests,discover,self.c.AcceptanceCoverage()))
        expected={
            'C1':('cut-before','cut-after','ioerr','full','partial'),'C2':('cut-before','cut-after','ioerr'),
            'P1':('partial',),'P2':('ioerr',),'R1':('cut-before','cut-after','partial'),'R2':('ioerr',),
            'U1':('cut-before',),'U2':('cut-before','cut-after','full','partial'),'U3':('ioerr',),
            'V1':('ioerr',),'V2':('ioerr',),'V3':('ioerr',),'V4':('partial',),
            'I1':('partial',),'I2':('ioerr',),'I3':('full',),'I4':('ioerr',),
            'K1':('partial',),'K2':('ioerr',),'K3':('cut-before','cut-after','ioerr'),
            'K4':('cut-before','cut-after','ioerr'),'A1':('full',),'A2':('ioerr',),'A3':('partial',)}
        self.assertEqual({row:modes for row,_,_,modes in self.c.REPRESENTATIVE_ROWS},expected)
        ids={(self.c.TARGET_POLICY,t['row_id'],b['sector'],mode) for b,t,mode in cases}
        self.assertEqual(ids,self.c.expected_case_ids());self.assertEqual(len(cases),78)
        summary=next(c.kwargs for c in self.log.call_args_list if c.kwargs['kind']=='application-plan')
        self.assertEqual((summary['targets'],summary['fault_cases'],summary['recoveries']),(48,78,234))
        self.assertEqual(summary['recovery_schedules'],[['discard',17],['retain',17],['reorder-torn',17]])
        self.assertEqual(len(summary['required_acknowledgements']),12)
        self.assertEqual(sum(f['fault_cases'] for f in summary['families']),78)
        self.assertEqual(sum(r['recoveries'] for r in summary['rows']),234)

    def test_full_plan_missing_sector_or_family_never_passes_preflight(self):
        requests,discover=self.planning_fixture();plans=self.c.compile_application_plans(requests,discover)
        for index in range(24):
            with self.assertRaisesRegex(AssertionError,'full plan families/sectors'):
                self.c.preflight_application_plans(plans[:index]+plans[index+1:])
        self.log.assert_not_called()

    def test_extra_missing_or_duplicate_row_never_passes_preflight(self):
        for alteration in ('extra','missing','duplicate'):
            requests,discover=self.planning_fixture();plans=self.c.compile_application_plans(requests,discover)
            targets=plans[0]['targets']
            if alteration=='missing':targets.pop()
            else:targets.append(targets[-1])
            with self.assertRaisesRegex(AssertionError,'family rows missing or duplicated'):
                self.c.preflight_application_plans(plans)
        self.log.assert_not_called()

    def test_unapproved_fault_expansion_is_rejected_before_injection(self):
        requests,discover=self.planning_fixture();plans=self.c.compile_application_plans(requests,discover)
        plans[0]['targets'][0][1]['modes'].append('full')
        with self.assertRaisesRegex(AssertionError,'unapproved representative row or fault assignment'):
            self.c.preflight_application_plans(plans)
        self.log.assert_not_called()

    def test_many_database_pages_do_not_expand_approved_case_set(self):
        requests,discover=self.planning_fixture(1477)
        summary=self.c.preflight_application_plans(self.c.compile_application_plans(requests,discover))
        self.assertEqual((summary['fault_cases'],summary['recoveries']),(78,234))
        self.assertEqual(len(summary['rows']),48)
        selected={r['row_id']:r['target'] for r in summary['rows'] if r['sector']==512}
        self.assertEqual(selected['K1']['group_position'],1)
        self.assertEqual(selected['K2']['group_position'],1477)
        self.assertEqual(selected['A1']['group_position'],739)

    def test_small_checkpoint_pass_fails_without_substitution(self):
        for operation,count in (('checkpoint',1),('autocheckpoint',2)):
            with self.assertRaisesRegex(AssertionError,'checkpoint pass too small'):
                self.row_plans(operation,count)

    def test_committing_sync_requires_complete_payload_not_startup_sync(self):
        events=self.representative_events('create')
        targets=dict(self.c.select_representative_targets(events,'create'))
        self.assertEqual(targets['C2']['baseline_seq'],events[-1]['seq'])
        # A committed header followed by half a page and a sync is insufficient.
        events[-2].update(length=2048,hex=bytes(2048).hex(),applied=2048)
        with self.assertRaisesRegex(AssertionError,'committing WAL sync not observed'):
            self.c.select_representative_targets(events,'create')

    def test_commit_fragment_cannot_replace_complete_header(self):
        events=self.representative_events('noop');header=events[-3]
        header['length']=12;header['hex']=header['hex'][:24];header['applied']=12
        with self.assertRaisesRegex(AssertionError,'commit marker not identified'):
            self.c.select_representative_targets(events,'noop')

    def test_upgrade_precommit_page_is_bound_to_noncommit_frame(self):
        plans=dict(self.row_plans('migration'));target=plans['U1']
        self.assertLess(target['baseline_seq'],plans['U2']['baseline_seq'])
        self.assertEqual(target['binding']['boundary'],'precommit-page')
        self.assertEqual(target['descriptor']['meaning'],'wal-page-data')
        events=self.representative_events('migration')
        events[2]['hex']=frame_header(4096,commit=True).hex()
        with self.assertRaisesRegex(AssertionError,'representative boundary missing'):
            self.c.select_representative_targets(events,'migration')

    def test_auto_reset_is_after_completed_pass_and_in_subsequent_create(self):
        targets=dict(self.row_plans('autocheckpoint'))
        reset=targets['A3'];sync=targets['A2']
        self.assertEqual(reset['binding']['checkpoint_phase'],sync['binding']['phase'])
        self.assertGreater(reset['binding']['phase'],sync['binding']['phase'])
        events=self.representative_events('autocheckpoint')
        events=[e for e in events if not (e['role']=='database' and e['op']=='sync')]
        with self.assertRaisesRegex(AssertionError,'automatic checkpoint pass missing'):
            self.c.select_representative_targets(events,'autocheckpoint')

    def test_multiple_database_passes_in_one_phase_fail_as_ambiguous(self):
        events=self.representative_events('checkpoint')
        events.append(dict(events[-3],seq=len(events)+1))
        with self.assertRaisesRegex(AssertionError,'multiple checkpoint passes'):
            self.c.select_representative_targets(events,'checkpoint')

    def test_representative_matcher_binds_phase_generation_and_private_prefix(self):
        events=self.representative_events('create');target=dict(self.row_plans('create'))['C2']
        matcher=self.c.RepresentativeMatcher(target);trace=[]
        for event in events:
            pre={k:v for k,v in event.items() if k not in ('rc','applied')};trace.append(pre)
            self.trace.write_text(''.join(json.dumps(e)+'\n' for e in trace))
            self.c.decide_io(self.sink,self.trace,matcher,self.message(event),[])
            trace.append(dict(seq=event['seq'],stage='post',rc=event['rc'],applied=event['applied']))
        self.assertTrue(matcher.trace_verified);self.assertTrue(matcher.decision_sent)
        self.assertEqual(matcher.selected_seq,events[-1]['seq'])
        matcher.require_reached({'seq':events[-1]['seq']})

    def test_representative_failed_prefix_cannot_authorize_selected_fault(self):
        events=self.representative_events('create');target=dict(self.row_plans('create'))['C2']
        matcher=self.c.RepresentativeMatcher(target);trace=[]
        for event in events[:-1]:
            pre={k:v for k,v in event.items() if k not in ('rc','applied')}
            matcher.observe(self.message(event));trace += [pre,dict(seq=event['seq'],stage='post',rc=0,applied=event['applied'])]
        trace[1]['rc']=10
        trace.append({k:v for k,v in events[-1].items() if k not in ('rc','applied')})
        self.trace.write_text(''.join(json.dumps(e)+'\n' for e in trace))
        with self.assertRaisesRegex(AssertionError,'pre-fault I/O did not complete successfully'):
            self.c.decide_io(self.sink,self.trace,matcher,self.message(events[-1]),[])
        self.assertEqual(self.sink.getvalue(),b'');self.assertFalse(matcher.decision_sent)

    def test_live_wal_generation_drift_is_rejected(self):
        events=self.representative_events('create');target=dict(self.row_plans('create'))['C1']
        matcher=self.c.RepresentativeMatcher(target)
        matcher.observe(self.message(events[0]))
        matcher.observe(self.message(dict(events[0],seq=2)))
        with self.assertRaisesRegex(AssertionError,'transaction/epoch drift'):
            for event in events[2:]:matcher.observe(self.message(dict(event,seq=event['seq']+1)))

    def test_missing_selected_last_write_never_falls_back_to_first(self):
        target=dict(self.row_plans('checkpoint'))['K2'];matcher=self.c.RepresentativeMatcher(target)
        events=self.representative_events('checkpoint')
        writes=[e for e in events if e['phase']=='checkpoint' and e['op']=='write']
        for event in events:
            if event['seq']>=writes[-1]['seq']:break
            matcher.observe(self.message(event))
        with self.assertRaisesRegex(AssertionError,'required descriptor target not reached'):
            matcher.require_reached(None)
        self.assertFalse(matcher.decision_sent)

    def test_fault_applied_bytes_are_checked(self):
        event=dict(self.event(length=24),rc=10,applied=12)
        self.c.verify_fault_result({},'partial',event)
        for wrong in (0,11,24):
            with self.assertRaisesRegex(AssertionError,'applied bytes mismatch'):
                self.c.verify_fault_result({},'partial',dict(event,applied=wrong))
        with self.assertRaisesRegex(AssertionError,'cut-before I/O was applied'):
            self.c.verify_fault_result({},'cut-before',event)

    def test_reorder_torn_validation_witness_rejects_discard_and_retain(self):
        self.c.validate_reorder_torn()
        controller=self.c
        for schedule in ('discard','retain'):
            class Broken(controller.Model):
                def crash(inner,mode,seed):return super().crash(schedule,seed)
            with self.assertRaisesRegex(AssertionError,'did not reorder and tear'):
                self.c.validate_reorder_torn(Broken)

    def test_later_header_or_checkpoint_sync_cannot_borrow_a_durable_commit(self):
        events=self.representative_events('create');committing_seq=events[-1]['seq']
        events.append(dict(events[-1],seq=committing_seq+1))
        target=dict(self.c.select_representative_targets(events,'create'))['C2']
        self.assertEqual(target['baseline_seq'],committing_seq)
        self.assertEqual(target['group_count'],1)

    def test_sync_halfway_through_commit_payload_is_not_committing_sync(self):
        events=self.representative_events('create');last_sync=events.pop()
        payload=events.pop();half=dict(payload,length=2048,hex=bytes(2048).hex(),applied=2048)
        events += [half,dict(last_sync,seq=payload['seq']+1),
                   dict(half,seq=payload['seq']+2,offset=payload['offset']+2048),
                   dict(last_sync,seq=payload['seq']+3)]
        target=dict(self.c.select_representative_targets(events,'create'))['C2']
        self.assertEqual(target['baseline_seq'],events[-1]['seq'])
        self.assertEqual(target['group_count'],1)

    def test_every_required_post_ack_category_is_independently_enforced(self):
        for category in self.c.required_acknowledgements():
            coverage=self.coverage_fixture();coverage.post_ack.remove(category)
            with self.assertRaisesRegex(AssertionError,'new acknowledged-effect recovery coverage missing'):
                coverage.require_complete()

    def test_missing_external_ledger_ack_prevents_checkpoint_injection_command(self):
        events=self.representative_events('checkpoint');plan=dict(self.row_plans('checkpoint'))['K1']
        matcher=self.c.RepresentativeMatcher(plan);trace=[]
        for event in events:
            pre={k:v for k,v in event.items() if k not in ('rc','applied')};trace.append(pre)
            self.trace.write_text(''.join(json.dumps(e)+'\n' for e in trace))
            if event['seq']==plan['baseline_seq']:
                with self.assertRaisesRegex(AssertionError,'lacks pre-fault acknowledgement'):
                    self.c.decide_io(self.sink,self.trace,matcher,self.message(event),[])
                break
            self.c.decide_io(self.sink,self.trace,matcher,self.message(event),[])
            trace.append(dict(seq=event['seq'],stage='post',rc=0,applied=event['applied']))
        self.assertNotIn(b'i ',self.sink.getvalue());self.assertFalse(matcher.decision_sent)

    def request_fixture(self):
        return dict(subjects=[dict(id='synthetic-subject')],
            records=[dict(id='archived-record',status='archived',revision=3),
                     dict(id='active-record',status='active',revision=3)],
            record_revisions=[dict(record_id='archived-record',data=json.dumps({'state':i}),
                key='old-key',sensitivity='private',provenance='{"source":"old"}',status='archived')
                for i in range(3)])

    def test_combined_patch_nullable_metadata_and_noop_keep_separate_requests(self):
        requests={r['Operation']:r for r in self.c.mutation_requests(self.request_fixture())}
        patch=self.c.precise(requests['patch']['Body']);metadata=json.loads(requests['metadata']['Body'])
        self.assertEqual(set(patch),{'base_revision','data','key','sensitivity','provenance','status'})
        self.assertEqual(patch['data']['n'],('number','9007199254740993'))
        self.assertEqual(patch['data']['d'],('number','0.30'))
        self.assertEqual(patch['data']['e'],('number','1e2'))
        self.assertEqual(patch['data']['z'],('number','-0'))
        self.assertNotIn('data',metadata);self.assertIsNone(metadata['key']);self.assertIsNone(metadata['provenance'])
        self.assertEqual(json.loads(requests['noop']['Body']),{'base_revision':3,'status':'archived'})
        self.assertEqual(requests['restore']['Target'],1)
        self.assertEqual(json.loads(requests['restore']['Body'])['base_revision'],3)

    def test_restore_requires_three_distinct_content_snapshots(self):
        before=self.request_fixture();before['record_revisions'][2]=dict(before['record_revisions'][1])
        with self.assertRaisesRegex(AssertionError,'snapshots are not distinct'):
            self.c.mutation_requests(before)

    def test_raw_and_unknown_lengths_are_bounded_separate_classes(self):
        raw=[self.event(i,offset=frame_offset(i),length=100+i) for i in range(1,101)]
        unknown=[dict(e,seq=e['seq']+100,wal_header='') for e in raw]
        self.assertEqual(len({self.c.target_group(self.message(e)) for e in raw+unknown}),2)
        self.assertTrue(all(self.c.coverage_bucket(self.message(e))['shape']=='wal-raw' for e in raw))
        self.assertTrue(all(self.c.coverage_bucket(self.message(e))['shape']=='wal-unknown' for e in unknown))
        self.assertTrue(all('length' not in self.c.coverage_bucket(self.message(e)) for e in raw+unknown))

    def test_main_application_fault_loop_uses_preflight_generator(self):
        tree=ast.parse(Path(self.c.__file__).read_text())
        main=next(n for n in tree.body if isinstance(n,ast.FunctionDef) and n.name=='main')
        loops=[n for n in main.body if isinstance(n,ast.For)]
        self.assertEqual(len(loops),1)
        loop=loops[0];self.assertEqual(loop.iter.func.id,'application_cases')
        fault_calls=[n for n in ast.walk(main) if isinstance(n,ast.Call) and
                     isinstance(n.func,ast.Name) and n.func.id=='execute' and
                     any(isinstance(a,ast.Name) and a.id=='target' for a in n.args)]
        self.assertEqual(len(fault_calls),1)
        self.assertIn(fault_calls[0],list(ast.walk(loop)))



if __name__=='__main__':
    unittest.main()
