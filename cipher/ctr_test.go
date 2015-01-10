package cipher

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"github.com/phylake/go-crypto"
	"github.com/stretchr/testify/assert"
	"io"
	"io/ioutil"
	"math"
	"testing"
)

func Test_CTR_Bijection(t *testing.T) {
	t.Parallel()

	plaintext1 := make([]byte, 123)
	_, err := io.ReadFull(rand.Reader, plaintext1)
	assert.Nil(t, err)

	randomKey, err := crypto.RandomAES256Key()
	assert.Nil(t, err)

	rBuf := bytes.NewBuffer(plaintext1)
	ctrReader, err := NewCTRReader(randomKey, rBuf)
	assert.Nil(t, err)

	var wBuf bytes.Buffer
	ctrWriter := NewCTRWriter(randomKey, &wBuf)

	bufioWriter := bufio.NewWriter(ctrWriter)
	bufioWriter.ReadFrom(ctrReader)
	bufioWriter.Flush()

	plaintext2 := wBuf.Bytes()

	assert.Equal(t, plaintext1, plaintext2)
}

func TestCTRExampleAndCTRReaderProduceSameResult(t *testing.T) {
	t.Parallel()

	key := []byte("example key 1234")
	plaintext := []byte("some plaintext")

	block, err := aes.NewCipher(key)
	assert.Nil(t, err)

	ciphertext1 := make([]byte, aes.BlockSize+len(plaintext))
	ciphertext2 := make([]byte, aes.BlockSize+len(plaintext))
	iv := ciphertext1[:aes.BlockSize]
	_, err = io.ReadFull(rand.Reader, iv)
	assert.Nil(t, err)

	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(ciphertext1[aes.BlockSize:], plaintext)

	buffer := bytes.NewBuffer(plaintext)
	readCloser := ioutil.NopCloser(buffer)
	ctrReader, err := newCTRReaderWithVector(key, readCloser, iv)
	assert.Nil(t, err)
	io.ReadFull(ctrReader, ciphertext2)

	assert.Equal(t, ciphertext1, ciphertext2)
}

//------------------------------------------------------------------------------
// Testing assumptions about CTR below this line
//------------------------------------------------------------------------------

func TestCTRExample(t *testing.T) {
	t.Parallel()

	key := []byte("example key 1234")
	plaintext := []byte("some plaintext")

	block, err := aes.NewCipher(key)
	assert.Nil(t, err)

	ciphertext := make([]byte, aes.BlockSize+len(plaintext))
	iv := ciphertext[:aes.BlockSize]
	_, err = io.ReadFull(rand.Reader, iv)
	assert.Nil(t, err)

	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], plaintext)

	plaintext2 := make([]byte, len(plaintext))
	stream = cipher.NewCTR(block, iv)
	stream.XORKeyStream(plaintext2, ciphertext[aes.BlockSize:])

	assert.Equal(t, plaintext, plaintext2)
}

// I wasn't sure if multiple calls to XORKeyStream where
// len(dst) < aes.BlockSize would work
func TestCTRHandlesMultipleByteSlicesSmallerThanAESBlockSize(t *testing.T) {
	t.Parallel()

	key := []byte("example key 1234")
	plaintext := []byte("some plaintext")

	block, _ := aes.NewCipher(key)

	ciphertext1 := make([]byte, aes.BlockSize+len(plaintext))
	iv := ciphertext1[:aes.BlockSize]
	io.ReadFull(rand.Reader, iv)
	ciphertext2 := dup(ciphertext1)

	stream1 := cipher.NewCTR(block, iv)
	stream1.XORKeyStream(ciphertext1[aes.BlockSize:], plaintext)

	mid := 6
	stream2 := cipher.NewCTR(block, iv)
	stream2.XORKeyStream(ciphertext2[aes.BlockSize:aes.BlockSize+mid], plaintext[:mid])
	stream2.XORKeyStream(ciphertext2[aes.BlockSize+mid:], plaintext[mid:])

	assert.Equal(t, ciphertext1, ciphertext2)
}

// the reason to use CTR instead of CBC is it's stream-based so you don't need
// in memory the entire message to be encrypted, making it more suitable for
// large files and easier to fit with an io.Reader pipeline.
func TestIncrementalCTR(t *testing.T) {
	t.Parallel()

	key := []byte("example key 1234")
	plainTextSngl := []byte("some text that's longer than aes.BlockSize and not a multiple of aes.BlockSize")
	plainTextIncr := []byte("some text that's longer than aes.BlockSize and not a multiple of aes.BlockSize")

	blockSngl, _ := aes.NewCipher(key)
	blockIncr, _ := aes.NewCipher(key)
	assert.Equal(t, blockIncr, blockSngl)

	ciphertextSngl := make([]byte, aes.BlockSize+len(plainTextSngl))
	ivSngl := ciphertextSngl[:aes.BlockSize]
	io.ReadFull(rand.Reader, ivSngl)
	ivIncr := dup(ivSngl)

	streamSngl := cipher.NewCTR(blockSngl, ivSngl)
	streamSngl.XORKeyStream(ciphertextSngl[aes.BlockSize:], plainTextSngl)

	streamIncr := cipher.NewCTR(blockIncr, ivIncr)

	ciphertextIncr := make([]byte, 0)
	ciphertextIncr = append(ivIncr, ciphertextIncr...)

	pLen := float64(len(plainTextIncr))
	blocks := int(math.Ceil(pLen / float64(aes.BlockSize)))
	assert.Equal(t, 5, blocks)

	for i := 0; i < blocks; i++ {
		beg := int(math.Min(pLen, float64(aes.BlockSize*(i+0))))
		end := int(math.Min(pLen, float64(aes.BlockSize*(i+1))))
		dst := make([]byte, end-beg)
		src := plainTextIncr[beg:end] // will be a io.Reader.Read(src)

		// the point of the test: to see if multiple calls to XORKeyStream
		// work the same as a single call
		streamIncr.XORKeyStream(dst, src)
		ciphertextIncr = append(ciphertextIncr, dst...)
	}

	assert.Equal(t, ciphertextSngl, ciphertextIncr)
}

func dup(p []byte) []byte {
	q := make([]byte, len(p))
	copy(q, p)
	return q
}