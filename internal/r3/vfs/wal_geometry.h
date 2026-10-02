/* Test-only WAL format classifier. No SQLite calls, I/O or sector assumptions.
 * Rules: pinned SQLite walFrameOffset, walEncodeFrame and walIndexRecover.
 * A marker label identifies a complete header write, not a validated frame
 * checksum chain, durable transaction, or client acknowledgement. */
#ifndef CONTEXTARIUM_R3_WAL_GEOMETRY_H
#define CONTEXTARIUM_R3_WAL_GEOMETRY_H
#include <stdint.h>
#include <string.h>
typedef struct { unsigned char header[32]; uint32_t page_size; } R3WalGeometry;
static uint32_t r3_word(const unsigned char *p,int big) {
 return big ? ((uint32_t)p[0]<<24)|((uint32_t)p[1]<<16)|((uint32_t)p[2]<<8)|p[3]
            : ((uint32_t)p[3]<<24)|((uint32_t)p[2]<<16)|((uint32_t)p[1]<<8)|p[0];
}
static uint32_t r3_header_page_size(const unsigned char *p,int n) {
 if(!p || n<32)return 0;
 uint32_t magic=r3_word(p,1),size=r3_word(p+8,1),a=0,b=0;
 if((magic&0xfffffffeU)!=0x377f0682U || r3_word(p+4,1)!=3007000 ||
    size<512 || size>65536 || (size&(size-1)))return 0;
 for(int i=0;i<24;i+=8){a+=r3_word(p+i,magic&1)+b;b+=r3_word(p+i+4,magic&1)+a;}
 return a==r3_word(p+24,1) && b==r3_word(p+28,1) ? size : 0;
}
static void r3_wal_forget(R3WalGeometry *g) {memset(g,0,sizeof(*g));}
/* Observe only bytes already obtained/applied through the intercepted I/O.
 * Partial header observations invalidate, never assemble a speculative header.
 * Zero applied bytes do not change the context. Reopen starts with zero state. */
static void r3_wal_observe(R3WalGeometry *g,const void *data,int n,int64_t off) {
 if(n<=0 || off<0 || off>=32)return;
 r3_wal_forget(g);
 if(off==0 && (g->page_size=r3_header_page_size(data,n))!=0)memcpy(g->header,data,32);
}
static void r3_wal_truncated(R3WalGeometry *g,int64_t size) {
 if(size<32)r3_wal_forget(g);
}
static const char *r3_wal_classify(const R3WalGeometry *g,const void *data,int n,int64_t off,uint32_t *size) {
 const unsigned char *p=data;
 *size=g->page_size;
 if(off<0 || n<=0 || !p)return "wal-raw";
 if(off<32){
  uint32_t pending=off==0 ? r3_header_page_size(p,n) : 0;
  if(off==0 && n==32 && pending){*size=pending;return "wal-header-reset";}
  return off+n<=32 && (off!=0 || n<32) ? "wal-header-fragment" : "wal-raw";
 }
 if(!*size)return "wal-unknown";
 uint32_t within=(uint32_t)((off-32)%((int64_t)*size+24));
 if(within>=24)return (uint32_t)n<=*size+24-within ? "wal-page-data" : "wal-raw";
 if(within!=0 || n!=24)return (uint32_t)n<=24-within ? "wal-frame-fragment" : "wal-raw";
 if(!r3_word(p,1) || memcmp(p+8,g->header+16,8))return "wal-raw";
 return r3_word(p+4,1) ? "wal-commit-marker" : "wal-frame";
}
#endif
