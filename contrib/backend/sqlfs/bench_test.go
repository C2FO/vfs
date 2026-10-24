package sqlfs_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkSmallFileRoundTrip(b *testing.B) {
	fsys := newSQLiteFS(b)
	content := pattern(1024)
	b.SetBytes(int64(len(content)))
	for b.Loop() {
		put(b, fsys, "/small.bin", content)
		_ = get(b, fsys, "/small.bin")
	}
}

func BenchmarkSequentialLargeFile(b *testing.B) {
	fsys := newSQLiteFS(b)
	content := pattern(16 << 20)
	b.SetBytes(int64(len(content)))
	for b.Loop() {
		put(b, fsys, "/large.bin", content)
		f, err := fsys.NewFile(testNS, "/large.bin")
		require.NoError(b, err)
		n, err := io.Copy(io.Discard, f)
		require.NoError(b, err)
		require.Equal(b, int64(len(content)), n)
		require.NoError(b, f.Close())
	}
}

func BenchmarkRandomSeekRead(b *testing.B) {
	fsys := newSQLiteFS(b)
	content := pattern(16 << 20)
	put(b, fsys, "/random.bin", content)
	f, err := fsys.NewFile(testNS, "/random.bin")
	require.NoError(b, err)
	b.Cleanup(func() { _ = f.Close() })

	buf := make([]byte, 4096)
	offset := int64(0)
	for b.Loop() {
		// Stride by a prime so reads land in different chunks.
		offset = (offset + 1_000_003) % int64(len(content)-len(buf))
		_, err := f.Seek(offset, io.SeekStart)
		require.NoError(b, err)
		_, err = io.ReadFull(f, buf)
		require.NoError(b, err)
		require.True(b, bytes.Equal(content[offset:offset+int64(len(buf))], buf))
	}
}

func BenchmarkSameInstanceCopy(b *testing.B) {
	fsys := newSQLiteFS(b)
	put(b, fsys, "/src.bin", pattern(16<<20))
	for b.Loop() {
		src, err := fsys.NewFile(testNS, "/src.bin")
		require.NoError(b, err)
		dst, err := fsys.NewFile(testNS, "/dst.bin")
		require.NoError(b, err)
		require.NoError(b, src.CopyToFile(dst))
	}
}
