// md5
package internal

import (
	"crypto/md5"
	"fmt"
	"io"
)

func copyAndMd5(w io.Writer, r io.Reader) (string, error) {
	buf := make([]byte, 10240)
	var err, inErr error
	var n int
	h := md5.New()

	for {
		n, err = r.Read(buf)
		if n > 0 {
			_, inErr = w.Write(buf[:n])
			if inErr != nil {
				return "", inErr
			}
			_, inErr = h.Write(buf[:n])
			if inErr != nil {
				return "", inErr
			}
		}

		if err == io.EOF {
			break
		} else if err != nil {
			return "", err
		}
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
