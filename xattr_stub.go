package main

import (
	"context"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
)

// xattrStub implements fast extended attribute handlers that return "no attributes"
// immediately without any I/O. This prevents macOS Spotlight and Finder from triggering
// expensive operations on the FUSE filesystem.
//
// Without these stubs, the kernel may call getxattr/listxattr on every file during
// Spotlight indexing, causing unnecessary FUSE round-trips and CPU spikes.
var xattrStubInstance = xattrStub{}

type xattrStub struct{}

// Listxattr returns ENOTSUP to signal "no extended attributes" without any I/O.
func (x *xattrStub) Listxattr(ctx context.Context, dest []byte) (uint32, syscall.Errno) {
	return 0, syscall.ENOTSUP
}

// Getxattr returns ENOTSUP to signal "attribute not available" without any I/O.
func (x *xattrStub) Getxattr(ctx context.Context, attr string, dest []byte) (uint32, syscall.Errno) {
	return 0, syscall.ENOTSUP
}

// Compile-time interface checks for all FUSE node types that need xattr stubs.
var _ = (fs.NodeListxattrer)((*VirtualMkvRoot)(nil))
var _ = (fs.NodeGetxattrer)((*VirtualMkvRoot)(nil))
var _ = (fs.NodeListxattrer)((*VirtualDirNode)(nil))
var _ = (fs.NodeGetxattrer)((*VirtualDirNode)(nil))
var _ = (fs.NodeListxattrer)((*VirtualMkvNode)(nil))
var _ = (fs.NodeGetxattrer)((*VirtualMkvNode)(nil))

// V463: NodeAccesser stubs — Finder calls access() before opening files to check permissions.
// Without this, the kernel falls back to expensive Getattr + permission check.
var _ = (fs.NodeAccesser)((*VirtualMkvRoot)(nil))
var _ = (fs.NodeAccesser)((*VirtualDirNode)(nil))
var _ = (fs.NodeAccesser)((*VirtualMkvNode)(nil))

// VirtualMkvRoot xattr stubs.
func (r *VirtualMkvRoot) Listxattr(ctx context.Context, dest []byte) (uint32, syscall.Errno) {
	return xattrStubInstance.Listxattr(ctx, dest)
}

func (r *VirtualMkvRoot) Getxattr(ctx context.Context, attr string, dest []byte) (uint32, syscall.Errno) {
	return xattrStubInstance.Getxattr(ctx, attr, dest)
}

// VirtualDirNode xattr stubs.
func (d *VirtualDirNode) Listxattr(ctx context.Context, dest []byte) (uint32, syscall.Errno) {
	return xattrStubInstance.Listxattr(ctx, dest)
}

func (d *VirtualDirNode) Getxattr(ctx context.Context, attr string, dest []byte) (uint32, syscall.Errno) {
	return xattrStubInstance.Getxattr(ctx, attr, dest)
}

// VirtualMkvNode xattr stubs.
func (n *VirtualMkvNode) Listxattr(ctx context.Context, dest []byte) (uint32, syscall.Errno) {
	return xattrStubInstance.Listxattr(ctx, dest)
}

func (n *VirtualMkvNode) Getxattr(ctx context.Context, attr string, dest []byte) (uint32, syscall.Errno) {
	return xattrStubInstance.Getxattr(ctx, attr, dest)
}

// V463: NodeAccesser stubs — return 0 (OK) immediately without any I/O.
// Finder calls access() to check read/write/execute permissions before opening files.
// Returning 0 tells the kernel "permission granted" without triggering Getattr.
func (r *VirtualMkvRoot) Access(ctx context.Context, mask uint32) syscall.Errno {
	return 0
}

func (d *VirtualDirNode) Access(ctx context.Context, mask uint32) syscall.Errno {
	return 0
}

func (n *VirtualMkvNode) Access(ctx context.Context, mask uint32) syscall.Errno {
	return 0
}
