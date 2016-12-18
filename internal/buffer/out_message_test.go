package buffer

import (
	"crypto/rand"
	"fmt"
	"io"
	"reflect"
	"testing"
	"unsafe"
)

func toByteSlice(p unsafe.Pointer, n int) []byte {
	sh := reflect.SliceHeader{
		Data: uintptr(p),
		Len:  n,
		Cap:  n,
	}

	return *(*[]byte)(unsafe.Pointer(&sh))
}

// fillWithGarbage writes random data to [p, p+n).
func fillWithGarbage(p unsafe.Pointer, n int) (err error) {
	b := toByteSlice(p, n)
	_, err = io.ReadFull(rand.Reader, b)
	return
}

func randBytes(n int) (b []byte, err error) {
	b = make([]byte, n)
	_, err = io.ReadFull(rand.Reader, b)
	return
}

// findNonZero finds the offset of the first non-zero byte in [p, p+n). If
// none, it returns n.
func findNonZero(p unsafe.Pointer, n int) int {
	b := toByteSlice(p, n)
	for i, x := range b {
		if x != 0 {
			return i
		}
	}

	return n
}

func TestMemclr(t *testing.T) {
	// All sizes up to 32 bytes.
	var sizes []int
	for i := 0; i <= 32; i++ {
		sizes = append(sizes, i)
	}

	// And a few hand-chosen sizes.
	sizes = append(sizes, []int{
		39, 41, 64, 127, 128, 129,
		1<<20 - 1,
		1 << 20,
		1<<20 + 1,
	}...)

	// For each size, fill a buffer with random bytes and then zero it.
	for _, size := range sizes {
		size := size
		t.Run(fmt.Sprintf("size=%d", size), func(t *testing.T) {
			// Generate
			b, err := randBytes(size)
			if err != nil {
				t.Fatalf("randBytes: %v", err)
			}

			// Clear
			var p unsafe.Pointer
			if len(b) != 0 {
				p = unsafe.Pointer(&b[0])
			}

			memclr(p, uintptr(len(b)))

			// Check
			if i := findNonZero(p, len(b)); i != len(b) {
				t.Fatalf("non-zero byte at offset %d", i)
			}
		})
	}
}

func TestOutMessageReset(t *testing.T) {
	var om OutMessage
	h := om.OutHeader()

	const trials = 100
	for i := 0; i < trials; i++ {
		err := fillWithGarbage(unsafe.Pointer(h), int(unsafe.Sizeof(*h)))
		if err != nil {
			t.Fatalf("fillWithGarbage: %v", err)
		}

		om.Reset()
		if h.Len != 0 {
			t.Fatalf("non-zero Len %v", h.Len)
		}

		if h.Error != 0 {
			t.Fatalf("non-zero Error %v", h.Error)
		}

		if h.Unique != 0 {
			t.Fatalf("non-zero Unique %v", h.Unique)
		}
	}
}

func TestOutMessageGrow(t *testing.T) {
	var om OutMessage

	// Overwrite with garbage.
	err := fillWithGarbage(unsafe.Pointer(&om), int(unsafe.Sizeof(om)))
	if err != nil {
		t.Fatalf("fillWithGarbage: %v", err)
	}

	// Zero the header.
	om.Reset()

	// Grow to the max size. This should zero the message.
	if p := om.Grow(MaxReadSize); p == nil {
		t.Fatal("Grow returned nil")
	}

	// Check that everything has been zeroed.
	b := om.Bytes()
	for i, x := range b {
		if x != 0 {
			t.Fatalf("non-zero byte 0x%02x at offset %d", x, i)
		}
	}
}

func BenchmarkOutMessageReset(b *testing.B) {
	// A single buffer, which should fit in some level of CPU cache.
	b.Run("Single buffer", func(b *testing.B) {
		var om OutMessage
		for i := 0; i < b.N; i++ {
			om.Reset()
		}

		b.SetBytes(int64(om.offset))
	})

	// Many megabytes worth of buffers, which should defeat the CPU cache.
	b.Run("Many buffers", func(b *testing.B) {
		// The number of messages; intentionally a power of two.
		const numMessages = 128

		var oms [numMessages]OutMessage
		if s := unsafe.Sizeof(oms); s < 128<<20 {
			panic(fmt.Sprintf("Array is too small; total size: %d", s))
		}

		for i := 0; i < b.N; i++ {
			oms[i%numMessages].Reset()
		}

		b.SetBytes(int64(oms[0].offset))
	})
}

func BenchmarkOutMessageGrowShrink(b *testing.B) {
	// A single buffer, which should fit in some level of CPU cache.
	b.Run("Single buffer", func(b *testing.B) {
		var om OutMessage
		for i := 0; i < b.N; i++ {
			om.Grow(MaxReadSize)
			om.ShrinkTo(OutMessageInitialSize)
		}

		b.SetBytes(int64(MaxReadSize))
	})

	// Many megabytes worth of buffers, which should defeat the CPU cache.
	b.Run("Many buffers", func(b *testing.B) {
		// The number of messages; intentionally a power of two.
		const numMessages = 128

		var oms [numMessages]OutMessage
		if s := unsafe.Sizeof(oms); s < 128<<20 {
			panic(fmt.Sprintf("Array is too small; total size: %d", s))
		}

		for i := 0; i < b.N; i++ {
			oms[i%numMessages].Grow(MaxReadSize)
			oms[i%numMessages].ShrinkTo(OutMessageInitialSize)
		}

		b.SetBytes(int64(MaxReadSize))
	})
}
