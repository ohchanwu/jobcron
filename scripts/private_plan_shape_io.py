"""Descriptor-relative owner-only storage for the private D1 diagnostic.

No plan values or source paths are included in errors. Same-UID/root compromise
is outside the attended workstation trust boundary. No lifecycle writer here.
"""
import ctypes
import errno
import fcntl
import os
import re
import stat
import sys
import uuid

from private_plan_shape import MAX_BYTES, ShapeError, require


DIRECTORY_FLAGS = os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW
FILE_FLAGS = os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK


def no_access_acl(fd):
    if sys.platform == "darwin":
        libc = ctypes.CDLL(None, use_errno=True)
        libc.acl_get_fd_np.argtypes = [ctypes.c_int, ctypes.c_int]
        libc.acl_get_fd_np.restype = ctypes.c_void_p
        libc.acl_get_entry.argtypes = [ctypes.c_void_p, ctypes.c_int, ctypes.POINTER(ctypes.c_void_p)]
        libc.acl_get_tag_type.argtypes = [ctypes.c_void_p, ctypes.POINTER(ctypes.c_int)]
        libc.acl_free.argtypes = [ctypes.c_void_p]
        acl = libc.acl_get_fd_np(fd, 0x100)  # ACL_TYPE_EXTENDED
        if not acl and ctypes.get_errno() == errno.ENOENT:
            # Darwin reports no extended ACL as ENOENT, including for an
            # existing opened inode. A missing/unlinked object is not accepted.
            require(os.fstat(fd).st_nlink > 0, "unsafe_path")
            return
        require(bool(acl), "unsafe_path")
        try:
            entry, tag = ctypes.c_void_p(), ctypes.c_int()
            selector = 0  # ACL_FIRST_ENTRY, then ACL_NEXT_ENTRY
            while True:
                result = libc.acl_get_entry(acl, selector, ctypes.byref(entry))
                # Darwin returns -1 at end, with errno EINVAL (unlike POSIX).
                if result == -1:
                    require(ctypes.get_errno() == 22, "unsafe_path")
                    break
                require(result == 0, "unsafe_path")
                require(libc.acl_get_tag_type(entry, ctypes.byref(tag)) == 0, "unsafe_path")
                require(tag.value == 2, "unsafe_path")  # ACL_EXTENDED_DENY
                selector = -1  # ACL_NEXT_ENTRY
        finally:
            libc.acl_free(acl)
    elif sys.platform.startswith("linux"):
        require(not any(name in ("system.posix_acl_access", "system.posix_acl_default")
                        for name in os.listxattr(fd)), "unsafe_path")
    else:
        raise ShapeError("unsupported_platform")


def identity(info):
    return (info.st_dev, info.st_ino, info.st_size, info.st_mtime_ns, info.st_ctime_ns)


def leaf(name):
    require(type(name) is str and re.fullmatch(r"[a-zA-Z0-9_-][a-zA-Z0-9_.-]{0,95}", name)
            and name not in (".", ".."), "unsafe_path")


def check_file(fd):
    info = os.fstat(fd)
    require(stat.S_ISREG(info.st_mode) and stat.S_IMODE(info.st_mode) == 0o600
            and info.st_uid == os.geteuid() and info.st_nlink == 1, "unsafe_file")
    no_access_acl(fd)
    return info


class Directory:
    def __init__(self, path):
        require(type(path) is str and path.startswith("/") and
                all(part not in ("", ".", "..") for part in path.split("/")[1:]), "unsafe_path")
        self.path = path
        self.fd = -1
        current = os.open("/", DIRECTORY_FLAGS)
        try:
            for part in path.split("/")[1:]:
                child = os.open(part, DIRECTORY_FLAGS, dir_fd=current)
                os.close(current)
                current = child
                info = os.fstat(current)
                require(info.st_uid in (0, os.geteuid()) and
                        not (info.st_mode & 0o022), "unsafe_path")
                no_access_acl(current)
            info = os.fstat(current)
            require(info.st_uid == os.geteuid() and stat.S_IMODE(info.st_mode) == 0o700, "unsafe_path")
            self.fd = current
        except BaseException:
            os.close(current)
            raise

    def __enter__(self):
        return self

    def __exit__(self, *_):
        if self.fd >= 0:
            os.close(self.fd)
            self.fd = -1

    def unchanged(self):
        # Rewalk every component without following links, then compare the held
        # directory. All operations themselves remain relative to the held FD.
        with Directory(self.path) as current:
            require(os.path.samestat(os.fstat(self.fd), os.fstat(current.fd)), "path_changed")

    def read(self, name, limit=MAX_BYTES):
        leaf(name)
        fd = os.open(name, FILE_FLAGS, dir_fd=self.fd)
        try:
            before = check_file(fd)
            require(before.st_size <= limit, "input_budget")
            chunks, total = [], 0
            while True:
                block = os.read(fd, min(65536, limit + 1 - total))
                if not block:
                    break
                chunks.append(block)
                total += len(block)
                require(total <= limit, "input_budget")
            require(identity(check_file(fd)) == identity(before), "source_changed")
            require(os.path.samestat(before, os.stat(name, dir_fd=self.fd, follow_symlinks=False)),
                    "source_changed")
            self.unchanged()
            return b"".join(chunks)
        finally:
            os.close(fd)

    def write_new(self, name, data):
        leaf(name)
        fd = os.open(name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW,
                     0o600, dir_fd=self.fd)
        try:
            check_file(fd)
            remaining = memoryview(data)
            while remaining:
                count = os.write(fd, remaining)
                require(count > 0, "write_failed")
                remaining = remaining[count:]
            os.fsync(fd)
        finally:
            os.close(fd)


def read_private(path):
    require(type(path) is str, "unsafe_path")
    parent, name = os.path.split(path)
    with Directory(parent) as directory:
        return directory.read(name)


def publish(root, name, files: dict[str, bytes]):
    """No-clobber directory publication; preserves failures for private audit."""
    leaf(name)
    require(type(files) is dict and bool(files), "invalid_publication")
    staging = "stage-" + uuid.uuid4().hex
    # Flock on the already-open directory shares one lock across all cooperating
    # publishers; there is no attacker-replaceable lock-file inode.
    fcntl.flock(root.fd, fcntl.LOCK_EX)
    try:
        root.unchanged()
        try:
            os.stat(name, dir_fd=root.fd, follow_symlinks=False)
        except FileNotFoundError:
            pass
        else:
            raise ShapeError("target_exists")
        os.mkdir(staging, 0o700, dir_fd=root.fd)
        with Directory(root.path + "/" + staging) as directory:
            for filename, data in files.items():
                directory.write_new(filename, data)
                require(directory.read(filename, max(MAX_BYTES, len(data))) == data, "write_failed")
            os.fsync(directory.fd)
            root.unchanged()
            # Rename under the held lock. A same-UID uncooperative writer is
            # outside the workstation boundary; never overwrite an existing dir.
            try:
                os.stat(name, dir_fd=root.fd, follow_symlinks=False)
            except FileNotFoundError:
                pass
            else:
                raise ShapeError("target_exists")
            os.rename(staging, name, src_dir_fd=root.fd, dst_dir_fd=root.fd)
        os.fsync(root.fd)
        root.unchanged()
        with Directory(root.path + "/" + name) as directory:
            for filename, data in files.items():
                require(directory.read(filename, max(MAX_BYTES, len(data))) == data, "write_failed")
    finally:
        fcntl.flock(root.fd, fcntl.LOCK_UN)
