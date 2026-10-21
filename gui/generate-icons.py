"""Reproduce the simple code-drawn X application icon; no external assets."""
from pathlib import Path
import struct
import zlib


def pixels(size):
    rows = []
    for y in range(size):
        row = bytearray()
        for x in range(size):
            mark = size*.22 < x < size*.78 and size*.22 < y < size*.78 and min(abs(x-y), abs(x+y-size+1)) < size*.075
            row.extend((255,255,255,255) if mark else (17,101,87,255))
        rows.append(bytes(row))
    return rows


def png(size):
    def chunk(kind,data):
        return struct.pack('!I',len(data))+kind+data+struct.pack('!I',zlib.crc32(kind+data)&0xffffffff)
    return b'\x89PNG\r\n\x1a\n'+chunk(b'IHDR',struct.pack('!2I5B',size,size,8,6,0,0,0))+chunk(b'IDAT',zlib.compress(b''.join(b'\0'+row for row in pixels(size))))+chunk(b'IEND',b'')


def main():
    output=Path(__file__).parent/'src-tauri/icons'
    output.mkdir(exist_ok=True)
    image=png(32)
    (output/'icon.ico').write_bytes(struct.pack('<3H',0,1,1)+struct.pack('<4B2H2I',32,32,0,0,1,32,len(image),22)+image)
    (output/'icon.png').write_bytes(png(128))
    parts=[kind+struct.pack('!I',len(data)+8)+data for kind,data in ((b'ic07',png(128)),(b'ic08',png(256)))]
    content=b''.join(parts)
    (output/'icon.icns').write_bytes(b'icns'+struct.pack('!I',len(content)+8)+content)
    (output/'icon.svg').write_text('<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128" viewBox="0 0 128 128"><rect width="128" height="128" rx="20" fill="#116557"/><path d="M35 35L93 93M93 35L35 93" stroke="white" stroke-width="16"/></svg>\n',encoding='utf-8')


if __name__=='__main__':main()
