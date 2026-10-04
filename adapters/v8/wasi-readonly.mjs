// uvwasi selects O_RDWR for DATASYNC/ALLOCATE/SET_SIZE rights, even when
// FD_WRITE is absent. Go requests these rights for read-only file opens.
// Reduce the rights for this explicitly read-only fixture host profile.
export function readonlyWasiImports(wasi) {
  const imports = wasi.getImportObject();
  const host = imports.wasi_snapshot_preview1;
  const open = host.path_open;
  const mutations = (1n << 0n) | (1n << 6n) | (1n << 8n) | (1n << 22n) | (1n << 23n);
  host.path_open = (fd, flags, pointer, length, oflags, base, inheriting, fdflags, result) => {
    // CREAT, EXCL, TRUNC and APPEND/DSYNC/SYNC request mutation semantics.
    if ((oflags & 13) || (fdflags & 27)) return 76; // ENOTCAPABLE
    return open(fd, flags, pointer, length, oflags, base & ~mutations, inheriting & ~mutations, fdflags, result);
  };
  return imports;
}
