package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

// TestNewImageValidator 测试创建图像验证器
func TestNewImageValidator(t *testing.T) {
	validator := NewImageValidator()

	if validator == nil {
		t.Fatal("NewImageValidator 不应返回 nil")
	}

	// 验证常量配置正确 (不再是结构体字段，而是直接使用全局常量)
	if MaxImageSizeBytes <= 0 {
		t.Error("MaxImageSizeBytes 常量应大于0")
	}

	if len(SupportedImageFormats) == 0 {
		t.Error("SupportedImageFormats 常量不应为空")
	}
}

// TestValidateImageData 测试图像数据验证
func TestValidateImageData(t *testing.T) {
	validator := NewImageValidator()

	// 创建一个小的有效 base64 图像数据
	smallData := base64.StdEncoding.EncodeToString([]byte("small test image data"))

	// 创建一个大的 base64 数据（超过限制）
	largeBytes := make([]byte, MaxImageSizeBytes+1000)
	for i := range largeBytes {
		largeBytes[i] = 'A'
	}
	largeData := base64.StdEncoding.EncodeToString(largeBytes)

	tests := []struct {
		name      string
		mediaType string
		data      string
		expectErr bool
		errMsg    string
	}{
		{
			name:      "有效PNG图像",
			mediaType: "image/png",
			data:      smallData,
			expectErr: false,
		},
		{
			name:      "有效JPEG图像",
			mediaType: "image/jpeg",
			data:      smallData,
			expectErr: false,
		},
		{
			name:      "有效GIF图像",
			mediaType: "image/gif",
			data:      smallData,
			expectErr: false,
		},
		{
			name:      "有效WebP图像",
			mediaType: "image/webp",
			data:      smallData,
			expectErr: false,
		},
		{
			name:      "不支持的格式",
			mediaType: "image/bmp",
			data:      smallData,
			expectErr: true,
			errMsg:    "unsupported image format",
		},
		{
			name:      "无效base64数据",
			mediaType: "image/png",
			data:      "not-valid-base64!!!",
			expectErr: true,
			errMsg:    "invalid base64",
		},
		{
			name:      "超大图像",
			mediaType: "image/png",
			data:      largeData,
			expectErr: true,
			errMsg:    "exceeds maximum allowed size",
		},
		{
			name:      "大小写不敏感的格式",
			mediaType: "IMAGE/PNG",
			data:      smallData,
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateImageData(tt.mediaType, tt.data)

			if tt.expectErr {
				if err == nil {
					t.Error("期望有错误，实际无错误")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("错误消息应包含 '%s'，实际: '%s'", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("不期望错误: %v", err)
				}
			}
		})
	}
}

// TestIsFormatSupported 测试格式支持检查
func TestIsFormatSupported(t *testing.T) {
	validator := NewImageValidator()

	tests := []struct {
		format   string
		expected bool
	}{
		{"image/png", true},
		{"image/jpeg", true},
		{"image/gif", true},
		{"image/webp", true},
		{"IMAGE/PNG", true},  // 大小写不敏感
		{"Image/Jpeg", true}, // 混合大小写
		{"image/bmp", false},
		{"image/tiff", false},
		{"text/plain", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			result := validator.isFormatSupported(tt.format)
			if result != tt.expected {
				t.Errorf("isFormatSupported(%q) = %v，期望 %v", tt.format, result, tt.expected)
			}
		})
	}
}

// TestExtractImageDataFromContent 测试从内容中提取图像数据
func TestExtractImageDataFromContent(t *testing.T) {
	tests := []struct {
		name          string
		content       any
		wantMediaType string
		wantData      string
		wantHasImage  bool
	}{
		{
			name:         "nil内容",
			content:      nil,
			wantHasImage: false,
		},
		{
			name:         "字符串内容",
			content:      "plain text",
			wantHasImage: false,
		},
		{
			name:         "空数组",
			content:      []any{},
			wantHasImage: false,
		},
		{
			name: "有效的图像URL数据",
			content: []any{
				map[string]any{
					"type": "image_url",
					"image_url": map[string]any{
						"url": "data:image/png;base64,iVBORw0KGgo=",
					},
				},
			},
			wantMediaType: "image/png",
			wantData:      "iVBORw0KGgo=",
			wantHasImage:  true,
		},
		{
			name: "JPEG图像",
			content: []any{
				map[string]any{
					"type": "image_url",
					"image_url": map[string]any{
						"url": "data:image/jpeg;base64,/9j/4AAQSkZJRg==",
					},
				},
			},
			wantMediaType: "image/jpeg",
			wantData:      "/9j/4AAQSkZJRg==",
			wantHasImage:  true,
		},
		{
			name: "非data URL",
			content: []any{
				map[string]any{
					"type": "image_url",
					"image_url": map[string]any{
						"url": "https://example.com/image.png",
					},
				},
			},
			wantHasImage: false,
		},
		{
			name: "混合内容取第一个图像",
			content: []any{
				map[string]any{
					"type": "text",
					"text": "Some text",
				},
				map[string]any{
					"type": "image_url",
					"image_url": map[string]any{
						"url": "data:image/gif;base64,R0lGODlh",
					},
				},
			},
			wantMediaType: "image/gif",
			wantData:      "R0lGODlh",
			wantHasImage:  true,
		},
		{
			name: "缺少image_url字段",
			content: []any{
				map[string]any{
					"type": "image_url",
				},
			},
			wantHasImage: false,
		},
		{
			name: "缺少url字段",
			content: []any{
				map[string]any{
					"type":      "image_url",
					"image_url": map[string]any{},
				},
			},
			wantHasImage: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mediaType, data, hasImage := ExtractImageDataFromContent(tt.content)

			if hasImage != tt.wantHasImage {
				t.Errorf("hasImage = %v，期望 %v", hasImage, tt.wantHasImage)
			}

			if tt.wantHasImage {
				if mediaType != tt.wantMediaType {
					t.Errorf("mediaType = %q，期望 %q", mediaType, tt.wantMediaType)
				}
				if data != tt.wantData {
					t.Errorf("data = %q，期望 %q", data, tt.wantData)
				}
			}
		})
	}
}
