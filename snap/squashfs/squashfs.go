// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2014-2018 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package squashfs

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/logger"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/snapdtool"
)

const (
	// https://github.com/plougher/squashfs-tools/blob/master/squashfs-tools/squashfs_fs.h#L289
	superblockSize = 96
)

var (
	// magic is the magic prefix of squashfs snap files.
	magic = []byte{'h', 's', 'q', 's'}

	// for testing
	isRootWritableOverlay = osutil.IsRootWritableOverlay
)

func FileHasSquashfsHeader(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	// a squashfs file would contain at least the superblock + some data
	header := make([]byte, superblockSize+1)
	if _, err := f.ReadAt(header, 0); err != nil {
		return false
	}

	return bytes.HasPrefix(header, magic)
}

// Snap is the squashfs based snap.
type Snap struct {
	path string
}

// Path returns the path of the backing file.
func (s *Snap) Path() string {
	return s.path
}

// New returns a new Squashfs snap.
func New(snapPath string) *Snap {
	return &Snap{path: snapPath}
}

var osLink = os.Link
var snapdtoolCommandFromSystemSnap = snapdtool.CommandFromSystemSnap

type linkFunc = func(string, string) error

var errLinkError = errors.New("linking error")

func tryLinkWithIntegrityData(link linkFunc, snapPath, targetPath string, opts *snap.InstallOptions) (retErr error) {
	if err := link(snapPath, targetPath); err != nil {
		// Specifically when link(2) is used, it returns EPERM on filesystems that don't
		// support hard links (like vfat), so checking the error here doesn't
		// make sense vs just trying to copy it.
		//
		// we use a specific error type here to allow the calling code to detect
		// generic linking errors and ignore them to allow the code to fall-through
		// and use a different linking or copying method.
		return errLinkError
	}

	defer func() {
		if retErr != nil {
			// unlink the snap if something below failed
			if err := os.Remove(targetPath); err != nil {
				logger.Noticef("cannot remove %q: %v", targetPath, err)
			}
		}
	}()

	if opts != nil && opts.IntegrityDataParams != nil {
		srcIntegrityFile, err := opts.IntegrityDataParams.IntegrityFile(snapPath)
		if err != nil {
			return err
		}
		destIntegrityFile, err := opts.IntegrityDataParams.IntegrityFile(targetPath)
		if err != nil {
			return err
		}
		if err := link(srcIntegrityFile, destIntegrityFile); err != nil {
			return err
		}
	}

	return nil
}

func tryCopyWithIntegrityData(snapPath, targetPath string, opts *snap.InstallOptions) (retErr error) {
	if err := osutil.CopyFile(snapPath, targetPath, osutil.CopyFlagPreserveAll|osutil.CopyFlagSync); err != nil {
		return err
	}

	defer func() {
		if retErr != nil {
			// remove the copy of the snap if something below failed
			if err := os.Remove(targetPath); err != nil {
				logger.Noticef("cannot remove %q: %v", targetPath, err)
			}
		}
	}()

	if opts != nil && opts.IntegrityDataParams != nil {
		srcIntegrityFile, err := opts.IntegrityDataParams.IntegrityFile(snapPath)
		if err != nil {
			return err
		}
		destIntegrityFile, err := opts.IntegrityDataParams.IntegrityFile(targetPath)
		if err != nil {
			return err
		}
		if err := osutil.CopyFile(srcIntegrityFile, destIntegrityFile, osutil.CopyFlagPreserveAll|osutil.CopyFlagSync); err != nil {
			return err
		}
	}

	return nil
}

// Install installs a squashfs snap file through an appropriate method.
func (s *Snap) Install(targetPath, mountDir string, opts *snap.InstallOptions) (bool, error) {

	// ensure mount-point and blob target dir.
	for _, dir := range []string{mountDir, filepath.Dir(targetPath)} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return false, err
		}
	}

	// This is required so that the tests can simulate a mounted
	// snap when we "install" a squashfs snap in the tests.
	// We can not mount it for real in the tests, so we just unpack
	// it to the location which is good enough for the tests.
	if osutil.GetenvBool("SNAPPY_SQUASHFS_UNPACK_FOR_TESTS") {
		if err := s.Unpack("*", mountDir); err != nil {
			return false, err
		}
	}

	// nothing to do, happens on e.g. first-boot when we already
	// booted with the OS snap but its also in the seed.yaml
	if s.path == targetPath || osutil.FilesAreEqual(s.path, targetPath) {
		didNothing := true
		return didNothing, nil
	}

	overlayRoot, err := isRootWritableOverlay()
	if err != nil {
		logger.Noticef("cannot detect root filesystem on overlay: %v", err)
	}
	// Hard-linking on overlayfs is identical to a full blown
	// copy. When we are operating on a overlayfs based system (e.g. live
	// installer) use symbolic links.
	// https://bugs.launchpad.net/snapd/+bug/1867415
	if overlayRoot == "" {
		// try to (hard)link the file, but go on to trying to copy it
		// if it fails for whatever reason
		err := tryLinkWithIntegrityData(osLink, s.path, targetPath, opts)
		if err == nil {
			// Success, no need to do the copy
			return false, nil
		}
		if !errors.Is(err, errLinkError) {
			return false, err
		}
	}

	// if the installation must not cross devices, then we should not use
	// symlinks and instead must copy the file entirely, this is the case
	// during seeding on uc20 in run mode for example
	if opts == nil || !opts.MustNotCrossDevices {
		// if the source snap file is in seed, but the hardlink failed, symlinking
		// it saves the copy (which in livecd is expensive) so try that next
		// note that on UC20, the snap file could be in a deep subdir of
		// SnapSeedDir, i.e. /var/lib/snapd/seed/systems/20200521/snaps/<name>.snap
		// so we need to check if it has the prefix of the seed dir
		cleanSrc := filepath.Clean(s.path)
		if strings.HasPrefix(cleanSrc, dirs.SnapSeedDir) {
			err := tryLinkWithIntegrityData(os.Symlink, s.path, targetPath, opts)
			if err == nil {
				// Success, no need to do the copy
				return false, nil
			}
			if !errors.Is(err, errLinkError) {
				return false, err
			}
		}
	}

	return false, tryCopyWithIntegrityData(s.path, targetPath, opts)
}

// Unpack unpacks files from the snap to the given directory.
// src is a path or glob pattern (e.g. "*" for everything).
//
// Extended attributes are not preserved.
func (s *Snap) Unpack(src, dstDir string) error {
	r, closer, err := newNativeReader(s.path)
	if err != nil {
		return fmt.Errorf("cannot open snap %q: %v", s.path, err)
	}
	defer closer()

	if src == "*" || src == "." {
		return r.extractAll(dstDir)
	}
	return r.extractMatching(src, dstDir)
}

// Size returns the size of the backing file.
func (s *Snap) Size() (size int64, err error) {
	st, err := os.Stat(s.path)
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}


// RandomAccessFile returns an implementation to read at any given
// location for a single file inside the squashfs snap plus
// information about the file size.
func (s *Snap) RandomAccessFile(filePath string) (interface {
	io.ReaderAt
	io.Closer
	Size() int64
}, error) {
	r, closer, err := newNativeReader(s.path)
	if err != nil {
		return nil, err
	}

	ino, err := r.resolvePath(filePath)
	if err != nil {
		closer()
		return nil, err
	}
	if !ino.IsRegular() {
		closer()
		return nil, fmt.Errorf("%q is not a regular file", filePath)
	}

	data, err := r.readFileData(ino)
	closer()
	if err != nil {
		return nil, err
	}

	return &byteSliceFile{data: data}, nil
}

// byteSliceFile implements io.ReaderAt, io.Closer, and Size() from a byte slice.
type byteSliceFile struct {
	data []byte
}

func (b *byteSliceFile) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(b.data)) {
		return 0, io.EOF
	}
	n := copy(p, b.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (b *byteSliceFile) Close() error        { return nil }
func (b *byteSliceFile) Size() int64          { return int64(len(b.data)) }

// ReadFile returns the content of a single file inside a squashfs snap.
func (s *Snap) ReadFile(filePath string) (content []byte, err error) {
	r, closer, err := newNativeReader(s.path)
	if err != nil {
		return nil, err
	}
	defer closer()

	ino, err := r.resolvePath(filePath)
	if err != nil {
		return nil, err
	}
	return r.readFileData(ino)
}

// ReadLink returns the target of a symlink inside a squashfs snap.
func (s *Snap) ReadLink(filePath string) (string, error) {
	r, closer, err := newNativeReader(s.path)
	if err != nil {
		return "", err
	}
	defer closer()

	ino, err := r.resolvePath(filePath)
	if err != nil {
		return "", err
	}
	if !ino.IsSymlink() {
		return "", fmt.Errorf("%q is not a symlink", filePath)
	}
	return ino.SymTarget, nil
}

// Lstat returns file info for a single path inside the snap.
func (s *Snap) Lstat(filePath string) (os.FileInfo, error) {
	r, closer, err := newNativeReader(s.path)
	if err != nil {
		return nil, err
	}
	defer closer()

	ino, err := r.resolvePath(filePath)
	if err != nil {
		return nil, os.ErrNotExist
	}
	return inodeToFileInfo(filepath.Base(filePath), ino), nil
}

// Walk (part of snap.Container) is like filepath.Walk, without the ordering guarantee.
func (s *Snap) Walk(relative string, walkFn filepath.WalkFunc) error {
	r, closer, err := newNativeReader(s.path)
	if err != nil {
		return walkFn(relative, nil, err)
	}
	defer closer()

	relative = filepath.Clean(relative)
	if relative == "" || relative == "/" || relative == "." {
		ino, err := r.readInode(r.sb.RootInodeRef)
		if err != nil {
			return walkFn(".", nil, err)
		}
		st := inodeToFileInfo(".", ino)
		if err := walkFn(".", st, nil); err != nil {
			return err
		}
		return r.walkDir(r.sb.RootInodeRef, ".", walkFn)
	}
	if relative[0] == '/' {
		relative = relative[1:]
	}

	ino, err := r.resolvePath(relative)
	if err != nil {
		return walkFn(relative, nil, os.ErrNotExist)
	}
	if !ino.IsDir() {
		return walkFn(relative, nil, fmt.Errorf("not a directory"))
	}

	return r.walkDirInode(ino, relative, walkFn, 0)
}

// ListDir returns the content of a single directory inside a squashfs snap.
func (s *Snap) ListDir(dirPath string) ([]string, error) {
	r, closer, err := newNativeReader(s.path)
	if err != nil {
		return nil, err
	}
	defer closer()

	ino, err := r.resolvePath(dirPath)
	if err != nil {
		return nil, err
	}
	if !ino.IsDir() {
		return nil, fmt.Errorf("%q is not a directory", dirPath)
	}

	entries, err := r.readDir(ino.DirStart, ino.DirOffset, ino.DirSize)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, e := range entries {
		names = append(names, e.Name)
	}
	return names, nil
}

const maxErrPaths = 10

type errPathsNotReadable struct {
	paths []string
}

func (e *errPathsNotReadable) accumulate(p string, fi os.FileInfo) error {
	if len(e.paths) >= maxErrPaths {
		return e
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		e.paths = append(e.paths, fmt.Sprintf("%s (owner %v:%v mode %#03o)", p, st.Uid, st.Gid, fi.Mode().Perm()))
	} else {
		e.paths = append(e.paths, p)
	}
	return nil
}

func (e *errPathsNotReadable) asErr() error {
	if len(e.paths) > 0 {
		return e
	}
	return nil
}

func (e *errPathsNotReadable) Error() string {
	var b bytes.Buffer

	b.WriteString("cannot access the following locations in the snap source directory:\n")
	for _, p := range e.paths {
		fmt.Fprintf(&b, "- %s\n", p)
	}
	if len(e.paths) == maxErrPaths {
		fmt.Fprintf(&b, "- too many errors, listing first %v entries\n", maxErrPaths)
	}
	return b.String()
}

// verifyContentAccessibleForBuild checks whether the content under source
// directory is usable to the user and can be represented by mksquashfs.
func verifyContentAccessibleForBuild(sourceDir string) error {
	var errPaths errPathsNotReadable

	withSlash := filepath.Clean(sourceDir) + "/"
	err := filepath.Walk(withSlash, func(path string, st os.FileInfo, err error) error {
		if err != nil {
			if !os.IsPermission(err) {
				return err
			}
			// accumulate permission errors
			return errPaths.accumulate(strings.TrimPrefix(path, withSlash), st)
		}
		mode := st.Mode()
		if !mode.IsRegular() && !mode.IsDir() {
			// device nodes are just recreated by mksquashfs
			return nil
		}
		if mode.IsRegular() && st.Size() == 0 {
			// empty files are also recreated
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			if !os.IsPermission(err) {
				return err
			}
			// accumulate permission errors
			if err = errPaths.accumulate(strings.TrimPrefix(path, withSlash), st); err != nil {
				return err
			}
			// workaround for https://github.com/golang/go/issues/21758
			// with pre 1.10 go, explicitly skip directory
			if mode.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		f.Close()
		return nil
	})
	if err != nil {
		return err
	}
	return errPaths.asErr()
}

type MksquashfsError struct {
	msg string
}

func (m MksquashfsError) Error() string {
	return m.msg
}

type BuildOpts struct {
	SnapType     string
	Compression  string
	ExcludeFiles []string
}

// MinimumSnapSize is the smallest size a snap can be. The kernel attempts to read a
// partition table from the snap when a loopback device is created from it. If the snap
// is smaller than this size, some versions of the kernel will print error logs while
// scanning the loopback device for partitions.
// TODO: revisit if necessary, some distros (eg. openSUSE) patch squashfs-tools to pad to 64k but
// kernel should work with this
const MinimumSnapSize int64 = 16384

// Build builds the snap.
func (s *Snap) Build(sourceDir string, opts *BuildOpts) error {
	if opts == nil {
		opts = &BuildOpts{}
	}
	if err := verifyContentAccessibleForBuild(sourceDir); err != nil {
		return err
	}

	fullSnapPath, err := filepath.Abs(s.path)
	if err != nil {
		return err
	}
	// default to xz
	compression := opts.Compression
	if compression == "" {
		// TODO: support other compression options, xz is very
		// slow for certain apps, see
		// https://forum.snapcraft.io/t/squashfs-performance-effect-on-snap-startup-time/13920
		compression = "xz"
	}
	cmd, err := snapdtoolCommandFromSystemSnap("/usr/bin/mksquashfs")
	if err != nil {
		cmd = exec.Command("mksquashfs")
	}
	cmd.Args = append(cmd.Args,
		".", fullSnapPath,
		"-noappend",
		"-comp", compression,
		"-no-fragments",
		"-no-progress",
	)

	if len(opts.ExcludeFiles) > 0 {
		cmd.Args = append(cmd.Args, "-wildcards")
		for _, excludeFile := range opts.ExcludeFiles {
			cmd.Args = append(cmd.Args, "-ef", excludeFile)
		}
	}
	snapType := opts.SnapType
	switch snapType {
	case "os", "core", "base", "snapd":
		// -xattrs is default, but let's be explicit about it
		cmd.Args = append(cmd.Args, "-xattrs")
	default:
		cmd.Args = append(cmd.Args, "-all-root", "-no-xattrs")
	}

	err = osutil.ChDir(sourceDir, func() error {
		output, err := cmd.CombinedOutput()
		if err != nil {
			return MksquashfsError{fmt.Sprintf("mksquashfs call failed: %s", osutil.OutputErr(output, err))}
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Grow the snap if it is smaller than the minimum snap size. See
	// MinimumSnapSize for more details.
	return growSnapToMinSize(s.path, MinimumSnapSize)
}

// BuildDate returns the modification time from the squashfs superblock.
func (s *Snap) BuildDate() time.Time {
	return BuildDate(s.path)
}

// BuildDate returns the modification time from the squashfs superblock.
func BuildDate(path string) time.Time {
	r, closer, err := newNativeReader(path)
	if err != nil {
		return time.Time{}
	}
	closer()
	return r.ModTime()
}
