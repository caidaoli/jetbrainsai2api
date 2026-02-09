package validate

import (
	"encoding/base64"
	"testing"
)

func TestImageValidator_ValidateImageData(t *testing.T) {
	validator := NewImageValidator()

	smallPNG := base64.StdEncoding.EncodeToString(make([]byte, 100))

	tests := []struct {
		name      string
		mediaType string
		data      string
		wantErr   bool
	}{
		{"有效PNG", "image/png", smallPNG, false},
		{"有效JPEG", "image/jpeg", smallPNG, false},
		{"有效GIF", "image/gif", smallPNG, false},
		{"有效WebP", "image/webp", smallPNG, false},
		{"不支持的格式", "image/bmp", smallPNG, true},
		{"无效base64", "image/png", "!!!invalid!!!", true},
		// Empty string is valid base64 (decodes to empty bytes), so no error expected
		{"空数据", "image/png", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateImageData(tt.mediaType, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateImageData() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExtractImageDataFromContent(t *testing.T) {
	tests := []struct {
		name      string
		content   any
		wantMedia string
		wantData  string
		wantImage bool
	}{
		{
			name:      "字符串内容无图片",
			content:   "Hello world",
			wantImage: false,
		},
		{
			name:      "nil内容",
			content:   nil,
			wantImage: false,
		},
		{
			name: "包含data URI图片",
			content: []any{
				map[string]any{
					"type": "image_url",
					"image_url": map[string]any{
						"url": "data:image/png;base64,iVBORw0KGgoAAAANSU",
					},
				},
			},
			wantMedia: "image/png",
			wantData:  "iVBORw0KGgoAAAANSU",
			wantImage: true,
		},
		{
			name: "只有文本块",
			content: []any{
				map[string]any{"type": "text", "text": "Just text"},
			},
			wantImage: false,
		},
		{
			// ExtractImageDataFromContent only handles "image_url" type with data: URI
			// It does NOT handle Anthropic-style "image" with "source" - that's a different path
			name: "非image_url类型不提取",
			content: []any{
				map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/jpeg", "data": "dGVzdA=="}},
			},
			wantImage: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mediaType, data, hasImage := ExtractImageDataFromContent(tt.content)
			if hasImage != tt.wantImage {
				t.Errorf("hasImage = %v, want %v", hasImage, tt.wantImage)
			}
			if hasImage {
				if mediaType != tt.wantMedia {
					t.Errorf("mediaType = %s, want %s", mediaType, tt.wantMedia)
				}
				if data != tt.wantData {
					t.Errorf("data = %s, want %s", data, tt.wantData)
				}
			}
		})
	}
}

func TestNewImageValidator(t *testing.T) {
	v := NewImageValidator()
	if v == nil {
		t.Error("NewImageValidator should not return nil")
	}
}
