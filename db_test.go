package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// testDB 创建测试数据库
func testDB(tb testing.TB) *convoDB {
	db, err := openDB(":memory:")
	require.NoError(tb, err)
	tb.Cleanup(func() {
		require.NoError(tb, db.Close())
	})
	return db
}

// TestConvoDB 测试对话数据库功能
func TestConvoDB(t *testing.T) {
	const testid = "df31ae23ab8b75b5643c2f846c570997edc71333"

	// 测试空列表
	t.Run("空列表", func(t *testing.T) {
		db := testDB(t)
		list, err := db.List()
		require.NoError(t, err)
		require.Empty(t, list)
	})

	// 测试保存
	t.Run("保存", func(t *testing.T) {
		db := testDB(t)

		require.NoError(t, db.Save(testid, "消息 1", "openai", "gpt-4o"))

		convo, err := db.Find("df31")
		require.NoError(t, err)
		require.Equal(t, testid, convo.ID)
		require.Equal(t, "消息 1", convo.Title)

		list, err := db.List()
		require.NoError(t, err)
		require.Len(t, list, 1)
	})

	// 测试保存无 ID
	t.Run("保存无 ID", func(t *testing.T) {
		db := testDB(t)
		require.Error(t, db.Save("", "消息 1", "openai", "gpt-4o"))
	})

	// 测试保存无消息
	t.Run("保存无消息", func(t *testing.T) {
		db := testDB(t)
		require.Error(t, db.Save(newConversationID(), "", "openai", "gpt-4o"))
	})

	// 测试更新
	t.Run("更新", func(t *testing.T) {
		db := testDB(t)

		require.NoError(t, db.Save(testid, "消息 1", "openai", "gpt-4o"))
		time.Sleep(100 * time.Millisecond)
		require.NoError(t, db.Save(testid, "消息 2", "openai", "gpt-4o"))

		convo, err := db.Find("df31")
		require.NoError(t, err)
		require.Equal(t, testid, convo.ID)
		require.Equal(t, "消息 2", convo.Title)

		list, err := db.List()
		require.NoError(t, err)
		require.Len(t, list, 1)
	})

	// 测试查找单个最新记录
	t.Run("查找单个最新记录", func(t *testing.T) {
		db := testDB(t)

		require.NoError(t, db.Save(testid, "消息 2", "openai", "gpt-4o"))

		head, err := db.FindHEAD()
		require.NoError(t, err)
		require.Equal(t, testid, head.ID)
		require.Equal(t, "消息 2", head.Title)
	})

	// 测试查找多个最新记录
	t.Run("查找多个最新记录", func(t *testing.T) {
		db := testDB(t)

		require.NoError(t, db.Save(testid, "消息 2", "openai", "gpt-4o"))
		time.Sleep(time.Millisecond * 100)
		nextConvo := newConversationID()
		require.NoError(t, db.Save(nextConvo, "另一条消息", "openai", "gpt-4o"))

		head, err := db.FindHEAD()
		require.NoError(t, err)
		require.Equal(t, nextConvo, head.ID)
		require.Equal(t, "另一条消息", head.Title)

		list, err := db.List()
		require.NoError(t, err)
		require.Len(t, list, 2)
	})

	// 测试按标题查找
	t.Run("按标题查找", func(t *testing.T) {
		db := testDB(t)

		require.NoError(t, db.Save(newConversationID(), "消息 1", "openai", "gpt-4o"))
		require.NoError(t, db.Save(testid, "消息 2", "openai", "gpt-4o"))

		convo, err := db.Find("消息 2")
		require.NoError(t, err)
		require.Equal(t, testid, convo.ID)
		require.Equal(t, "消息 2", convo.Title)
	})

	// 测试无匹配查找
	t.Run("无匹配查找", func(t *testing.T) {
		db := testDB(t)
		require.NoError(t, db.Save(testid, "消息 1", "openai", "gpt-4o"))
		_, err := db.Find("消息")
		require.ErrorIs(t, err, errNoMatches)
	})

	// 测试多个匹配查找
	t.Run("多个匹配查找", func(t *testing.T) {
		db := testDB(t)
		const testid2 = "df31ae23ab9b75b5641c2f846c571000edc71315"
		require.NoError(t, db.Save(testid, "消息 1", "openai", "gpt-4o"))
		require.NoError(t, db.Save(testid2, "消息 2", "openai", "gpt-4o"))
		_, err := db.Find("df31ae")
		require.ErrorIs(t, err, errManyMatches)
	})

	// 测试删除
	t.Run("删除", func(t *testing.T) {
		db := testDB(t)

		require.NoError(t, db.Save(testid, "消息 1", "openai", "gpt-4o"))
		require.NoError(t, db.Delete(newConversationID()))

		list, err := db.List()
		require.NoError(t, err)
		require.NotEmpty(t, list)

		for _, item := range list {
			require.NoError(t, db.Delete(item.ID))
		}

		list, err = db.List()
		require.NoError(t, err)
		require.Empty(t, list)
	})

	// 测试自动补全
	t.Run("自动补全", func(t *testing.T) {
		db := testDB(t)

		const testid1 = "fc5012d8c67073ea0a46a3c05488a0e1d87df74b"
		const title1 = "某个标题"
		const testid2 = "6c33f71694bf41a18c844a96d1f62f153e5f6f44"
		const title2 = "足球队"
		require.NoError(t, db.Save(testid1, title1, "openai", "gpt-4o"))
		require.NoError(t, db.Save(testid2, title2, "openai", "gpt-4o"))

		results, err := db.Completions("f")
		require.NoError(t, err)
		require.Equal(t, []string{
			fmt.Sprintf("%s\t%s", testid1[:sha1short], title1),
			fmt.Sprintf("%s\t%s", title2, testid2[:sha1short]),
		}, results)

		results, err = db.Completions(testid1[:8])
		require.NoError(t, err)
		require.Equal(t, []string{
			fmt.Sprintf("%s\t%s", testid1, title1),
		}, results)
	})
}
