"""Temporary legacy participation; docs/adr/0093-runtime-transition-guard.md:145."""

import fcntl
import os
import secrets
import stat


class TransitionError(ValueError):
    pass


def _failure():
    return TransitionError("unsafe or unavailable runtime transition storage")


def _safe(info, kind):
    if info.st_uid != os.geteuid():
        return False
    mode = stat.S_IMODE(info.st_mode)
    if kind == "control":
        return stat.S_ISREG(info.st_mode) and mode == 0o600 and info.st_nlink == 1 and info.st_size == 0
    return stat.S_ISDIR(info.st_mode) and not mode & 0o7022 and (kind != "private" or mode == 0o700)


def _same(first, second):
    return first.st_dev == second.st_dev and first.st_ino == second.st_ino and stat.S_IFMT(first.st_mode) == stat.S_IFMT(second.st_mode)


class Guard:
    def __init__(self):
        self.pins = []
        self.marker = None
        self.activity = None
        self.clean = False
        self.closed = False

    @staticmethod
    def _named(parent, name):
        return os.stat(name, dir_fd=parent, follow_symlinks=False)

    def check(self):
        for entry in self.pins:
            parent, name, descriptor, original, kind = entry
            current = self._named(parent, name)
            if not _safe(current, kind) or not _same(current, original):
                raise _failure()
            if descriptor is not None:
                opened = os.fstat(descriptor)
                if not _safe(opened, kind) or not _same(current, opened):
                    raise _failure()
                if kind == "control" and os.read(descriptor, 1):
                    raise _failure()

    def existing(self, parent, name, kind):
        before = self._named(parent, name)
        if not _safe(before, kind):
            raise _failure()
        flags = os.O_RDWR if kind == "control" else os.O_RDONLY | os.O_DIRECTORY
        descriptor = os.open(name, flags | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC, dir_fd=parent)
        entry = [parent, name, descriptor, before, kind]
        self.pins.append(entry)
        self.check()
        return entry

    def directory(self, parent, name, private):
        self.check()
        try:
            self._named(parent[2], name)
        except FileNotFoundError:
            try:
                os.mkdir(name, 0o700, dir_fd=parent[2])
            except FileExistsError:
                pass
        return self.existing(parent[2], name, "private" if private else "directory")

    def control(self, parent, name, reuse):
        self.check()
        try:
            descriptor = os.open(name, os.O_RDWR | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC,
                                 0o600, dir_fd=parent[2])
        except FileExistsError:
            if not reuse:
                raise _failure() from None
            return self.existing(parent[2], name, "control")
        entry = [parent[2], name, descriptor, None, "control"]
        self.pins.append(entry)
        entry[3] = os.fstat(descriptor)
        self.check()
        return entry

    def sync(self, descriptor):
        self.check()
        os.fsync(descriptor)
        self.check()

    def _close_files(self):
        self.closed = True
        failed = False
        for entry in reversed(self.pins):
            if entry[2] is not None:
                try:
                    os.close(entry[2])
                except OSError:
                    failed = True
                finally:
                    entry[2] = None
        if failed:
            raise TransitionError("cannot close runtime transition storage")

    # Caller establishes harmless refusal or durable terminal ownership first.
    # docs/adr/0093-runtime-transition-guard.md:79.
    def close(self, clean=False):
        if self.closed:
            raise _failure()
        try:
            self.check()
            if clean and self.marker is not None:
                descriptor = self.marker[2]
                self.marker[2] = None
                os.close(descriptor)
                self.check()
                os.unlink(self.marker[1], dir_fd=self.activity[2])
                self.pins.pop()
                # A post-unlink sync failure reports uncertainty without creating
                # replacement evidence. docs/adr/0093-runtime-transition-guard.md:83.
                self.sync(self.activity[2])
        except OSError:
            raise _failure() from None
        finally:
            self._close_files()


def release(guard, primary=None):
    """Report release failure without discarding a primary process identity."""
    # Keep the direct exception type/PID used by checkpoint attribution. Cleanup
    # diagnostics stay fixed rather than exposing arbitrary syscall details.
    # docs/adr/0093-runtime-transition-guard.md:183.
    try:
        guard.close(guard.clean)
    except BaseException:
        pass
    else:
        return
    cleanup = TransitionError("cannot close runtime transition storage")
    if primary is None:
        raise cleanup from None
    if isinstance(primary, OSError) and primary.strerror is not None:
        primary.strerror = str(primary.strerror) + "; cannot close runtime transition storage"
    else:
        primary.args = (str(primary) + "; cannot close runtime transition storage",)
    raise primary from cleanup


def _acquire(root, exclusive):
    root = os.fspath(root)
    if not root or "\0" in root or ".." in root.split("/"):
        raise _failure()
    while root != "/" and (root.endswith("/") or root.endswith("/.")):
        root = root[:-2] if root.endswith("/.") else root[:-1]
    guard = Guard()
    success = False
    try:
        project = guard.existing(None, root, "directory")
        state = guard.directory(project, ".factory", False)
        lock = guard.control(state, "runtime-transition.lock", True)
        fcntl.flock(lock[2], (fcntl.LOCK_EX if exclusive else fcntl.LOCK_SH) | fcntl.LOCK_NB)
        guard.check()
        guard.activity = guard.directory(state, "runtime-activity", True)
        if exclusive:
            with os.scandir(guard.activity[2]) as entries:
                if next(entries, None) is not None:
                    raise TransitionError("runtime activity remains or cannot be inspected")
        else:
            guard.marker = guard.control(guard.activity, "activity-" + secrets.token_hex(16), False)
            guard.sync(guard.marker[2])
        for entry in (lock, guard.activity, state, project):
            guard.sync(entry[2])
        guard.check()
        success = True
        return guard
    except OSError:
        raise _failure() from None
    finally:
        if not success:
            guard._close_files()


def shared(root):
    return _acquire(root, False)


def exclusive(root):
    return _acquire(root, True)
