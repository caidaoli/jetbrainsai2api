package main

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// ImageValidator provides image validation functionality for JetBrains AI API v8
type ImageValidator struct {
	MaxSizeBytes     int64
	SupportedFormats []string
}

// NewImageValidator creates a new image validator with default settings
func NewImageValidator() *ImageValidator {
	return &ImageValidator{
		MaxSizeBytes:     MaxImageSizeBytes,
		SupportedFormats: SupportedImageFormats,
	}
}

// ValidateImageData validates base64 encoded image data
func (v *ImageValidator) ValidateImageData(mediaType, data string) error {
	// Check if media type is supported
	if !v.isFormatSupported(mediaType) {
		return fmt.Errorf("unsupported image format: %s. Supported formats: %v",
			mediaType, v.SupportedFormats)
	}

	// Decode base64 to check size
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return fmt.Errorf("invalid base64 data: %v", err)
	}

	// Check file size
	if int64(len(decoded)) > v.MaxSizeBytes {
		return fmt.Errorf("image size %d bytes exceeds maximum allowed size %d bytes",
			len(decoded), v.MaxSizeBytes)
	}

	return nil
}

// isFormatSupported checks if the given media type is supported
func (v *ImageValidator) isFormatSupported(mediaType string) bool {
	for _, format := range v.SupportedFormats {
		if strings.EqualFold(format, mediaType) {
			return true
		}
	}
	return false
}

// ExtractImageDataFromContent extracts image data from OpenAI content format
func ExtractImageDataFromContent(content any) (mediaType, data string, hasImage bool) {
	if content == nil {
		return "", "", false
	}

	// Handle array of content items (OpenAI multimodal format)
	if contentArray, ok := content.([]any); ok {
		for _, item := range contentArray {
			if itemMap, ok := item.(map[string]any); ok {
				if itemType, ok := itemMap["type"].(string); ok && itemType == "image_url" {
					if imageUrl, ok := itemMap["image_url"].(map[string]any); ok {
						if url, ok := imageUrl["url"].(string); ok {
							// Parse data URL format: data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAA...
							if strings.HasPrefix(url, "data:") {
								parts := strings.SplitN(url, ",", 2)
								if len(parts) == 2 {
									// Extract media type from data URL
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
