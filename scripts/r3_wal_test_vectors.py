"""Synthetic WAL format builders for explicitly authorized future tests only.

These builders follow the documented format, not either classifier's output.
They do not open a database or execute application code.
"""
import struct


def checksum(data, big=False, initial=(0, 0)):
    words=struct.unpack(('>' if big else '<')+'I'*(len(data)//4),data)
    a,b=initial
    for x,y in zip(words[::2],words[1::2]):
        a=(a+x+b)&0xffffffff;b=(b+y+a)&0xffffffff
    return a,b


def wal_header(size=4096, *, big=False, salt=(17,29)):
    prefix=struct.pack('>6I',0x377f0682+int(big),3007000,size,0,*salt)
    return prefix+struct.pack('>2I',*checksum(prefix,big))


def frame_header(size=4096, *, commit=True, big=False, salt=(17,29)):
    # Valid first-frame checksum for a zero-filled page. Later-frame targeting
    # fixtures use header geometry only and do not claim a checksum chain.
    header=wal_header(size,big=big,salt=salt)
    prefix=struct.pack('>2I',1,int(commit))
    sums=checksum(prefix,big,struct.unpack('>2I',header[24:]))
    sums=checksum(bytes(size),big,sums)
    return prefix+struct.pack('>2I',*salt)+struct.pack('>2I',*sums)


def frame_offset(index=0, size=4096):
    return 32+index*(size+24)


def page_offset(index=0, size=4096):
    return frame_offset(index,size)+24
