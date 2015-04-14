// db
package internal

import (
	"database/sql"
)

// sqlite 的工具方法
//func isTableExist(db *sql.DB, name string) (bool, error) {
//	row := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name=? AND type='table';",
//		name)
//	count := 0
//	err := row.Scan(&count)
//	if err != nil {
//		return false, err
//	}
//	return count > 0, nil
//}

func createRecordTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS record (
		id INTEGER,
		md5 TEXT,
		data BLOB,
		dest_md5 TEXT,
		PRIMARY KEY (id, md5) ON CONFLICT IGNORE);`)

	if err != nil {
		return err
	}
	return nil
}
