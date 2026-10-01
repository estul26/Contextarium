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
                    role='wal' if name.endswith('-wal') else 'database',op=op,
                    offset=offset,length=length,flags=flags,hex=data.hex())

    def message(self, event):
        return dict(self.c.event_descriptor(event),seq=event['seq'],kind='io-candidate')

    def plan(self, items=None, index=0):
        return self.c.make_target(items or [self.event()],index,['last'])

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

    def test_changed_range_fails_before_sending_decision(self):
        matcher=self.c.TargetMatcher(self.plan())
        with self.assertRaisesRegex(AssertionError,'required target group prefix changed'):
            self.deliver(matcher,self.event(47,offset=90000))
        self.assertEqual(self.sink.getvalue(),b'')
        self.assertFalse(matcher.decision_sent)

    def test_reordered_group_prefix_is_not_substituted(self):
        first=self.event(10,offset=4000);last=self.event(47)
        matcher=self.c.TargetMatcher(self.plan([first,last],1))
        with self.assertRaisesRegex(AssertionError,'required target group prefix changed'):
            self.deliver(matcher,last)
        self.assertEqual(self.sink.getvalue(),b'')

    def test_descriptor_and_private_trace_verified_before_injection_command(self):
        matcher=self.c.TargetMatcher(self.plan());event=self.event(88)
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
