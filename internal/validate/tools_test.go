package validate

import (
	"testing"

	"jetbrainsai2api/internal/core"
)

func TestSimplifyComplexTool(t *testing.T) {
	tests := []struct {
		name       string
		properties map[string]any
	}{
		{"少于5个属性", map[string]any{"name": map[string]any{"type": "string"}, "age": map[string]any{"type": "integer"}}},
		{"超过5个属性只保留前5个", map[string]any{
			"prop1": map[string]any{"type": "string"}, "prop2": map[string]any{"type": "string"},
			"prop3": map[string]any{"type": "string"}, "prop4": map[string]any{"type": "string"},
			"prop5": map[string]any{"type": "string"}, "prop6": map[string]any{"type": "string"},
			"prop7": map[string]any{"type": "string"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := simplifyComplexTool(tt.properties)
			if _, ok := result["data"]; !ok { t.Error("期望有 data 字段") }
			if dataField, ok := result["data"].(map[string]any); ok {
				if dataField["type"] != core.SchemaTypeString { t.Errorf("data 字段类型应为 string") }
			}
			if len(result) > 6 { t.Errorf("简化后的属性数量不应超过6个，实际有 %d 个", len(result)) }
		})
	}
}

func TestSimplifyUnionType(t *testing.T) {
	tests := []struct {
		name      string
		schemaMap map[string]any
		unionType string
	}{
		{"anyOf类型简化", map[string]any{"anyOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "integer"}}}, "anyOf"},
		{"oneOf类型简化", map[string]any{"oneOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "boolean"}}}, "oneOf"},
		{"保留原有description", map[string]any{"description": "原始描述", "anyOf": []any{map[string]any{"type": "string"}}}, "anyOf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := simplifyUnionType(tt.schemaMap, tt.unionType)
			if result["type"] != core.SchemaTypeString { t.Errorf("简化后类型应为 string") }
			if _, ok := result["description"]; !ok { t.Error("应该有 description 字段") }
			if origDesc, hasOrig := tt.schemaMap["description"]; hasOrig {
				if result["description"] != origDesc { t.Errorf("应保留原始描述") }
			}
		})
	}
}

func TestTransformParamName(t *testing.T) {
	tests := []struct {
		name, input, expected string
	}{
		{"合法名称保持不变", "valid_name", "valid_name"},
		{"移除空格", "name with spaces", "namewithspaces"},
		{"移除特殊字符", "name@#$%test", "nametest"},
		{"保留点和横线", "name.test-value", "name.test-value"},
		{"空字符串返回默认值", "!!!", "param"},
		{"截断超长名称", "this_is_a_very_long_parameter_name_that_exceeds_the_maximum_length_limit_of_64_characters", "this_is_a_very_long_parameter_name_that_exceeds_the_maximum_leng"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := transformParamName(tt.input)
			if result != tt.expected { t.Errorf("transformParamName(%q) = %q，期望 %q", tt.input, result, tt.expected) }
			if len(result) > core.MaxParamNameLength { t.Errorf("结果长度 %d 超过最大限制 %d", len(result), core.MaxParamNameLength) }
		})
	}
}

func TestIsValidParamName(t *testing.T) {
	tests := []struct {
		name, input string
		expected    bool
	}{
		{"合法名称", "valid_name", true},
		{"带数字", "name123", true},
		{"带空格", "name space", false},
		{"带特殊字符", "name@test", false},
		{"中文字符", "参数名", false},
		{"超长名称", "this_is_a_very_long_parameter_name_that_exceeds_the_maximum_length_limit", false},
		{"空字符串", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if result := isValidParamName(tt.input); result != tt.expected {
				t.Errorf("isValidParamName(%q) = %v，期望 %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTransformObject(t *testing.T) {
	tests := []struct {
		name     string
		schema   map[string]any
		depth    int
		wantType string
	}{
		{"简单对象", map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}}, 0, "object"},
		{"无properties的对象简化为string", map[string]any{"type": "object"}, 0, "string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := make(map[string]any)
			transformObject(tt.schema, result, tt.depth)
			if result["type"] != tt.wantType { t.Errorf("期望类型 %s，实际 %v", tt.wantType, result["type"]) }
		})
	}
}

func TestTransformArray(t *testing.T) {
	tests := []struct {
		name   string
		schema map[string]any
	}{
		{"带items的数组", map[string]any{"type": "array", "items": map[string]any{"type": "string"}}},
		{"无items的数组", map[string]any{"type": "array"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := make(map[string]any)
			transformArray(tt.schema, result)
			if result["type"] != "array" { t.Errorf("期望类型 array") }
			if _, ok := result["items"]; !ok { t.Error("应该有 items 字段") }
		})
	}
}

func TestCopySimpleProperties(t *testing.T) {
	tests := []struct {
		name     string
		src      map[string]any
		wantKeys []string
	}{
		{"复制description", map[string]any{"description": "测试描述", "type": "string"}, []string{"description"}},
		{"复制多个字段", map[string]any{"description": "描述", "enum": []string{"a", "b"}, "pattern": "^[a-z]+$"}, []string{"description", "enum", "pattern"}},
		{"复制支持的format", map[string]any{"format": "email"}, []string{"format"}},
		{"忽略不支持的format", map[string]any{"format": "unsupported-format"}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dest := make(map[string]any)
			copySimpleProperties(tt.src, dest)
			for _, key := range tt.wantKeys {
				if _, ok := dest[key]; !ok { t.Errorf("期望字段 %s 存在", key) }
			}
		})
	}
}

func TestValidateRequiredFields(t *testing.T) {
	tests := []struct {
		name       string
		required   []any
		properties map[string]any
		wantCount  int
	}{
		{"全部有效的必填字段", []any{"name", "age"}, map[string]any{"name": map[string]any{"type": "string"}, "age": map[string]any{"type": "integer"}}, 2},
		{"部分字段不存在", []any{"name", "missing"}, map[string]any{"name": map[string]any{"type": "string"}}, 1},
		{"空必填列表", []any{}, map[string]any{"name": map[string]any{"type": "string"}}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateRequiredFields(tt.required, tt.properties)
			if len(result) != tt.wantCount { t.Errorf("期望 %d 个有效必填字段，实际 %d 个", tt.wantCount, len(result)) }
		})
	}
}

func TestTransformPropertySchema(t *testing.T) {
	tests := []struct {
		name     string
		schema   any
		wantType string
	}{
		{"字符串类型", map[string]any{"type": "string"}, "string"},
		{"整数类型", map[string]any{"type": "integer"}, "integer"},
		{"anyOf类型简化", map[string]any{"anyOf": []any{map[string]any{"type": "string"}}}, "string"},
		{"nil输入", nil, "string"},
		{"非map输入", "invalid", "string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _ := transformPropertySchema(tt.schema, 0)
			if result["type"] != tt.wantType { t.Errorf("期望类型 %s，实际 %v", tt.wantType, result["type"]) }
		})
	}
}

func TestTransformProperties(t *testing.T) {
	tests := []struct {
		name       string
		properties map[string]any
		wantKeys   []string
	}{
		{"简单属性", map[string]any{"name": map[string]any{"type": "string"}, "age": map[string]any{"type": "integer"}}, []string{"name", "age"}},
		{"非法参数名会被转换", map[string]any{"valid_name": map[string]any{"type": "string"}, "invalid name": map[string]any{"type": "string"}}, []string{"valid_name", "invalidname"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := transformProperties(tt.properties)
			if err != nil { t.Fatalf("不期望错误: %v", err) }
			for _, key := range tt.wantKeys {
				if _, ok := result[key]; !ok { t.Errorf("期望键 %s 存在", key) }
			}
		})
	}
}

func TestTransformParameters(t *testing.T) {
	tests := []struct {
		name     string
		params   map[string]any
		wantType string
		wantErr  bool
	}{
		{"简单参数", map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}}, "object", false},
		{"空参数", nil, "object", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := transformParameters(tt.params)
			if tt.wantErr {
				if err == nil { t.Error("期望有错误") }
				return
			}
			if err != nil { t.Fatalf("不期望错误: %v", err) }
			if result["type"] != tt.wantType { t.Errorf("期望类型 %s", tt.wantType) }
		})
	}
}

func TestValidateToolCallResponse(t *testing.T) {
	tests := []struct {
		name      string
		toolCall  core.ToolCall
		expectErr bool
	}{
		{"有效的工具调用", core.ToolCall{ID: "call_123", Type: core.ToolTypeFunction, Function: core.Function{Name: "get_weather", Arguments: `{"city":"Beijing"}`}}, false},
		{"空ID", core.ToolCall{ID: "", Type: core.ToolTypeFunction, Function: core.Function{Name: "get_weather", Arguments: `{}`}}, true},
		{"空函数名", core.ToolCall{ID: "call_123", Type: core.ToolTypeFunction, Function: core.Function{Name: "", Arguments: `{}`}}, true},
		{"无效JSON参数", core.ToolCall{ID: "call_123", Type: core.ToolTypeFunction, Function: core.Function{Name: "test_func", Arguments: `{invalid json}`}}, true},
		{"空参数（有效）", core.ToolCall{ID: "call_123", Type: core.ToolTypeFunction, Function: core.Function{Name: "no_args_func", Arguments: ""}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateToolCallResponse(tt.toolCall)
			if tt.expectErr {
				if err == nil { t.Error("期望有错误，实际无错误") }
			} else {
				if err != nil { t.Errorf("不期望错误: %v", err) }
			}
		})
	}
}
