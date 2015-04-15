// entry
package internal

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func Main(args []string) {
	cmd := flag.NewFlagSet("tinypng", flag.ExitOnError)

	var zipFlag = cmd.String("zip", "", "输入的压缩文件，zip 格式")
	var keyFlag = cmd.String("key", "", "tinypng 的 key")
	var outFlag = cmd.String("o", "a.zip", "[可选]输出的压缩后的 zip 文件")
	var dbFlag = cmd.String("db", "img.db", "[可选]缓存结果的 sqlite 数据库文件")

	err := cmd.Parse(args)
	if err != nil {
		log.Fatalln(err)
	}

	if *keyFlag == "" {
		fmt.Println("You must input a tinypng's key to compress\n")
		cmd.PrintDefaults()
		os.Exit(1)
	}

	zipReader, err := zip.OpenReader(*zipFlag)
	if err != nil {
		fmt.Println(err, "\n")
		cmd.PrintDefaults()
		os.Exit(1)
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
	results := make(chan *result)  // 阻塞管道，因为不能多个 goroutine 同时写一个文件
	errors := []*result{}

	go func() {
		// 信号量控制
		semaphore := make(chan bool, 50)
		for _, f := range r.File {
			semaphore <- true // 如果放不进去，表明信号量已满需要等待
			go func() {
				compressOne(f, key, results, db)
				<-semaphore // 执行任务完成，释放信号量
			}()
		}
		close(semaphore) // 关闭信号量
	}()

	// 所有的写文件操作必须集中在一个 goroutine 中
	for i := 0; i < size; i++ {
		result := <-results
		writeResult(result, w, &errors)
	}
	close(results)

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

// 数据库读写锁
var rwlock = new(sync.RWMutex)

func compressOne(f *zip.File, k string, rs chan <- *result, db *sql.DB) {
	defer log.Println(f.Name, "处理完成...")
	result := new(result)
	result.name = f.Name

	// 文件夹过滤
	if result.isDir = f.FileInfo().IsDir(); result.isDir {
		rs <- result
		return
	}

	// 过滤 Mac 下的隐藏目录
	if strings.Contains(f.Name, "__MACOSX") {
		rs <- result
		return
	}

	// 过滤 svn 目录
	if strings.Contains(f.Name, ".svn") {
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
	rwlock.RLock()
	row := db.QueryRow("SELECT data FROM record WHERE md5=?;", md5Str)
	err = row.Scan(&result.data)
	rwlock.RUnlock()
	// 已经从 DB 中获取到数据
	if err == nil {
		log.Println(f.Name, "数据库命中...")
		rs <- result
		return
	}

	var count int
	rwlock.RLock()
	row = db.QueryRow("SELECT count(*) FROM record WHERE dest_md5=?;", md5Str)
	err = row.Scan(&count)
	rwlock.RUnlock()
	// 已经是压缩过的图片
	if err == nil && count > 0 {
		log.Println(f.Name, "数据库命中...已经压缩过...")
		result.data = bufReader.Bytes() // 原样返回
		rs <- result
		return
	}

	// 从网络中获取数据
	outBuf := new(bytes.Buffer)
	destMd5, err := upload(k, bufReader, outBuf)
	if err != nil {
		result.e = err
		rs <- result
		return
	}

	// 存入数据库
	data := outBuf.Bytes()
	rwlock.Lock()
	_, err = db.Exec("INSERT INTO record (md5, data, dest_md5) values(?, ?, ?);", md5Str, data, destMd5)
	rwlock.Unlock()
	if err != nil {
		// 数据库存入失败不影响大局，这里只是输出一句 Log
		log.Println(err)
	}

	// 返回
	result.data = data
	rs <- result
}
