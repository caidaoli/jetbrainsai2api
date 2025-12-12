package main

import (
	"testing"
)

func TestOpenAIToJetbrainsMessages_MultipleTextContent(t *testing.T) {
	messages := []ChatMessage{
		{
			Role: RoleUser,
			Content: []any{
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "第一条消息内容",
				},
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "第二条消息内容",
				},
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "第三条消息内容",
				},
			},
		},
	}

	result := openAIToJetbrainsMessages(messages)

	// 修复后应该生成3个独立的user_message
	expectedCount := 3
	if len(result) != expectedCount {
		t.Errorf("期望生成 %d 个消息，实际生成 %d 个", expectedCount, len(result))
	}

	// 验证每个消息都是user_message类型
	for i, msg := range result {
		if msg.Type != JetBrainsMessageTypeUser {
			t.Errorf("消息 %d 类型错误，期望 '%s'，实际 '%s'", i, JetBrainsMessageTypeUser, msg.Type)
		}
	}

	// 验证消息内容是否正确分离
	expectedContents := []string{"第一条消息内容", "第二条消息内容", "第三条消息内容"}
	for i, expectedContent := range expectedContents {
		if result[i].Content != expectedContent {
			t.Errorf("消息 %d 内容错误，期望 '%s'，实际 '%s'", i, expectedContent, result[i].Content)
		}
	}
}

func TestOpenAIToJetbrainsMessages_SingleTextContent(t *testing.T) {
	messages := []ChatMessage{
		{
			Role:    RoleUser,
			Content: "单一文本消息",
		},
	}

	result := openAIToJetbrainsMessages(messages)

	if len(result) != 1 {
		t.Errorf("期望生成 1 个消息，实际生成 %d 个", len(result))
	}

	if result[0].Type != JetBrainsMessageTypeUser {
		t.Errorf("消息类型错误，期望 '%s'，实际 '%s'", JetBrainsMessageTypeUser, result[0].Type)
	}

	if result[0].Content != "单一文本消息" {
		t.Errorf("消息内容错误，期望 '单一文本消息'，实际 '%s'", result[0].Content)
	}
}
