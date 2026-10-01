"""Source-review-only fake-trace targeting regressions; NOT RUN.

No SQLite, worker, fixture binary, HTTP or subprocess is executed by these tests.
Ordinary CI does not run this file. Separate execution approval is required.
"""
import importlib.util
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock


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
              op='write', offset=86552, length=24, flags=0, commit=True):
        data=bytearray(length) if op=='write' else bytearray()
        if commit and len(data)==24:data[7]=1
        return dict(seq=seq,stage='pre',phase=phase,name=name,
                    role='wal' if name.endswith('-wal') else ('journal' if name.endswith('-journal') else 'database'),op=op,
                    offset=offset,length=length,flags=flags,hex=data.hex())

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
        self.deliver(matcher,self.event(25,offset=32,length=4096,commit=False))
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

    def test_changed_range_follows_explicit_file_policy(self):
        # Replaces the former blanket range rejection, preserving strict ranges
        # for database/journal writes while explicitly allowing WAL geometry.
        for name,allowed in [('store.db-wal',True),('store.db',False),('store.db-journal',False)]:
            with self.subTest(name=name):
                self.sink=io.BytesIO()
                original=self.event(name=name)
                matcher=self.c.TargetMatcher(self.plan([original]))
                changed=dict(original,offset=90000)
                if allowed:
                    self.deliver(matcher,changed)
                    self.assertEqual(self.sink.getvalue(),b'i 47\n')
                else:
                    with self.assertRaisesRegex(AssertionError,'required target group prefix changed'):
                        self.deliver(matcher,changed)
                    self.assertEqual(self.sink.getvalue(),b'')
                    self.assertFalse(matcher.decision_sent)

    def test_first_commit_marker_at_different_append_offset_is_selected(self):
        plan=self.plan([self.event(39,offset=70072),self.event(41,offset=74192),
                        self.event(42,offset=74216)])
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
        with self.assertRaisesRegex(AssertionError,'required target group prefix changed'):
            self.deliver(matcher,self.event(name='probe.db',length=4096))
        self.assertEqual(self.sink.getvalue(),b'')

    def test_incompatible_append_length_fails_before_injection(self):
        plan=self.plan([self.event(length=4096,commit=False)])
        matcher=self.c.TargetMatcher(plan)
        with self.assertRaisesRegex(AssertionError,'required target group prefix changed'):
            self.deliver(matcher,self.event(offset=90000,length=2048,commit=False))
        self.assertEqual(self.sink.getvalue(),b'')

    def test_commit_marker_requires_24_byte_shape(self):
        matcher=self.c.TargetMatcher(self.plan());message=self.message(self.event())
        message['length']=23
        with self.assertRaisesRegex(AssertionError,'invalid WAL commit-marker shape'):
            self.c.decide_io(self.sink,self.trace,matcher,message,[])
        self.assertEqual(self.sink.getvalue(),b'')

    def test_wal_header_reset_requires_offset_zero(self):
        header=self.event(offset=0,length=32,commit=False)
        matcher=self.c.TargetMatcher(self.plan([header]));message=self.message(header)
        message['offset']=32
        with self.assertRaisesRegex(AssertionError,'WAL header/append offset mismatch'):
            self.c.decide_io(self.sink,self.trace,matcher,message,[])
        self.assertEqual(self.sink.getvalue(),b'')

    def test_wal_append_cannot_claim_offset_zero(self):
        matcher=self.c.TargetMatcher(self.plan());message=self.message(self.event())
        message['offset']=0
        with self.assertRaisesRegex(AssertionError,'WAL header/append offset mismatch'):
            self.c.decide_io(self.sink,self.trace,matcher,message,[])
        self.assertEqual(self.sink.getvalue(),b'')

    def test_truncate_size_remains_identity(self):
        event=self.event(op='truncate',offset=4096,length=0)
        matcher=self.c.TargetMatcher(self.plan([event]))
        with self.assertRaisesRegex(AssertionError,'required target group prefix changed'):
            self.deliver(matcher,dict(event,offset=0))
        self.assertEqual(self.sink.getvalue(),b'')

    def test_sync_flags_remain_identity(self):
        event=self.event(op='sync',offset=0,length=0,flags=2)
        matcher=self.c.TargetMatcher(self.plan([event]))
        with self.assertRaisesRegex(AssertionError,'required target group prefix changed'):
            self.deliver(matcher,dict(event,flags=3))
        self.assertEqual(self.sink.getvalue(),b'')

    def test_append_flags_remain_identity(self):
        matcher=self.c.TargetMatcher(self.plan())
        with self.assertRaisesRegex(AssertionError,'required target group prefix changed'):
            self.deliver(matcher,self.event(offset=90000,flags=1))
        self.assertEqual(self.sink.getvalue(),b'')

    def test_reordered_group_prefix_is_not_substituted(self):
        # Stable shape, not geometry, distinguishes these prefix members.
        first=self.event(10,offset=4000,commit=False)
        last=self.event(47,length=4096,commit=False)
        matcher=self.c.TargetMatcher(self.plan([first,last],1))
        with self.assertRaisesRegex(AssertionError,'required target group prefix changed'):
            self.deliver(matcher,last)
        self.assertEqual(self.sink.getvalue(),b'')

    def test_append_prefix_accepts_geometry_but_enforces_ordered_position(self):
        items=[self.event(10,offset=4000),self.event(47)]
        matcher=self.c.TargetMatcher(self.plan(items,1))
        self.deliver(matcher,self.event(20,offset=6000))
        self.assertEqual(self.sink.getvalue(),b'c 20\n')
        self.assertIsNone(matcher.selected_seq)
        self.deliver(matcher,self.event(80,offset=90000))
        self.assertEqual(self.sink.getvalue(),b'c 20\ni 80\n')
        self.assertEqual(matcher.seen,2)
        self.assertEqual([c.kwargs['group_position'] for c in self.log.call_args_list],[1,2])
        self.assertEqual([c.kwargs['baseline']['seq'] for c in self.log.call_args_list],[10,47])

    def test_indistinguishable_append_members_resolve_only_by_group_position(self):
        plan=self.plan([self.event(10,offset=4000),self.event(47)],1)
        matcher=self.c.TargetMatcher(plan)
        # There is deliberately no hidden transaction/page identity. An extra
        # identical selector counts as a member; the explicit second one wins.
        self.deliver(matcher,self.event(11,offset=5000))
        self.deliver(matcher,self.event(12,offset=6000))
        self.assertEqual(self.sink.getvalue(),b'c 11\ni 12\n')
        self.assertEqual(matcher.selected_seq,12)

    def test_removed_prefix_shape_cannot_skip_to_selected_member(self):
        first=self.event(10,length=24,commit=False)
        middle=self.event(20,length=4096,commit=False)
        last=self.event(30,length=24,commit=False)
        matcher=self.c.TargetMatcher(self.plan([first,middle,last],2))
        self.deliver(matcher,first)
        with self.assertRaisesRegex(AssertionError,'required target group prefix changed'):
            self.deliver(matcher,last)
        self.assertEqual(self.sink.getvalue(),b'c 10\n')

    def test_descriptor_and_private_trace_verified_before_injection_command(self):
        matcher=self.c.TargetMatcher(self.plan());event=self.event(88,offset=90000)
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
            self.c.decide_io(self.sink,self.trace,matcher,self.message(self.event(offset=90000)),[])
        self.assertEqual(self.sink.getvalue(),b'')
        self.assertFalse(matcher.trace_verified)
        self.assertFalse(matcher.decision_sent)

    def test_malformed_private_trace_cannot_authorize_injection(self):
        matcher=self.c.TargetMatcher(self.plan());self.trace.write_text('{\n')
        with self.assertRaises(json.JSONDecodeError):
            self.c.decide_io(self.sink,self.trace,matcher,self.message(self.event(offset=90000)),[])
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

    def test_identical_descriptors_use_explicit_group_prefix_position(self):
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
        frames=[self.event(i+10,offset=100+i*4096,length=4096,commit=False) for i in range(5)]
        sync=self.event(20,op='sync',offset=0,length=0,flags=2)
        targets=self.c.select_targets(frames+[sync,self.event(21)],'noop')
        frame_plans=[p for key,p in targets if key[2]=='wal-frame']
        self.assertEqual([p['baseline_seq'] for p in frame_plans],[10,12,14])
        self.assertEqual([p['samples'] for p in frame_plans],[['first'],['middle'],['last']])
        self.assertEqual([len(p['prefix']) for p in frame_plans],[1,3,5])
        self.assertTrue(all(p['group_count']==5 for p in frame_plans))

    def test_single_member_group_retains_all_sample_labels(self):
        targets=self.c.select_targets([self.event(4,op='sync',offset=0,length=0),self.event()], 'noop')
        self.assertTrue(all(p['samples']==['first','middle','last'] for _,p in targets))

    def test_required_commit_group_absent_fails_discovery(self):
        with self.assertRaisesRegex(AssertionError,'commit marker not identified'):
            self.c.select_targets([self.event(4,op='sync',offset=0,length=0)],'noop')

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

    def coverage_fixture(self, acknowledged=False):
        # Synthetic gate inputs only, never application/durability evidence.
        coverage=self.c.AcceptanceCoverage()
        for op in self.c.REQUIRED_OPERATIONS:
            for sector in (512,4096):
                events=[self.event(1,name='store.db',length=4096),
                        self.event(2,name='store.db',op='sync',offset=0,length=0),
                        self.event(3,op='truncate',offset=0,length=0),self.event(4,offset=0)]
                plans=[(None,self.plan([e])) for e in events]
                coverage.plan(op,sector,plans)
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
                    coverage=self.coverage_fixture(True);del coverage.planned[(op,sector)]
                    with self.assertRaisesRegex(AssertionError,'required operation/sector family absent'):
                        coverage.require_complete()

    def test_missing_required_target_combination_cannot_pass(self):
        coverage=self.coverage_fixture(True);coverage.completed.pop()
        with self.assertRaisesRegex(AssertionError,'required target/fault cases incomplete'):
            coverage.require_complete()

    def test_zero_acknowledged_effect_coverage_cannot_pass(self):
        with self.assertRaisesRegex(AssertionError,'new acknowledged-effect recovery coverage missing'):
            self.coverage_fixture(False).require_complete()

    def test_incomplete_recovery_schedules_cannot_complete_case(self):
        coverage=self.c.AcceptanceCoverage();plan=self.plan();coverage.plan('noop',512,[(None,plan)])
        with self.assertRaisesRegex(AssertionError,'incomplete recovery schedules'):
            coverage.complete('noop',512,plan,'ioerr',self.recovered()[:-1],1)
        self.assertEqual(coverage.completed,set())

    def test_complete_synthetic_gate_inputs_are_accepted(self):
        self.coverage_fixture(True).require_complete()


if __name__=='__main__':
    unittest.main()
