"""Prepared format regressions, NOT RUN. Separate execution approval required.

When authorized, builds a tiny bridge to the exact pure C classifier used by
the VFS, not SQLite/application code. No database or fault schedule is run.
Each language must meet independent, format-derived expected labels/geometry.
"""
import ctypes
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock

from r3_wal_test_vectors import wal_header, frame_header, frame_offset, page_offset


C_BRIDGE = r'''
#include <stdlib.h>
#include "wal_geometry.h"
void *new_context(void){return calloc(1,sizeof(R3WalGeometry));}
void free_context(void *g){free(g);}
void observe(void *g,const char *p,int n,int64_t o){r3_wal_observe(g,p,n,o);}
void truncate_context(void *g,int64_t n){r3_wal_truncated(g,n);}
unsigned int context_size(void *g){return ((R3WalGeometry*)g)->page_size;}
const char *classify(void *g,const char *p,int n,int64_t o,uint32_t *size){return r3_wal_classify(g,p,n,o,size);}
'''


class WalClassifierTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.directory=tempfile.TemporaryDirectory(prefix='r3-wal-format-unit-')
        cls.addClassCleanup(cls.directory.cleanup)
        root=Path(cls.directory.name);source=root/'bridge.c';library=root/'bridge.so'
        source.write_text(C_BRIDGE)
        include=Path(__file__).resolve().parents[1]/'internal/r3/vfs'
        subprocess.run(['cc','-std=c99','-shared','-fPIC','-I'+str(include),str(source),'-o',str(library)],
                       check=True,capture_output=True,timeout=30)
        cls.c=ctypes.CDLL(str(library))
        cls.c.new_context.restype=ctypes.c_void_p
        cls.c.free_context.argtypes=[ctypes.c_void_p];cls.c.free_context.restype=None
        cls.c.observe.argtypes=[ctypes.c_void_p,ctypes.c_char_p,ctypes.c_int,ctypes.c_int64];cls.c.observe.restype=None
        cls.c.truncate_context.argtypes=[ctypes.c_void_p,ctypes.c_int64];cls.c.truncate_context.restype=None
        cls.c.context_size.argtypes=[ctypes.c_void_p];cls.c.context_size.restype=ctypes.c_uint32
        cls.c.classify.argtypes=[ctypes.c_void_p,ctypes.c_char_p,ctypes.c_int,ctypes.c_int64,ctypes.POINTER(ctypes.c_uint32)]
        cls.c.classify.restype=ctypes.c_char_p
        spec=importlib.util.spec_from_file_location('r3_wal_format_controller',Path(__file__).with_name('r3-check.py'))
        cls.controller=importlib.util.module_from_spec(spec);spec.loader.exec_module(cls.controller)

    def setUp(self):
        self.context=self.c.new_context();self.assertTrue(self.context)
        self.addCleanup(self.c.free_context,self.context)

    def install(self, header):
        self.c.observe(self.context,header,len(header),0)

    def assert_classification(self, header, offset, data, label, size):
        actual_size=ctypes.c_uint32()
        actual=self.c.classify(self.context,data,len(data),offset,ctypes.byref(actual_size)).decode('ascii')
        self.assertEqual((actual,actual_size.value),(label,size))
        self.assertEqual(self.controller.wal_write_classification(data,offset,header),(label,size))

    def test_valid_noncommit_header_at_first_frame_boundary(self):
        h=wal_header();self.install(h)
        self.assert_classification(h,32,frame_header(commit=False),'wal-frame',4096)

    def test_valid_commit_header_at_first_frame_boundary(self):
        h=wal_header();self.install(h)
        self.assert_classification(h,32,frame_header(),'wal-commit-marker',4096)

    def test_offset_56_page_data_counterexample(self):
        h=wal_header();self.install(h)
        self.assert_classification(h,56,frame_header(),'wal-page-data',4096)

    def test_24_byte_page_fragment_with_nonzero_marker_bytes(self):
        h=wal_header();self.install(h)
        self.assert_classification(h,page_offset(2)+100,frame_header(),'wal-page-data',4096)

    def test_page_sizes_and_frame_boundaries_follow_format(self):
        for size in (512,1024,2048,4096,8192,16384,32768,65536):
            with self.subTest(size=size):
                h=wal_header(size);self.install(h)
                self.assert_classification(h,32+3*(size+24),frame_header(size),'wal-commit-marker',size)
                self.assert_classification(h,56+3*(size+24),frame_header(size),'wal-page-data',size)

    def test_4096_boundary_is_not_a_1024_frame_boundary(self):
        h=wal_header(1024);self.install(h)
        self.assert_classification(h,4152,frame_header(1024),'wal-page-data',1024)

    def test_sector_parameter_cannot_define_database_geometry(self):
        h=wal_header(1024);self.install(h)
        for sector in (512,4096):
            event=dict(phase='mutation-noop',name='store.db-wal',role='wal',op='write',offset=1080,
                       length=24,flags=0,hex=frame_header(1024).hex(),wal_header=h.hex(),sector=sector)
            self.assertEqual(self.controller.event_descriptor(event)['wal_page_size'],1024)
            self.assertEqual(self.controller.event_descriptor(event)['meaning'],'wal-commit-marker')
        # The shared C classifier API has no sector-size input or global.
        self.assert_classification(h,1080,frame_header(1024),'wal-commit-marker',1024)

    def test_both_header_checksum_byte_orders(self):
        for big in (False,True):
            h=wal_header(8192,big=big);self.install(h)
            self.assert_classification(h,32,frame_header(8192,big=big),'wal-commit-marker',8192)

    def test_invalid_header_magic_version_size_or_checksum_never_sets_context(self):
        good=wal_header()
        invalid=[b'\0'*32,wal_header(256),wal_header(1000),wal_header(131072)]
        for index in (0,4,24):
            bad=bytearray(good);bad[index]^=1;invalid.append(bytes(bad))
        for h in invalid:
            with self.subTest(header_length=len(h)):
                self.install(h)
                self.assertEqual(self.c.context_size(self.context),0)
                self.assertEqual(self.controller.wal_header_page_size(h),0)
                with self.assertRaisesRegex(AssertionError,'invalid private WAL geometry header'):
                    self.controller.wal_write_classification(frame_header(),32,h)

    def test_invalid_complete_header_write_is_raw_not_reset(self):
        h=wal_header();self.install(h)
        self.assert_classification(h,0,bytes(32),'wal-raw',4096)

    def test_misaligned_24_byte_header_like_write_is_not_a_marker(self):
        h=wal_header();self.install(h)
        self.assert_classification(h,33,frame_header(),'wal-raw',4096)

    def test_unknown_context_never_claims_commit(self):
        self.assert_classification(b'',32,frame_header(),'wal-unknown',0)

    def test_new_full_header_classifies_before_context_is_installed(self):
        h=wal_header(8192)
        self.assert_classification(b'',0,h,'wal-header-reset',8192)
        self.assertEqual(self.c.context_size(self.context),0)
        self.install(h)
        self.assert_classification(h,32,frame_header(8192),'wal-commit-marker',8192)

    def test_split_frame_header_parts_never_claim_complete_marker(self):
        h=wal_header();self.install(h);frame=frame_header()
        for split in (1,4,8,16,23):
            self.assert_classification(h,32,frame[:split],'wal-frame-fragment',4096)
            self.assert_classification(h,32+split,frame[split:],'wal-frame-fragment',4096)

    def test_split_page_data_fragments_stay_page_data(self):
        h=wal_header();self.install(h)
        self.assert_classification(h,56,bytes(24),'wal-page-data',4096)
        self.assert_classification(h,80,bytes(4072),'wal-page-data',4096)

    def test_cross_boundary_and_combined_writes_remain_raw(self):
        h=wal_header();self.install(h)
        self.assert_classification(h,32,frame_header()+bytes(4096),'wal-raw',4096)
        self.assert_classification(h,4151,bytes(2),'wal-raw',4096)
        self.assert_classification(h,0,h+frame_header(),'wal-raw',4096)

    def test_zero_page_number_or_old_salt_is_not_a_marker(self):
        h=wal_header();self.install(h)
        zero=bytearray(frame_header());zero[:4]=bytes(4)
        self.assert_classification(h,32,bytes(zero),'wal-raw',4096)
        self.assert_classification(h,32,frame_header(salt=(90,91)),'wal-raw',4096)

    def test_reset_replaces_size_and_salts_before_reuse(self):
        old=wal_header();self.install(old);new=wal_header(1024,salt=(90,91))
        self.assert_classification(old,0,new,'wal-header-reset',1024)
        self.install(new)
        self.assert_classification(new,1080,frame_header(1024,salt=(90,91)),'wal-commit-marker',1024)
        self.assert_classification(new,1080,frame_header(),'wal-raw',1024)

    def test_partial_reset_invalidates_old_context_without_assembling_parts(self):
        h=wal_header();self.install(h);new=wal_header(1024)
        self.assert_classification(h,0,new[:16],'wal-header-fragment',4096)
        self.c.observe(self.context,new[:16],16,0)
        self.c.observe(self.context,new[16:],16,16)
        self.assertEqual(self.c.context_size(self.context),0)
        self.assert_classification(b'',32,frame_header(1024),'wal-unknown',0)

    def test_zero_applied_header_bytes_preserve_context(self):
        h=wal_header();self.install(h)
        self.c.observe(self.context,wal_header(1024),0,0)
        self.assert_classification(h,32,frame_header(),'wal-commit-marker',4096)

    def test_truncate_below_header_invalidates_but_retained_header_survives(self):
        h=wal_header();self.install(h)
        self.c.truncate_context(self.context,32)
        self.assert_classification(h,32,frame_header(),'wal-commit-marker',4096)
        self.c.truncate_context(self.context,0)
        self.assert_classification(b'',32,frame_header(),'wal-unknown',0)

    def test_complete_existing_header_read_can_initialize_new_handle(self):
        h=wal_header(2048);self.c.observe(self.context,h+bytes(100),132,0)
        self.assert_classification(h,2104,frame_header(2048),'wal-commit-marker',2048)

    def test_reopen_does_not_inherit_previous_handle_context(self):
        self.install(wal_header())
        other=self.c.new_context();self.assertTrue(other);self.addCleanup(self.c.free_context,other)
        self.assertEqual(self.c.context_size(other),0)

    def test_public_false_marker_at_page_offset_cannot_arm_injection(self):
        c=self.controller;h=wal_header()
        row=dict(seq=1,stage='pre',phase='mutation-noop',name='store.db-wal',role='wal',op='write',
                 offset=32,length=24,flags=0,hex=frame_header().hex(),wal_header=h.hex())
        plan=c.make_target([row],0,['first','middle','last']);matcher=c.TargetMatcher(plan)
        live=dict(c.event_descriptor(row),seq=2,offset=56)
        sink=io.BytesIO()
        with self.assertRaisesRegex(AssertionError,'invalid WAL frame-header geometry'):
            c.decide_io(sink,Path(self.directory.name)/'unused',matcher,live,[])
        self.assertEqual(sink.getvalue(),b'')

    def test_private_format_validation_precedes_inject_reply(self):
        c=self.controller;h=wal_header()
        row=dict(seq=1,stage='pre',phase='mutation-noop',name='store.db-wal',role='wal',op='write',
                 offset=32,length=24,flags=0,hex=frame_header().hex(),wal_header=h.hex())
        plan=c.make_target([row],0,['first','middle','last']);matcher=c.TargetMatcher(plan)
        trace=Path(self.directory.name)/'pending.jsonl';trace.write_text(json.dumps(row)+'\n')
        ordering=[];verify=c.verify_pending_trace
        def checking(path,message):
            self.assertEqual(ordering,[]);verify(path,message);ordering.append('verified')
        class Sink(io.BytesIO):
            def write(inner,data):
                self.assertEqual(ordering,['verified']);ordering.append('reply');return super().write(data)
        sink=Sink()
        with mock.patch.object(c,'verify_pending_trace',side_effect=checking):
            c.decide_io(sink,trace,matcher,dict(c.event_descriptor(row),seq=1),[])
        self.assertEqual(ordering,['verified','reply']);self.assertEqual(sink.getvalue(),b'i 1\n')

    def test_private_geometry_context_mismatch_cannot_authorize_injection(self):
        c=self.controller
        row=dict(seq=1,stage='pre',phase='mutation-noop',name='store.db-wal',role='wal',op='write',
                 offset=32,length=24,flags=0,hex=frame_header().hex(),wal_header=wal_header().hex())
        matcher=c.TargetMatcher(c.make_target([row],0,['first','middle','last']))
        trace=Path(self.directory.name)/'pending-mismatch.jsonl'
        trace.write_text(json.dumps(dict(row,wal_header=wal_header(1024).hex()))+'\n')
        sink=io.BytesIO()
        with self.assertRaisesRegex(AssertionError,'pending trace descriptor mismatch'):
            c.decide_io(sink,trace,matcher,dict(c.event_descriptor(row),seq=1),[])
        self.assertEqual(sink.getvalue(),b'');self.assertFalse(matcher.decision_sent)


if __name__=='__main__':
    unittest.main()
