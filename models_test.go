package main

import (
	"testing"

	"github.com/bytedance/sonic"
)

// TestFlexibleString_UnmarshalJSON 测试 FlexibleString 的 JSON 反序列化
func TestFlexibleString_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      FlexibleString
		wantError bool
	}{
		{
			name:      "简单字符串",
			input:     `"Hello, World!"`,
			want:      FlexibleString("Hello, World!"),
			wantError: false,
		},
		{
			name:      "空字符串",
			input:     `""`,
			want:      FlexibleString(""),
			wantError: false,
		},
		{
			name:      "包含特殊字符的字符串",
			input:     `"Line 1\nLine 2\tTabbed"`,
			want:      FlexibleString("Line 1\nLine 2\tTabbed"),
			wantError: false,
		},
		{
			name:      "Unicode字符串",
			input:     `"你好世界 🚀"`,
			want:      FlexibleString("你好世界 🚀"),
			wantError: false,
		},
		{
			name:      "数组格式-单个text字段",
			input:     `[{"text": "Hello from array"}]`,
			want:      FlexibleString("Hello from array"),
			wantError: false,
		},
		{
			name:      "数组格式-多个text字段",
			input:     `[{"text": "Part 1"}, {"text": " Part 2"}, {"text": " Part 3"}]`,
			want:      FlexibleString("Part 1 Part 2 Part 3"),
			wantError: false,
		},
		{
			name:      "数组格式-type+content字段",
			input:     `[{"type": "text", "content": "Content from type field"}]`,
			want:      FlexibleString("Content from type field"),
			wantError: false,
		},
		{
			name:      "数组格式-混合text和type+content",
			input:     `[{"text": "Text 1"}, {"type": "text", "content": "Text 2"}]`,
			want:      FlexibleString("Text 1Text 2"),
			wantError: false,
		},
		{
			name:      "数组格式-空数组",
			input:     `[]`,
			want:      FlexibleString(""),
			wantError: false,
		},
		{
			name:      "数组格式-包含非文本字段",
			input:     `[{"text": "Valid"}, {"other": "ignored"}]`,
			want:      FlexibleString("Valid"),
			wantError: false,
		},
		{
			name:      "数组格式-text字段非字符串",
			input:     `[{"text": 123}]`,
			want:      FlexibleString(""),
			wantError: false,
		},
		{
			name:      "数组格式-type字段错误",
			input:     `[{"type": "image", "content": "should be ignored"}]`,
			want:      FlexibleString(""),
			wantError: false,
		},
		{
			name:      "无效JSON-数字",
			input:     `123`,
			wantError: true,
		},
		{
			name:      "无效JSON-布尔值",
			input:     `true`,
			wantError: true,
		},
		{
			name:      "无效JSON-对象",
			input:     `{"key": "value"}`,
			wantError: true,
		},
		{
			name:      "无效JSON-null",
			input:     `null`,
			want:      FlexibleString(""),
			wantError: false, // sonic 将 null 解析为空字符串
		},
		{
			name:      "无效JSON-格式错误",
			input:     `{invalid json}`,
			wantError: true,
		},
		{
			name:      "数组格式-复杂嵌套",
			input:     `[{"text": "Start"}, {"type": "text", "content": " Middle"}, {"text": " End"}]`,
			want:      FlexibleString("Start Middle End"),
			wantError: false,
		},
		{
			name:      "字符串-包含JSON特殊字符",
			input:     `"Quote: \" Backslash: \\ Slash: /"`,
			want:      FlexibleString(`Quote: " Backslash: \ Slash: /`),
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got FlexibleString
			err := sonic.Unmarshal([]byte(tt.input), &got)

			if tt.wantError {
				if err == nil {
					t.Errorf("UnmarshalJSON() 期望错误，但成功了，得到结果: %q", got)
				}
				return
			}

			if err != nil {
				t.Errorf("UnmarshalJSON() 意外错误 = %v", err)
				return
			}

			if got != tt.want {
				t.Errorf("UnmarshalJSON() 结果不匹配\n得到: %q\n期望: %q", got, tt.want)
			}
		})
	}
}

// TestFlexibleString_InStruct 测试 FlexibleString 在结构体中的使用
func TestFlexibleString_InStruct(t *testing.T) {
	type TestStruct struct {
		System FlexibleString `json:"system"`
	}

	tests := []struct {
		name      string
		input     string
		want      string
		wantError bool
	}{
		{
			name:      "结构体中的字符串",
			input:     `{"system": "System prompt"}`,
			want:      "System prompt",
			wantError: false,
		},
		{
			name:      "结构体中的数组",
			input:     `{"system": [{"text": "System"}, {"text": " prompt"}]}`,
			want:      "System prompt",
			wantError: false,
		},
		{
			name:      "结构体中的空字符串",
			input:     `{"system": ""}`,
			want:      "",
			wantError: false,
		},
		{
			name:      "结构体缺少system字段",
			input:     `{}`,
			want:      "",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result TestStruct
			err := sonic.Unmarshal([]byte(tt.input), &result)

			if tt.wantError {
				if err == nil {
					t.Errorf("Unmarshal() 期望错误，但成功了")
				}
				return
			}

			if err != nil {
				t.Errorf("Unmarshal() 意外错误 = %v", err)
				return
			}

			got := string(result.System)
			if got != tt.want {
				t.Errorf("System 字段不匹配\n得到: %q\n期望: %q", got, tt.want)
			}
		})
	}
}

// TestFlexibleString_RealWorldCases 测试真实场景的用例
func TestFlexibleString_RealWorldCases(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      FlexibleString
		wantError bool
	}{
		{
			name:      "Anthropic API风格-简单文本",
			input:     `"You are a helpful assistant. Always respond in Chinese."`,
			want:      FlexibleString("You are a helpful assistant. Always respond in Chinese."),
			wantError: false,
		},
		{
			name: "Anthropic API风格-数组格式（带text字段）",
			input: `[
				{"type": "text", "text": "You are Claude, a helpful AI assistant."},
				{"type": "text", "text": " Please be concise and accurate."}
			]`,
			want:      FlexibleString("You are Claude, a helpful AI assistant. Please be concise and accurate."),
			wantError: false,
		},
		{
			name:      "OpenAI风格-长系统提示",
			input:     `"You are an AI assistant specialized in code review.\n\nYour responsibilities:\n1. Analyze code quality\n2. Identify bugs\n3. Suggest improvements"`,
			want:      FlexibleString("You are an AI assistant specialized in code review.\n\nYour responsibilities:\n1. Analyze code quality\n2. Identify bugs\n3. Suggest improvements"),
			wantError: false,
		},
		{
			name: "多语言内容",
			input: `[
				{"text": "English text. "},
				{"text": "中文文本。"},
				{"text": "日本語テキスト。"}
			]`,
			want:      FlexibleString("English text. 中文文本。日本語テキスト。"),
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got FlexibleString
			err := sonic.Unmarshal([]byte(tt.input), &got)

			if tt.wantError {
				if err == nil {
					t.Errorf("UnmarshalJSON() 期望错误，但成功了")
				}
				return
			}

			if err != nil {
				t.Errorf("UnmarshalJSON() 意外错误 = %v", err)
				return
			}

			if got != tt.want {
				t.Errorf("UnmarshalJSON() 结果不匹配\n得到: %q\n期望: %q", got, tt.want)
			}
		})
	}
}

// BenchmarkFlexibleString_String 基准测试-字符串解析
func BenchmarkFlexibleString_String(b *testing.B) {
	input := []byte(`"This is a test string"`)
	var fs FlexibleString

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sonic.Unmarshal(input, &fs)
	}
}

// BenchmarkFlexibleString_Array 基准测试-数组解析
func BenchmarkFlexibleString_Array(b *testing.B) {
	input := []byte(`[{"text": "Part 1"}, {"text": "Part 2"}, {"text": "Part 3"}]`)
	var fs FlexibleString

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sonic.Unmarshal(input, &fs)
	}
}
