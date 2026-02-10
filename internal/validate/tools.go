package validate

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"jetbrainsai2api/internal/core"

	"github.com/bytedance/sonic"
)

var (
	paramNameRegex = regexp.MustCompile(core.ParamNamePattern)
)

// ValidateAndTransformTools validates and transforms tool definitions
func ValidateAndTransformTools(tools []core.Tool, logger core.Logger) ([]core.Tool, error) {
	if len(tools) == 0 {
		return tools, nil
	}

	validatedTools := make([]core.Tool, 0, len(tools))
	skippedCount := 0

	for _, tool := range tools {
		if !isValidParamName(tool.Function.Name) {
			logger.Warn("Skipping tool with invalid name: %s (must match %s)", tool.Function.Name, core.ParamNamePattern)
			skippedCount++
			continue
		}

		transformedParams, err := transformParameters(tool.Function.Parameters)
		if err != nil {
			logger.Warn("Skipping tool %s: parameter transformation failed: %v", tool.Function.Name, err)
			skippedCount++
			continue
		}

		validatedTool := core.Tool{
			Type: tool.Type,
			Function: core.ToolFunction{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  transformedParams,
			},
		}

		validatedTools = append(validatedTools, validatedTool)
	}

	if skippedCount > 0 {
		logger.Warn("Tool validation: %d/%d tools skipped due to validation errors", skippedCount, len(tools))
	}

	return validatedTools, nil
}

func transformParameters(params map[string]any) (map[string]any, error) {
	if params == nil {
		return map[string]any{
			"type":                 core.SchemaTypeObject,
			"properties":           map[string]any{},
			"additionalProperties": false,
		}, nil
	}

	result := make(map[string]any)
	if schemaType, ok := params["type"]; ok {
		result["type"] = schemaType
	}

	if properties, ok := params["properties"].(map[string]any); ok {
		propCount := len(properties)

		if propCount > core.MaxPropertiesBeforeSimplification {
			result["properties"] = simplifyComplexTool(properties)
		} else {
			transformedProps, err := transformProperties(properties)
			if err != nil {
				return nil, err
			}
			result["properties"] = transformedProps
		}
	}

	if required, ok := params["required"].([]any); ok {
		result["required"] = validateRequiredFields(required, result["properties"])
	}

	result["additionalProperties"] = false

	return result, nil
}

func simplifyComplexTool(properties map[string]any) map[string]any {
	resultProps := map[string]any{
		"data": map[string]any{
			"type":        core.SchemaTypeString,
			"description": fmt.Sprintf("Provide all %d required fields as a single JSON string", len(properties)),
		},
	}

	keys := make([]string, 0, len(properties))
	for k := range properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	count := 0
	for _, propName := range keys {
		if count >= core.MaxPreservedPropertiesInSimplifiedSchema {
			break
		}
		validName := propName
		if !isValidParamName(propName) {
			validName = transformParamName(propName)
		}
		if isValidParamName(validName) {
			simplified, _ := transformPropertySchema(properties[propName], 0)
			resultProps[validName] = simplified
			count++
		}
	}

	return resultProps
}

func transformProperties(properties map[string]any) (map[string]any, error) {
	result := make(map[string]any)

	for propName, propSchema := range properties {
		validName := propName
		if !isValidParamName(propName) {
			validName = transformParamName(propName)
			if !isValidParamName(validName) {
				continue
			}
		}

		transformedSchema, err := transformPropertySchema(propSchema, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to transform property '%s': %v", propName, err)
		}

		result[validName] = transformedSchema
	}

	return result, nil
}

func transformPropertySchema(schema any, depth int) (map[string]any, error) {
	schemaMap, ok := schema.(map[string]any)
	if !ok {
		return map[string]any{"type": core.SchemaTypeString}, nil
	}

	if depth > core.MaxNestingDepth {
		return map[string]any{
			"type":        core.SchemaTypeString,
			"description": "Deeply nested object - provide as JSON string",
		}, nil
	}

	result := make(map[string]any)

	for _, unionType := range []string{"anyOf", "oneOf", "allOf"} {
		if _, hasUnion := schemaMap[unionType]; hasUnion {
			return simplifyUnionType(schemaMap, unionType), nil
		}
	}

	schemaType, hasType := schemaMap["type"]
	if !hasType {
		schemaType = core.SchemaTypeString
	}
	result["type"] = schemaType

	switch schemaType {
	case core.SchemaTypeObject:
		transformObject(schemaMap, result, depth)
	case core.SchemaTypeArray:
		transformArray(schemaMap, result)
	default:
		copySimpleProperties(schemaMap, result)
	}

	return result, nil
}

func simplifyUnionType(schemaMap map[string]any, unionType string) map[string]any {
	result := map[string]any{"type": core.SchemaTypeString}

	if desc, hasDesc := schemaMap["description"]; hasDesc {
		result["description"] = desc
	} else {
		result["description"] = fmt.Sprintf("Complex type (%s) simplified to string", unionType)
	}

	return result
}

func transformObject(schemaMap, result map[string]any, depth int) {
	properties, hasProps := schemaMap["properties"].(map[string]any)
	if !hasProps {
		result["type"] = core.SchemaTypeString
		result["description"] = "Object without properties - provide as JSON string"
		return
	}

	propCount := len(properties)
	if propCount > core.MaxPropertiesBeforeSimplification {
		result["type"] = core.SchemaTypeString
		result["description"] = fmt.Sprintf("Complex object with %d properties - provide as JSON string", propCount)
		return
	}

	simplifiedProps := make(map[string]any)
	for propName, propSchema := range properties {
		validName := propName
		if !isValidParamName(propName) {
			validName = transformParamName(propName)
		}
		if isValidParamName(validName) {
			simplified, _ := transformPropertySchema(propSchema, depth+1)
			simplifiedProps[validName] = simplified
		}
	}

	result["type"] = core.SchemaTypeObject
	result["properties"] = simplifiedProps
	result["additionalProperties"] = false

	if req, hasReq := schemaMap["required"].([]any); hasReq {
		result["required"] = validateRequiredFields(req, simplifiedProps)
	}
}

func transformArray(schemaMap, result map[string]any) {
	result["type"] = core.SchemaTypeArray

	if items, ok := schemaMap["items"].(map[string]any); ok {
		if itemType, ok := items["type"]; ok {
			result["items"] = map[string]any{"type": itemType}
		} else {
			result["items"] = map[string]any{"type": core.SchemaTypeString}
		}
	} else {
		result["items"] = map[string]any{"type": core.SchemaTypeString}
	}
}

func copySimpleProperties(schemaMap, result map[string]any) {
	simpleProps := []string{
		"description", "enum", "pattern",
		"minimum", "maximum", "minLength", "maxLength",
		"minItems", "maxItems",
	}

	for _, key := range simpleProps {
		if value, ok := schemaMap[key]; ok {
			result[key] = value
		}
	}

	if formatStr, ok := schemaMap["format"].(string); ok {
		supportedFormats := []string{"email", "uri", "date", "date-time"}
		for _, supported := range supportedFormats {
			if formatStr == supported {
				result["format"] = formatStr
				break
			}
		}
	}

	if enum, ok := schemaMap["enum"]; ok {
		result["enum"] = enum
	}
}

func validateRequiredFields(required []any, properties any) []string {
	var validRequired []string

	propsMap, ok := properties.(map[string]any)
	if !ok {
		return validRequired
	}

	for _, req := range required {
		reqStr, ok := req.(string)
		if !ok {
			continue
		}

		validName := reqStr
		if !isValidParamName(reqStr) {
			validName = transformParamName(reqStr)
		}

		if isValidParamName(validName) && propsMap[validName] != nil {
			validRequired = append(validRequired, validName)
		}
	}

	return validRequired
}

func isValidParamName(name string) bool {
	return len(name) <= core.MaxParamNameLength && paramNameRegex.MatchString(name)
}

func transformParamName(name string) string {
	var builder strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '.' || r == '-' {
			if builder.Len() < core.MaxParamNameLength {
				builder.WriteRune(r)
			}
		}
	}

	result := builder.String()
	if result == "" {
		result = "param"
	}

	if len(result) > core.MaxParamNameLength {
		result = result[:core.MaxParamNameLength]
	}

	return result
}

// ValidateToolCallResponse validates a tool call response
func ValidateToolCallResponse(tc core.ToolCall) error {
	if tc.ID == "" {
		return fmt.Errorf("tool call ID is empty")
	}
	if tc.Function.Name == "" {
		return fmt.Errorf("tool call function name is empty")
	}
	if tc.Function.Arguments != "" {
		var js any
		if err := sonic.Unmarshal([]byte(tc.Function.Arguments), &js); err != nil {
			return fmt.Errorf("tool call arguments is not valid JSON: %w", err)
		}
	}
	return nil
}
