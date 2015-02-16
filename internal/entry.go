// entry
package internal

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func Main(args []string) {
	var zipFlag = stringFlag("zip", "", "输入的压缩文件，zip 格式")
	var keyFlag = stringFlag("key", "", "tinypng 的 key")
	var outFlag = stringFlag("o", "a.zip", "[可选]输出的压缩后的 zip 文件")
	var dbFlag = stringFlag("db", "img.db", "[可选]缓存结果的 sqlite 数据库文件")

	err := parse(args)
	if err != nil {
		log.Fatalln(err)
	}

	if *keyFlag == "" {
		log.Fatalln("You must input a tinypng's key to compress")
	}

	zipReader, err := zip.OpenReader(*zipFlag)
	if err != nil {
		log.Fatalln(err)
	}
	defer zipReader.Close()

	fileOut, err := os.Create(*outFlag)
	if err != nil {
		log.Fatalln(err)
	}
	defer fileOut.Close()

	zipWriter := zip.NewWriter(fileOut)
	defer zipWriter.Close()

	db, err := sql.Open("sqlite3", *dbFlag)
	if err != nil {
		log.Fatalln(err)
	}
	err = createRecordTable(db)
	if err != nil {
		log.Fatalln(err)
	}

	compressAll(zipReader, zipWriter, *keyFlag, db)
}

type result struct {
	isDir bool
	name  string
	data  []byte
	e     error
}

func compressAll(r *zip.ReadCloser, w *zip.Writer, key string, db *sql.DB) {
	// 调节 OS 的线程数量
	runtime.GOMAXPROCS(runtime.NumCPU() * 4)

	size := len(r.File)
	results := make(chan *result)
	errors := []*result{}

	for _, f := range r.File {
		go compressOne(f, key, results, db)
	}

	for i := 0; i < size; i++ {
		result := <-results
		writeResult(result, w, &errors)
	}

	writeErrorResult(w, errors)
	log.Println("全部处理完成!")
}

func writeErrorResult(w *zip.Writer, es []*result) error {
	if len(es) == 0 {
		return nil
	}

	buf := new(bytes.Buffer)
	for i, e := range es {
		fmt.Fprintf(buf, "(%d) %s: %v\n", i, e.name, e.e)
	}
	writer, err := w.Create("errors_in_compress.txt")
	if err != nil {
		return err
	}

	_, err = io.Copy(writer, buf)
	if err != nil {
		return err
	}
	return nil
}

func writeResult(r *result, w *zip.Writer, es *[]*result) {
	if r.name == "" {
		panic("name must not be empty!")
	}

	if r.e != nil {
		*es = append(*es, r)
		return
	}

	if !r.isDir {
		log.Println("开始写入", r.name)
		writer, err := w.Create(r.name)
		if err != nil {
			r.e = err
			*es = append(*es, r)
			return
		}
		_, err = io.Copy(writer, bytes.NewBuffer(r.data))
		if err != nil {
			r.e = err
			*es = append(*es, r)
			return
		}
	}
}

func compressOne(f *zip.File, k string, rs chan<- *result, db *sql.DB) {
	defer log.Println(f.Name, "处理完成...")
	result := new(result)
	result.name = f.Name
	if result.isDir = f.FileInfo().IsDir(); result.isDir {
		rs <- result
		return
	}

	rawReader, err := f.Open()
	if err != nil {
		result.e = err
		rs <- result
		return
	}
	defer rawReader.Close()

	bufReader := new(bytes.Buffer)
	md5Str, err := copyAndMd5(bufReader, rawReader)
	if err != nil {
		result.e = err
		rs <- result
		return
	}

	// 过滤非 png 和 jpg 图片
	lowername := strings.ToLower(result.name)
	if !strings.HasSuffix(lowername, ".png") &&
		!strings.HasSuffix(lowername, ".jpg") {
		result.data = bufReader.Bytes() // 原样返回
		rs <- result
		return
	}

	// 查找数据库
	row := db.QueryRow("SELECT data FROM record WHERE md5=?;", md5Str)
	err = row.Scan(&result.data)
	// 已经从 DB 中获取到数据
	if err == nil {
		log.Println(f.Name, "数据库命中...")
		rs <- result
		return
	}

	// 从网络中获取数据
	outBuf := new(bytes.Buffer)
	err = upload(k, bufReader, outBuf)
	if err != nil {
		result.e = err
		rs <- result
		return
	}

	// 存入数据库
	data := outBuf.Bytes()
	_, err = db.Exec("INSERT INTO record (md5, data) values(?, ?);", md5Str, data)
	if err != nil {
		// 数据库存入失败不影响大局，这里只是输出一句 Log
		log.Println(err)
	}

	// 返回
	result.data = data
	rs <- result
}
