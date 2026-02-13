package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"modernc.org/sqlite"
)

var (
	errNoMatches   = errors.New("未找到对话")     // 未找到匹配的对话
	errManyMatches = errors.New("多个对话匹配输入") // 多个对话匹配输入
)

// handleSqliteErr 处理 SQLite 错误
func handleSqliteErr(err error) error {
	sqerr := &sqlite.Error{}
	if errors.As(err, &sqerr) {
		return fmt.Errorf(
			"%w: %s",
			sqerr,
			sqlite.ErrorCodeString[sqerr.Code()],
		)
	}
	return err
}

// openDB 打开数据库连接
// ds: 数据源字符串
// 返回：对话数据库实例和错误
func openDB(ds string) (*convoDB, error) {
	db, err := sqlx.Open("sqlite", ds)
	if err != nil {
		return nil, fmt.Errorf(
			"无法创建数据库: %w",
			handleSqliteErr(err),
		)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf(
			"无法连接数据库: %w",
			handleSqliteErr(err),
		)
	}
	// 创建对话表
	if _, err := db.Exec(`
		CREATE TABLE
		  IF NOT EXISTS conversations (
		    id string NOT NULL PRIMARY KEY,
		    title string NOT NULL,
		    updated_at datetime NOT NULL DEFAULT (strftime ('%Y-%m-%d %H:%M:%f', 'now')),
		    CHECK (id <> ''),
		    CHECK (title <> '')
		  )
	`); err != nil {
		return nil, fmt.Errorf("无法迁移数据库: %w", err)
	}
	// 创建 ID 索引
	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_conv_id ON conversations (id)
	`); err != nil {
		return nil, fmt.Errorf("无法迁移数据库: %w", err)
	}
	// 创建标题索引
	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_conv_title ON conversations (title)
	`); err != nil {
		return nil, fmt.Errorf("无法迁移数据库: %w", err)
	}

	// 检查并添加 model 列
	if !hasColumn(db, "model") {
		if _, err := db.Exec(`
			ALTER TABLE conversations ADD COLUMN model string
		`); err != nil {
			return nil, fmt.Errorf("无法迁移数据库: %w", err)
		}
	}
	// 检查并添加 api 列
	if !hasColumn(db, "api") {
		if _, err := db.Exec(`
			ALTER TABLE conversations ADD COLUMN api string
		`); err != nil {
			return nil, fmt.Errorf("无法迁移数据库: %w", err)
		}
	}

	return &convoDB{db: db}, nil
}

// hasColumn 检查表中是否存在指定列
// db: 数据库连接
// col: 列名
// 返回：是否存在
func hasColumn(db *sqlx.DB, col string) bool {
	var count int
	if err := db.Get(&count, `
		SELECT count(*)
		FROM pragma_table_info('conversations') c
		WHERE c.name = $1
	`, col); err != nil {
		return false
	}
	return count > 0
}

// convoDB 对话数据库
type convoDB struct {
	db *sqlx.DB
}

// Conversation 数据库中的对话记录
type Conversation struct {
	ID        string    `db:"id"`         // 对话 ID
	Title     string    `db:"title"`      // 对话标题
	UpdatedAt time.Time `db:"updated_at"` // 更新时间
	API       *string   `db:"api"`        // API 名称
	Model     *string   `db:"model"`      // 模型名称
}

// Close 关闭数据库连接
func (c *convoDB) Close() error {
	return c.db.Close() //nolint: wrapcheck
}

// Save 保存对话记录
// id: 对话 ID
// title: 对话标题
// api: API 名称
// model: 模型名称
// 返回：错误信息
func (c *convoDB) Save(id, title, api, model string) error {
	res, err := c.db.Exec(c.db.Rebind(`
		UPDATE conversations
		SET
		  title = ?,
		  api = ?,
		  model = ?,
		  updated_at = CURRENT_TIMESTAMP
		WHERE
		  id = ?
	`), title, api, model, id)
	if err != nil {
		return fmt.Errorf("保存失败: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("保存失败: %w", err)
	}

	if rows > 0 {
		return nil
	}

	// 如果更新失败，则插入新记录
	if _, err := c.db.Exec(c.db.Rebind(`
		INSERT INTO
		  conversations (id, title, api, model)
		VALUES
		  (?, ?, ?, ?)
	`), id, title, api, model); err != nil {
		return fmt.Errorf("保存失败: %w", err)
	}

	return nil
}

// Delete 删除对话记录
// id: 对话 ID
// 返回：错误信息
func (c *convoDB) Delete(id string) error {
	if _, err := c.db.Exec(c.db.Rebind(`
		DELETE FROM conversations
		WHERE
		  id = ?
	`), id); err != nil {
		return fmt.Errorf("删除失败: %w", err)
	}
	return nil
}

// ListOlderThan 列出早于指定时间的对话
// t: 时间间隔
// 返回：对话列表和错误信息
func (c *convoDB) ListOlderThan(t time.Duration) ([]Conversation, error) {
	var convos []Conversation
	if err := c.db.Select(&convos, c.db.Rebind(`
		SELECT
		  *
		FROM
		  conversations
		WHERE
		  updated_at < ?
		`), time.Now().Add(-t)); err != nil {
		return nil, fmt.Errorf("查询早于指定时间的对话失败: %w", err)
	}
	return convos, nil
}

// FindHEAD 查找最新的对话
// 返回：对话记录和错误信息
func (c *convoDB) FindHEAD() (*Conversation, error) {
	var convo Conversation
	if err := c.db.Get(&convo, `
		SELECT
		  *
		FROM
		  conversations
		ORDER BY
		  updated_at DESC
		LIMIT
		  1
	`); err != nil {
		return nil, fmt.Errorf("查找最新对话失败: %w", err)
	}
	return &convo, nil
}

// findByExactTitle 按精确标题查找对话
// result: 结果列表
// in: 标题
// 返回：错误信息
func (c *convoDB) findByExactTitle(result *[]Conversation, in string) error {
	if err := c.db.Select(result, c.db.Rebind(`
		SELECT
		  *
		FROM
		  conversations
		WHERE
		  title = ?
	`), in); err != nil {
		return fmt.Errorf("按精确标题查找失败: %w", err)
	}
	return nil
}

// findByIDOrTitle 按 ID 或标题查找对话
// result: 结果列表
// in: ID 或标题
// 返回：错误信息
func (c *convoDB) findByIDOrTitle(result *[]Conversation, in string) error {
	if err := c.db.Select(result, c.db.Rebind(`
		SELECT
		  *
		FROM
		  conversations
		WHERE
		  id glob ?
		  OR title = ?
	`), in+"*", in); err != nil {
		return fmt.Errorf("按 ID 或标题查找失败: %w", err)
	}
	return nil
}

// Completions 获取自动补全列表
// in: 输入字符串
// 返回：补全列表和错误信息
func (c *convoDB) Completions(in string) ([]string, error) {
	var result []string
	if err := c.db.Select(&result, c.db.Rebind(`
		SELECT
		  printf (
		    '%s%c%s',
		    CASE
		      WHEN length (?) < ? THEN substr (id, 1, ?)
		      ELSE id
		    END,
		    char(9),
		    title
		  )
		FROM
		  conversations
		WHERE
		  id glob ?
		UNION
		SELECT
		  printf ("%s%c%s", title, char(9), substr (id, 1, ?))
		FROM
		  conversations
		WHERE
		  title glob ?
	`), in, sha1short, sha1short, in+"*", sha1short, in+"*"); err != nil {
		return result, fmt.Errorf("获取补全列表失败: %w", err)
	}
	return result, nil
}

// Find 查找对话
// in: ID 或标题
// 返回：对话记录和错误信息
func (c *convoDB) Find(in string) (*Conversation, error) {
	var conversations []Conversation
	var err error

	if len(in) < sha1minLen {
		err = c.findByExactTitle(&conversations, in)
	} else {
		err = c.findByIDOrTitle(&conversations, in)
	}
	if err != nil {
		return nil, fmt.Errorf("查找 %q 失败: %w", in, err)
	}

	if len(conversations) > 1 {
		return nil, fmt.Errorf("%w: %s", errManyMatches, in)
	}
	if len(conversations) == 1 {
		return &conversations[0], nil
	}
	return nil, fmt.Errorf("%w: %s", errNoMatches, in)
}

// List 列出所有对话
// 返回：对话列表和错误信息
func (c *convoDB) List() ([]Conversation, error) {
	var convos []Conversation
	if err := c.db.Select(&convos, `
		SELECT
		  *
		FROM
		  conversations
		ORDER BY
		  updated_at DESC
	`); err != nil {
		return convos, fmt.Errorf("列出对话失败: %w", err)
	}
	return convos, nil
}
