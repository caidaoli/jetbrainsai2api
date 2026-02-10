package validate

import (
	"encoding/base64"
	"fmt"
	"strings"

	"jetbrainsai2api/internal/core"
)

// ImageValidator provides image validation functionality
type ImageValidator struct{}

// NewImageValidator creates a new image validator
func NewImageValidator() *ImageValidator {
	return &ImageValidator{}
}

// ValidateImageData validates base64 encoded image data
func (v *ImageValidator) ValidateImageData(mediaType, data string) error {
	if !v.isFormatSupported(mediaType) {
		return fmt.Errorf("unsupported image format: %s. Supported formats: %v",
			mediaType, core.SupportedImageFormats)
	}

	// Pre-check base64 string length to avoid OOM from decoding huge data
	estimatedSize := int64(len(data)) * 3 / 4
	if estimatedSize > core.MaxImageSizeBytes {
		return fmt.Errorf("image data too large: estimated %d bytes exceeds %d limit", estimatedSize, core.MaxImageSizeBytes)
	}

	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return fmt.Errorf("invalid base64 data: %v", err)
	}

	if int64(len(decoded)) > core.MaxImageSizeBytes {
		return fmt.Errorf("image size %d bytes exceeds maximum allowed size %d bytes",
			len(decoded), core.MaxImageSizeBytes)
	}

	return nil
}

func (v *ImageValidator) isFormatSupported(mediaType string) bool {
	for _, format := range core.SupportedImageFormats {
		if strings.EqualFold(format, mediaType) {
			return true
		}
	}
	return false
}

// ExtractImageDataFromContent extracts the first image data from OpenAI content format.
// NOTE: Only the first image is extracted; subsequent images in the content array are ignored.
func ExtractImageDataFromContent(content any) (mediaType, data string, hasImage bool) {
	if content == nil {
		return "", "", false
	}

	if contentArray, ok := content.([]any); ok {
		for _, item := range contentArray {
			if itemMap, ok := item.(map[string]any); ok {
				if itemType, ok := itemMap["type"].(string); ok && itemType == "image_url" {
					if imageUrl, ok := itemMap["image_url"].(map[string]any); ok {
						if url, ok := imageUrl["url"].(string); ok {
							if strings.HasPrefix(url, "data:") {
								parts := strings.SplitN(url, ",", 2)
								if len(parts) == 2 {
									headerParts := strings.Split(parts[0], ";")
									if len(headerParts) > 0 {
										mediaType := strings.TrimPrefix(headerParts[0], "data:")
										return mediaType, parts[1], true
									}
								}
							}
						}
					}
				}
			}
		}
	}

	return "", "", false
}
