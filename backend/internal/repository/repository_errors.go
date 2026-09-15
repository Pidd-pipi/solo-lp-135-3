package repository

import (
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
)

// isDuplicateKeyErr 判断是否为唯一约束冲突。
// 生产库为 MySQL（错误码 1062）；测试使用 SQLite，错误信息含 UNIQUE constraint。
func isDuplicateKeyErr(err error) bool {
	if err == nil {
		return false
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "duplicate")
}
