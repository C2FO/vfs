package mocks

import (
	"bytes"
	"io"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/stretchr/testify/mock"
)

// NewStringFile creates a new ReadWriteFile instance that can be read from the provided string as it's contents.
func NewStringFile(data, fileName string) *ReadWriteFile {
	buffer := &bytes.Buffer{}
	file := &ReadWriteFile{
		File:          File{},
		Reader:        strings.NewReader(data),
		Writer:        buffer,
		Buffer:        buffer,
		ReaderContent: data,
	}

	// Set default expectations for file operations
	file.On("Read", mock.Anything).Return(len(data), nil)
	file.On("Write", mock.Anything).Return(len(data), nil)
	file.On("Close").Return(nil)
	file.On("Name").Return(fileName)

	return file
}

// NewMockFromFilepath creates a new ReadWriteFile instance that can read a file from the provided path.
func NewMockFromFilepath(filePath string) *ReadWriteFile {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		data = make([]byte, 0)
	}
	buffer := &bytes.Buffer{}
	file := &ReadWriteFile{
		File:          File{},
		Reader:        strings.NewReader(string(data)),
		Writer:        buffer,
		Buffer:        buffer,
		ReaderContent: string(data),
	}

	// Set default expectations for file operations
	file.On("Read", mock.Anything).Return(len(data), nil)
	file.On("Write", mock.Anything).Return(len(data), nil)
	file.On("Close").Return(nil)
	file.On("Name").Return(filepath.Base(filePath))

	return file
}

// ReadWriteFile is a custom mock which allows the consumer to assign a custom reader and writer for
// easily mocking file contents.
type ReadWriteFile struct {
	File
	Reader        io.Reader
	Writer        io.Writer
	Buffer        *bytes.Buffer
	ReaderContent string
}

// Read statisfieds io.Reader interface
func (f *ReadWriteFile) Read(p []byte) (n int, err error) {
	// Deal with mocks for potential assertions
	n, err = f.File.Read(p)
	if err != nil {
		return
	}
	return f.Reader.Read(p)
}

// Write statisfieds io.Writer interface
func (f *ReadWriteFile) Write(p []byte) (n int, err error) {
	n, err = f.File.Write(p)
	if err != nil {
		return
	}
	return f.Writer.Write(p)
}

// Content returns the data held by the ReadWriteFile struct
func (f *ReadWriteFile) Content() string {
	return f.Buffer.String()
}
