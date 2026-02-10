package storage

import (
	"os"
)

type Writer struct {
    f *os.File
    path string 
}

func New(path string, size int64)(*Writer,error){
	f, err:= os.OpenFile(path,os.O_CREATE|os.O_RDWR,0644)
	if err!=nil{
		return nil,err
	}

	if err:=f.Truncate(size);err!=nil{
		return nil,err
	}
	return &Writer{f: f, path: path}, nil
}

func (w *Writer) WriteAt(offset int64, data []byte)error{
	_,err:=w.f.WriteAt(data,offset)
	return err
}

func (w *Writer)Close()error{
	return w.f.Close()
}
func (w *Writer) ReadAt(b []byte, off int64) (int, error) {
	// Access the underlying file directly
	return w.f.ReadAt(b, off)
}