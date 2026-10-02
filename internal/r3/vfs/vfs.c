//go:build r3 && cgo

/* Test-only shim; sqlite3-binding.h is from the pinned go-sqlite3 module.
 * No second SQLite library, changed amalgamation, or production entry point. */
#include "sqlite3-binding.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <limits.h>
#include "wal_geometry.h"

typedef struct { sqlite3_file base; sqlite3_file *real; char name[32]; R3WalGeometry wal; } RFile;
static sqlite3_vfs shim, *native;
static FILE *logfile;
static char rootdir[PATH_MAX], phase[64]="startup", fault[32];
static long sequence, target; /* target is selected in this execution, never copied. */
static unsigned long reads, fetches, null_fetches, mmap_rejections;
static int sector=4096;
static const sqlite3_io_methods methods;
/* Fixed reasons only: no paths, SQL, write bytes, or environment values. */
static void die(const char *reason) {
 dprintf(STDERR_FILENO,"{\"kind\":\"vfs-fatal\",\"reason\":\"%s\",\"seq\":%ld}\n",reason,sequence);
 _exit(91);
}
static const char* basename_checked(const char *path) {
 size_t n=strlen(rootdir);
 if(!path || strncmp(path,rootdir,n) || path[n]!='/' || strchr(path+n+1,'/') || strstr(path+n+1,"..")) die("path-boundary");
 const char *p=path+n+1;
 if(strcmp(p,"store.db") && strcmp(p,"store.db-wal") && strcmp(p,"store.db-journal") && strcmp(p,"probe.db")) die("file-not-allowed");
 return p;
}
static const char* role(const char *name) {
 if(strstr(name,"-wal")) return "wal";
 if(strstr(name,"-journal")) return "journal";
 return "database";
}
static long pre(const char *op,const char *name,sqlite3_int64 off,int n,int flags,const void *data,const R3WalGeometry *g) {
 long id=++sequence;
 /* Metadata is separate from the private, byte-bearing reconstruction trace. */
 if(fprintf(stderr,"{\"kind\":\"vfs-operation\",\"seq\":%ld,\"stage\":\"pre\",\"phase\":\"%s\",\"op\":\"%s\",\"role\":\"%s\",\"offset\":%lld,\"length\":%d,\"flags\":%d}\n",id,phase,op,role(name),(long long)off,n,flags)<0 || fflush(stderr)) die("metadata-write");
 if(fprintf(logfile,"{\"seq\":%ld,\"stage\":\"pre\",\"phase\":\"%s\",\"op\":\"%s\",\"name\":\"%s\",\"role\":\"%s\",\"offset\":%lld,\"length\":%d,\"flags\":%d,\"hex\":\"",id,phase,op,name,role(name),(long long)off,n,flags)<0) die("trace-write");
 if(data) for(int i=0;i<n;i++) if(fprintf(logfile,"%02x",((const unsigned char*)data)[i])<0) die("trace-write");
 if(fputs("\",\"wal_header\":\"",logfile)==EOF)die("trace-write");
 if(g && !strcmp(role(name),"wal") && g->page_size)
  for(int i=0;i<32;i++)if(fprintf(logfile,"%02x",g->header[i])<0)die("trace-write");
 if(fputs("\"}\n",logfile)==EOF || fflush(logfile)) die("trace-flush");
 return id;
}
static void post(long id,int rc,int applied) {
 if(fprintf(stderr,"{\"kind\":\"vfs-operation\",\"seq\":%ld,\"stage\":\"post\",\"rc\":%d,\"applied\":%d}\n",id,rc,applied)<0 || fflush(stderr)) die("metadata-write");
 if(fprintf(logfile,"{\"seq\":%ld,\"stage\":\"post\",\"rc\":%d,\"applied\":%d}\n",id,rc,applied)<0 || fflush(logfile)) die("trace-flush");
}
static void reached(long id) {
 if(printf("{\"kind\":\"target\",\"seq\":%ld,\"mode\":\"%s\"}\n",id,fault)<0 || fflush(stdout)) die("protocol-write");
}
static void pause_here(void) { char c; if(read(STDIN_FILENO,&c,1)!=1) die("barrier-input-eof"); die("unexpected-barrier-release"); }
/* Classify the pending operation before any native write/sync/truncate. No page
 * bytes cross the public decision protocol. The private trace still has bytes. */
static const char* semantic(const char *op,const char *name,sqlite3_int64 off,int n,const void *data,const R3WalGeometry *g,uint32_t *size) {
 *size=0;
 if(strcmp(role(name),"wal"))return op;
 *size=g->page_size;
 if(strcmp(op,"write"))return op;
 return r3_wal_classify(g,data,n,off,size);
}
static void decide(long id,const char *op,const char *name,sqlite3_int64 off,int n,int flags,const void *data,const R3WalGeometry *g) {
 if(target || !strcmp(fault,"none")) return;
 /* One pending I/O at a time. The controller verifies its planned descriptor
  * and Nth stable-selector group position before replying. An EOF/bad/stale
  * reply exits without performing this operation or injecting a fault elsewhere. */
 uint32_t page_size;const char *meaning=semantic(op,name,off,n,data,g,&page_size);
 if(printf("{\"kind\":\"io-candidate\",\"seq\":%ld,\"phase\":\"%s\",\"op\":\"%s\",\"name\":\"%s\",\"role\":\"%s\",\"meaning\":\"%s\",\"offset\":%lld,\"length\":%d,\"flags\":%d,\"wal_page_size\":%u}\n",
    id,phase,op,name,role(name),meaning,(long long)off,n,flags,page_size)<0 || fflush(stdout)) die("protocol-write");
 char reply[64];size_t used=0;
 do {
  if(used==sizeof(reply)-1)die("decision-reply-invalid");
  if(read(STDIN_FILENO,reply+used,1)!=1)die("decision-input-eof");
 } while(reply[used++]!='\n');
 reply[used]=0;
 char action=0,extra=0;long acknowledged=0;
 if(sscanf(reply,"%c %ld %c",&action,&acknowledged,&extra)!=2 || acknowledged!=id || (action!='c' && action!='i'))die("decision-reply-invalid");
 if(action=='i')target=id;
}
static int before(long id,const char *op,const char *name,sqlite3_int64 off,int n,int flags,const void *data,const R3WalGeometry *g) {
 decide(id,op,name,off,n,flags,data,g);
 if(id!=target) return 0;
 if(!strcmp(fault,"cut-after")) return 0;
 reached(id);
 if(!strcmp(fault,"cut-before")) pause_here();
 if(!strcmp(fault,"full")) return SQLITE_FULL;
 if(!strcmp(fault,"ioerr") || !strcmp(fault,"partial")) return SQLITE_IOERR;
 die("unknown-fault"); return SQLITE_IOERR;
}
static void after(long id) {if(id==target && !strcmp(fault,"cut-after")){reached(id);pause_here();}}
static int close_file(sqlite3_file *f) {RFile *p=(RFile*)f;int rc=p->real->pMethods->xClose(p->real);sqlite3_free(p->real);p->base.pMethods=0;return rc;}
static int read_file(sqlite3_file *f,void *b,int n,sqlite3_int64 o){
 RFile*p=(RFile*)f;reads++;int rc=p->real->pMethods->xRead(p->real,b,n,o);
 /* Learn only from SQLite's own successful header read; add no native I/O. */
 if(!strcmp(role(p->name),"wal") && o<32){
  if(rc==SQLITE_OK)r3_wal_observe(&p->wal,b,n,o);else r3_wal_forget(&p->wal);
 }
 return rc;
}
static int write_file(sqlite3_file *f,const void *b,int n,sqlite3_int64 o){
 RFile*p=(RFile*)f;long id=pre("write",p->name,o,n,0,b,&p->wal);int rc=before(id,"write",p->name,o,n,0,b,&p->wal),applied=0;
 if(rc && id==target && !strcmp(fault,"partial")){applied=n/2;int inner=p->real->pMethods->xWrite(p->real,b,applied,o);if(inner!=SQLITE_OK)die("native-partial-write-failed");}
 else if(!rc){rc=p->real->pMethods->xWrite(p->real,b,n,o);if(rc==SQLITE_OK)applied=n;else die("native-write-failed");}
 if(!strcmp(role(p->name),"wal"))r3_wal_observe(&p->wal,b,applied,o);
 post(id,rc,applied);after(id);return rc;
}
static int truncate_file(sqlite3_file*f,sqlite3_int64 size){RFile*p=(RFile*)f;long id=pre("truncate",p->name,size,0,0,0,&p->wal);int rc=before(id,"truncate",p->name,size,0,0,0,&p->wal);if(!rc)rc=p->real->pMethods->xTruncate(p->real,size);if(!rc && !strcmp(role(p->name),"wal"))r3_wal_truncated(&p->wal,size);post(id,rc,rc==0);after(id);return rc;}
static int sync_file(sqlite3_file*f,int flags){RFile*p=(RFile*)f;long id=pre("sync",p->name,0,0,flags,0,&p->wal);int rc=before(id,"sync",p->name,0,0,flags,0,&p->wal);if(!rc)rc=p->real->pMethods->xSync(p->real,flags);post(id,rc,rc==0);after(id);return rc;}
static int size_file(sqlite3_file*f,sqlite3_int64*n){RFile*p=(RFile*)f;return p->real->pMethods->xFileSize(p->real,n);}
static int lock_file(sqlite3_file*f,int n){RFile*p=(RFile*)f;return p->real->pMethods->xLock(p->real,n);}
static int unlock_file(sqlite3_file*f,int n){RFile*p=(RFile*)f;return p->real->pMethods->xUnlock(p->real,n);}
static int reserved(sqlite3_file*f,int*n){RFile*p=(RFile*)f;return p->real->pMethods->xCheckReservedLock(p->real,n);}
static int control(sqlite3_file*f,int op,void*a){
 RFile*p=(RFile*)f;
 if(op==SQLITE_FCNTL_FILE_POINTER){*(sqlite3_file**)a=f;return SQLITE_OK;}
 if(op==SQLITE_FCNTL_VFS_POINTER){*(sqlite3_vfs**)a=&shim;return SQLITE_OK;}
 /* These native controls can resize/write without calling the outer methods. */
 if(op==SQLITE_FCNTL_MMAP_SIZE){mmap_rejections++;return SQLITE_NOTFOUND;}
 if(op==SQLITE_FCNTL_SIZE_HINT || op==SQLITE_FCNTL_CHUNK_SIZE) return SQLITE_NOTFOUND;
 return p->real->pMethods->xFileControl(p->real,op,a);
}
static int sectors(sqlite3_file*f){(void)f;return sector;}
static int characteristics(sqlite3_file*f){(void)f;return 0;} /* No atomic/PSOW/safe-append claims. */
static int shmmap(sqlite3_file*f,int i,int n,int w,void volatile**p){RFile*r=(RFile*)f;return r->real->pMethods->xShmMap(r->real,i,n,w,p);}
static int shmlock(sqlite3_file*f,int i,int n,int flags){RFile*r=(RFile*)f;return r->real->pMethods->xShmLock(r->real,i,n,flags);}
static void shmbarrier(sqlite3_file*f){RFile*r=(RFile*)f;r->real->pMethods->xShmBarrier(r->real);}
static int shmunmap(sqlite3_file*f,int del){RFile*r=(RFile*)f;return r->real->pMethods->xShmUnmap(r->real,del);}
static int fetch(sqlite3_file*f,sqlite3_int64 o,int n,void**p){(void)f;(void)o;(void)n;fetches++;*p=0;null_fetches++;return SQLITE_OK;}
static int unfetch(sqlite3_file*f,sqlite3_int64 o,void*p){(void)f;(void)o;if(p)die("unexpected-mapped-pointer");return SQLITE_OK;}
static const sqlite3_io_methods methods={3,close_file,read_file,write_file,truncate_file,sync_file,size_file,lock_file,unlock_file,reserved,control,sectors,characteristics,shmmap,shmlock,shmbarrier,shmunmap,fetch,unfetch};
static int open_file(sqlite3_vfs*v,const char*name,sqlite3_file*f,int flags,int*out){
 (void)v;const char *shortname=basename_checked(name);RFile*p=(RFile*)f;memset(p,0,sizeof(*p));strcpy(p->name,shortname);
 p->real=sqlite3_malloc(native->szOsFile);if(!p->real)return SQLITE_NOMEM;memset(p->real,0,native->szOsFile);
 int exists=0;native->xAccess(native,name,SQLITE_ACCESS_EXISTS,&exists);
 long id=pre("open",shortname,0,0,exists,0,&p->wal);int rc=native->xOpen(native,name,p->real,flags,out);
 if(p->real->pMethods)p->base.pMethods=&methods;else{sqlite3_free(p->real);p->real=0;}
 post(id,rc,rc==0 && !exists);return rc;
}
static int delete_file(sqlite3_vfs*v,const char*name,int syncdir){(void)v;const char*n=basename_checked(name);long id=pre("delete",n,0,0,syncdir,0,0);int rc=native->xDelete(native,name,syncdir);post(id,rc,rc==0);return rc;}
int r3_init(const char*root,const char*trace,const char*mode,int unit){
 if(strlen(root)>=sizeof(rootdir) || strlen(mode)>=sizeof(fault) || (unit!=512 && unit!=4096)) return 1;
 if(strcmp(mode,"none") && strcmp(mode,"cut-before") && strcmp(mode,"cut-after") && strcmp(mode,"ioerr") && strcmp(mode,"full") && strcmp(mode,"partial")) return 1;
 strcpy(rootdir,root);strcpy(fault,mode);target=0;sector=unit;
 logfile=fopen(trace,"wx");if(!logfile)return 2;
 native=sqlite3_vfs_find(0);if(!native || native->iVersion<2)return 3;
 shim=*native;shim.zName="contextarium-r3-test-only";shim.szOsFile=sizeof(RFile);shim.xOpen=open_file;shim.xDelete=delete_file;
 return sqlite3_vfs_register(&shim,1);
}
void r3_phase(const char*p){
 if(strlen(p)>=sizeof(phase))die("phase-length");
 for(const char*s=p;*s;s++)if((*s<'a'||*s>'z') && *s!='-' && (*s<'0'||*s>'9'))die("phase-label");
 strcpy(phase,p);
}
void r3_report(void){
 if(!native)return;
 if(printf("{\"kind\":\"vfs-coverage\",\"fetch_policy\":\"always-null-no-native-delegation\",\"reads\":%lu,\"fetches\":%lu,\"null_fetches\":%lu,\"mmap_control_rejections\":%lu}\n",reads,fetches,null_fetches,mmap_rejections)<0 || fflush(stdout))die("protocol-write");
}
int r3_probe(const char*path){
 sqlite3_file*f=sqlite3_malloc(shim.szOsFile);int flags=0,rc=shim.xOpen(&shim,path,f,SQLITE_OPEN_READWRITE|SQLITE_OPEN_CREATE|SQLITE_OPEN_MAIN_DB,&flags);if(rc)return rc;
 char b[7]={0};sqlite3_int64 size=0;
 #define DO(x) if((rc=(x))!=SQLITE_OK)return rc
 DO(f->pMethods->xWrite(f,"abcdef",6,0));DO(f->pMethods->xRead(f,b,6,0));if(memcmp(b,"abcdef",6))return 92;
 DO(f->pMethods->xSync(f,SQLITE_SYNC_FULL));
 DO(f->pMethods->xWrite(f,"XYZ",3,2));DO(f->pMethods->xRead(f,b,6,0));if(memcmp(b,"abXYZf",6))return 93;
 DO(f->pMethods->xTruncate(f,4));DO(f->pMethods->xFileSize(f,&size));if(size!=4)return 94;
 DO(f->pMethods->xSync(f,SQLITE_SYNC_FULL));DO(f->pMethods->xClose(f));sqlite3_free(f);
 DO(shim.xDelete(&shim,path,1));return 0;
}
