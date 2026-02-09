package core

import (
	"testing"

	"github.com/bytedance/sonic"
)

func TestFlexibleString_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      FlexibleString
		wantError bool
	}{
		{"简单字符串", `"Hello, World!"`, FlexibleString("Hello, World!"), false},
		{"空字符串", `""`, FlexibleString(""), false},
		{"Unicode字符串", `"你好世界 🚀"`, FlexibleString("你好世界 🚀"), false},
		{"数组格式-单个text字段", `[{"text": "Hello from array"}]`, FlexibleString("Hello from array"), false},
		{"数组格式-多个text字段", `[{"text": "Part 1"}, {"text": " Part 2"}, {"text": " Part 3"}]`, FlexibleString("Part 1 Part 2 Part 3"), false},
		{"数组格式-type+content字段", `[{"type": "text", "content": "Content from type field"}]`, FlexibleString("Content from type field"), false},
		{"数组格式-空数组", `[]`, FlexibleString(""), false},
		{"无效JSON-数字", `123`, FlexibleString(""), true},
		{"无效JSON-布尔值", `true`, FlexibleString(""), true},
		{"无效JSON-对象", `{"key": "value"}`, FlexibleString(""), true},
		{"null", `null`, FlexibleString(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got FlexibleString
			err := sonic.Unmarshal([]byte(tt.input), &got)
			if tt.wantError {
				if err == nil {
					t.Errorf("期望错误，但成功了，得到: %q", got)
				}
				return
			}
			if err != nil {
				t.Errorf("意外错误 = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("结果不匹配\n得到: %q\n期望: %q", got, tt.want)
			}
		})
	}
}

func TestFlexibleString_InStruct(t *testing.T) {
	type TestStruct struct {
		System FlexibleString `json:"system"`
	}
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"结构体中的字符串", `{"system": "System prompt"}`, "System prompt"},
		{"结构体中的数组", `{"system": [{"text": "System"}, {"text": " prompt"}]}`, "System prompt"},
		{"结构体中的空字符串", `{"system": ""}`, ""},
		{"结构体缺少system字段", `{}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result TestStruct
			err := sonic.Unmarshal([]byte(tt.input), &result)
			if err != nil {
				t.Errorf("意外错误 = %v", err)
				return
			}
			if string(result.System) != tt.want {
				t.Errorf("System 字段不匹配\n得到: %q\n期望: %q", result.System, tt.want)
			}
		})
	}
}

func TestFlexibleString_RealWorldCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  FlexibleString
	}{
		{"Anthropic API风格-简单文本", `"You are a helpful assistant."`, FlexibleString("You are a helpful assistant.")},
		{"Anthropic API风格-数组格式", `[{"type": "text", "text": "You are Claude."}, {"type": "text", "text": " Please be concise."}]`, FlexibleString("You are Claude. Please be concise.")},
		{"多语言内容", `[{"text": "English. "},{"text": "中文。"},{"text": "日本語。"}]`, FlexibleString("English. 中文。日本語。")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got FlexibleString
			if err := sonic.Unmarshal([]byte(tt.input), &got); err != nil {
				t.Errorf("意外错误 = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("结果不匹配\n得到: %q\n期望: %q", got, tt.want)
			}
		})
	}
}

func BenchmarkFlexibleString_String(b *testing.B) {
	input := []byte(`"This is a test string"`)
	var fs FlexibleString
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sonic.Unmarshal(input, &fs)
	}
}

func BenchmarkFlexibleString_Array(b *testing.B) {
	input := []byte(`[{"text": "Part 1"}, {"text": "Part 2"}, {"text": "Part 3"}]`)
	var fs FlexibleString
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sonic.Unmarshal(input, &fs)
	}
}
