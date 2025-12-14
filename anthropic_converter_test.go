package main

import (
	"strings"
	"testing"
)

// TestGenerateMessageID 测试消息ID生成
func TestGenerateMessageID(t *testing.T) {
	id := generateMessageID()

	// 验证前缀
	if !strings.HasPrefix(id, MessageIDPrefix) {
		t.Errorf("消息ID应以 '%s' 为前缀，实际: '%s'", MessageIDPrefix, id)
	}

	// 验证长度合理（前缀 + 纳秒时间戳）
	if len(id) < len(MessageIDPrefix)+10 {
		t.Errorf("消息ID长度过短: %s", id)
	}

	// 验证格式：前缀后应该是数字
	numPart := id[len(MessageIDPrefix):]
	for _, c := range numPart {
		if c < '0' || c > '9' {
			t.Errorf("消息ID数字部分包含非数字字符: %c in %s", c, id)
			break
		}
	}
}
