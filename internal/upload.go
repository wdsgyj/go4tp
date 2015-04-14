// upload
package internal

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

var client = &http.Client{
	Timeout: 30 * time.Second, // timeout -> 30秒
}

func upload(key string, r io.Reader, w io.Writer) (string, error) {
	if key == "" {
		return "", errors.New("upload() can't process because key is empty")
	}

	if r == nil {
		return "", errors.New("upload() can't read from nil io.Reader")
	}

	if w == nil {
		return "", errors.New("upload() can't write to nil io.Writer")
	}

	var uploadResp *http.Response
	var imgResp *http.Response
	var rs string

	defer func() {
		if uploadResp != nil {
			uploadResp.Body.Close()
		}

		if imgResp != nil {
			imgResp.Body.Close()
		}
	}()

	// 上传图片文件
	post, err := http.NewRequest("POST", "https://api.tinypng.com/shrink", r)
	if err != nil {
		return "", err
	}
	post.Header.Add("Content-Type", "application/octet-stream")
	post.SetBasicAuth("api", key)
	uploadResp, err = client.Do(post)
	if err != nil {
		return "", err
	}

	if uploadResp.StatusCode == 201 {
		// err 遮蔽了外层同名变量
		url, err := uploadResp.Location()
		if err != nil {
			return "", err
		}
		imgResp, err = client.Get(url.String())
		if err != nil {
			return "", err
		}
		// 向 w 中写入压缩后的数据
		rs, err = copyAndMd5(w, imgResp.Body)
		if err != nil {
			return "", err
		}
	} else {
		buf := new(bytes.Buffer)
		_, err = io.Copy(buf, uploadResp.Body)
		if err != nil {
			return "", err
		} else {
			return "", errors.New(fmt.Sprintf("upload() failed! Because status code: %d reason: %s",
				uploadResp.StatusCode, buf.String()))
		}
	}

	return rs, nil
}
