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
