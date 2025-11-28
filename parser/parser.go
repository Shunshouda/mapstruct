package parser

import (
	"fmt"
	"go/ast"
	"reflect"
	"strings"
)

// FieldInfo 字段信息
type FieldInfo struct {
	Name     string
	Type     string
	Tag      string
	Exported bool
	JSONName string // JSON标签名
}

// StructInfo 结构体信息
type StructInfo struct {
	Name       string
	Package    string
	FilePath   string
	ImportPath string // 完整的导入路径
	Fields     []FieldInfo
}

// ParseStruct 解析结构体定义
func ParseStruct(name string, structType *ast.StructType, file *ast.File) *StructInfo {
	info := &StructInfo{
		Name:   name,
		Fields: make([]FieldInfo, 0),
	}

	// 获取包名
	if file.Name != nil {
		info.Package = file.Name.Name
	}

	// 解析字段
	for _, field := range structType.Fields.List {
		if len(field.Names) == 0 {
			continue // 跳过嵌入字段
		}

		fieldName := field.Names[0].Name
		if !ast.IsExported(fieldName) {
			continue // 跳过未导出字段
		}

		fieldInfo := FieldInfo{
			Name:     fieldName,
			Type:     getTypeString(field.Type),
			Exported: true,
		}

		// 解析标签
		if field.Tag != nil {
			tagValue := strings.Trim(field.Tag.Value, "`")
			fieldInfo.Tag = tagValue
			fieldInfo.JSONName = parseJSONTag(tagValue)
		}

		info.Fields = append(info.Fields, fieldInfo)
	}

	return info
}

// String 方法用于调试
func (s *StructInfo) String() string {
	return fmt.Sprintf("StructInfo{Name: %s, Package: %s, Fields: %d}",
		s.Name, s.Package, len(s.Fields))
}

// getTypeString 获取类型字符串表示
func getTypeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + getTypeString(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + getTypeString(t.Elt)
		}
		return "[" + getTypeString(t.Len) + "]" + getTypeString(t.Elt)
	case *ast.SelectorExpr:
		return getTypeString(t.X) + "." + t.Sel.Name
	case *ast.MapType:
		return "map[" + getTypeString(t.Key) + "]" + getTypeString(t.Value)
	case *ast.InterfaceType:
		return "interface{}"
	default:
		return "interface{}"
	}
}

// parseJSONTag 解析JSON标签
func parseJSONTag(tag string) string {
	if tag == "" {
		return ""
	}

	structTag := reflect.StructTag(tag)
	jsonTag := structTag.Get("json")
	if jsonTag == "" {
		return ""
	}

	// 处理 json:"name,omitempty" 这种情况
	parts := strings.Split(jsonTag, ",")
	return parts[0]
}

// GetFieldByName 根据名称查找字段
func (s *StructInfo) GetFieldByName(name string) *FieldInfo {
	for _, field := range s.Fields {
		if field.Name == name {
			return &field
		}
	}
	return nil
}

// GetFieldByJSONName 根据JSON名称查找字段
func (s *StructInfo) GetFieldByJSONName(jsonName string) *FieldInfo {
	for _, field := range s.Fields {
		if field.JSONName == jsonName {
			return &field
		}
	}
	return nil
}

// GetMapStructField 根据mapstruct标签查找映射字段
func (s *StructInfo) GetMapStructField(sourceFieldName string) *FieldInfo {
	for _, field := range s.Fields {
		if hasMapStructTag(field.Tag, sourceFieldName) {
			return &field
		}
	}
	return nil
}

// hasMapStructTag 检查字段是否有对应的mapstruct标签
func hasMapStructTag(tag, fieldName string) bool {
	if tag == "" {
		return false
	}

	structTag := reflect.StructTag(tag)
	mapStructTag := structTag.Get("mapstruct")
	return mapStructTag == fieldName
}
