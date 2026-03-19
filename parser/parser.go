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
	JSONName string // JSON 标签名
	// IsEmbedded 表示此字段是否来自嵌入结构体
	IsEmbedded bool
	// EmbeddedPath 嵌入路径，用于生成代码时访问嵌入字段
	EmbeddedPath []string
	// IsNestedObject 表示此字段是否是嵌套对象（结构体类型）
	IsNestedObject bool
	// NestedStructInfo 如果是嵌套对象，存储其结构体信息
	NestedStructInfo *StructInfo
}

// StructInfo 结构体信息
type StructInfo struct {
	Name       string
	Package    string
	FilePath   string
	ImportPath string // 完整的导入路径
	Fields     []FieldInfo
	// EmbeddedTypes 记录嵌入的类型
	EmbeddedTypes []EmbeddedType

	// 缓存字段查找，提升性能
	fieldByNameMap     map[string]*FieldInfo
	fieldByJSONNameMap map[string]*FieldInfo
}

// EmbeddedType 嵌入类型信息
type EmbeddedType struct {
	TypeName string
	// 如果是外部包的类型，需要包名
	Package string
	// 嵌入字段的访问路径
	FieldPath []string
}

// ParseStruct 解析结构体定义
// allStructs 用于解析嵌入字段和嵌套对象，可以为 nil（向后兼容）
func ParseStruct(name string, structType *ast.StructType, file *ast.File, allStructs map[string]*StructInfo) *StructInfo {
	info := &StructInfo{
		Name:          name,
		Fields:        make([]FieldInfo, 0),
		EmbeddedTypes: make([]EmbeddedType, 0),
	}

	// 获取包名
	if file.Name != nil {
		info.Package = file.Name.Name
	}

	// 解析字段
	info.parseFields(structType.Fields, file, allStructs, nil)

	return info
}

// parseFields 递归解析字段列表
func (s *StructInfo) parseFields(fields *ast.FieldList, file *ast.File, allStructs map[string]*StructInfo, embedPath []string) {
	if fields == nil {
		return
	}

	for _, field := range fields.List {
		// 嵌入字段（匿名字段）：没有名字或名字为空
		if len(field.Names) == 0 {
			s.parseEmbeddedField(field, file, allStructs, embedPath)
			continue
		}

		fieldName := field.Names[0].Name
		if !ast.IsExported(fieldName) {
			continue // 跳过未导出字段
		}

		fieldType := getTypeString(field.Type)

		fieldInfo := FieldInfo{
			Name:         fieldName,
			Type:         fieldType,
			Exported:     true,
			IsEmbedded:   len(embedPath) > 0,
			EmbeddedPath: append([]string(nil), embedPath...), // 复制路径
		}

		// 检查是否是嵌套对象（结构体类型）
		if allStructs != nil {
			nestedStruct := s.findNestedStruct(fieldType, file, allStructs)
			if nestedStruct != nil {
				fieldInfo.IsNestedObject = true
				fieldInfo.NestedStructInfo = nestedStruct
			}
		}

		// 解析标签
		if field.Tag != nil {
			tagValue := strings.Trim(field.Tag.Value, "`")
			fieldInfo.Tag = tagValue
			fieldInfo.JSONName = parseJSONTag(tagValue)
		}

		s.Fields = append(s.Fields, fieldInfo)
	}
}

// parseEmbeddedField 解析嵌入字段
func (s *StructInfo) parseEmbeddedField(field *ast.Field, file *ast.File, allStructs map[string]*StructInfo, embedPath []string) {
	if allStructs == nil {
		return // 如果没有提供 allStructs，无法解析嵌入字段
	}

	// 获取嵌入类型的名称
	embeddedTypeName := getTypeString(field.Type)
	if embeddedTypeName == "" {
		return
	}

	// 解析嵌入类型的包名和类型名
	pkgName, typeName := s.parseTypeWithPackage(embeddedTypeName, file)

	// 构建嵌入类型的访问路径
	newEmbedPath := append(append([]string(nil), embedPath...), typeName)

	// 记录嵌入类型
	s.EmbeddedTypes = append(s.EmbeddedTypes, EmbeddedType{
		TypeName:  typeName,
		Package:   pkgName,
		FieldPath: append([]string(nil), newEmbedPath...),
	})

	// 查找嵌入类型的结构体定义
	var embeddedStruct *StructInfo

	// 尝试多种键查找
	keys := []string{
		embeddedTypeName,                   // 完整类型名
		typeName,                           // 仅类型名
		pkgName + "." + typeName,           // 包名。类型名
		s.Package + "." + embeddedTypeName, // 当前包。类型名
	}

	for _, key := range keys {
		if st, ok := allStructs[key]; ok {
			embeddedStruct = st
			break
		}
	}

	// 如果找到了嵌入结构体，递归解析其字段
	if embeddedStruct != nil {
		// 复制嵌入结构体的字段，更新嵌入路径
		for _, f := range embeddedStruct.Fields {
			// 跳过已存在的字段（避免冲突）
			if s.GetFieldByName(f.Name) != nil {
				continue
			}

			newField := FieldInfo{
				Name:         f.Name,
				Type:         f.Type,
				Tag:          f.Tag,
				Exported:     f.Exported,
				JSONName:     f.JSONName,
				IsEmbedded:   true,
				EmbeddedPath: append([]string(nil), newEmbedPath...),
			}

			// 检查是否是嵌套对象
			nestedStruct := s.findNestedStruct(f.Type, file, allStructs)
			if nestedStruct != nil {
				newField.IsNestedObject = true
				newField.NestedStructInfo = nestedStruct
			}

			s.Fields = append(s.Fields, newField)
		}
	}
}

// findNestedStruct 查找嵌套的结构体类型
func (s *StructInfo) findNestedStruct(fieldType string, file *ast.File, allStructs map[string]*StructInfo) *StructInfo {
	// 处理指针类型
	typeName := strings.TrimPrefix(fieldType, "*")

	// 处理数组类型
	if strings.HasPrefix(fieldType, "[]") {
		typeName = strings.TrimPrefix(fieldType, "[]")
		typeName = strings.TrimPrefix(typeName, "*")
	}

	// 解析包名和类型名
	pkgName, bareTypeName := s.parseTypeWithPackage(typeName, file)

	// 尝试多种键查找，包括依赖包的路径
	keys := []string{
		typeName,                     // 完整类型名
		bareTypeName,                 // 仅类型名
		pkgName + "." + bareTypeName, // 包名。类型名
		s.Package + "." + typeName,   // 当前包。类型名
	}

	// 如果有包名前缀，添加导入路径的键
	if pkgName != "" && pkgName != s.Package {
		// 尝试从导入中查找实际路径
		if file != nil {
			for _, imp := range file.Imports {
				importPath := strings.Trim(imp.Path.Value, `"`)
				parts := strings.Split(importPath, "/")
				lastPart := parts[len(parts)-1]

				// 匹配包名别名
				if imp.Name != nil && imp.Name.Name == pkgName {
					keys = append(keys, importPath+"."+bareTypeName)
				} else if lastPart == pkgName {
					keys = append(keys, importPath+"."+bareTypeName)
				}
			}
		}
	}

	for _, key := range keys {
		if st, ok := allStructs[key]; ok {
			return st
		}
	}

	return nil
}

// parseTypeWithPackage 解析类型名，返回包名和类型名
func (s *StructInfo) parseTypeWithPackage(typeName string, file *ast.File) (pkgName, bareTypeName string) {
	// 处理指针类型
	if strings.HasPrefix(typeName, "*") {
		typeName = strings.TrimPrefix(typeName, "*")
	}

	// 检查是否包含包名前缀（如 pkg.Type）
	if idx := strings.Index(typeName, "."); idx > 0 {
		pkgAlias := typeName[:idx]
		bareName := typeName[idx+1:]

		// 从导入声明中查找实际的包名
		if file != nil {
			for _, imp := range file.Imports {
				importPath := strings.Trim(imp.Path.Value, `"`)
				// 如果使用了别名，匹配别名
				if imp.Name != nil && imp.Name.Name == pkgAlias {
					// 使用别名作为包名
					return pkgAlias, bareName
				}
				// 否则匹配导入路径的最后一部分
				parts := strings.Split(importPath, "/")
				lastPart := parts[len(parts)-1]
				if lastPart == pkgAlias {
					return pkgAlias, bareName
				}
			}
		}
		return pkgAlias, bareName
	}

	// 没有包名前缀，使用当前包
	return s.Package, typeName
}

// String 方法用于调试
func (s *StructInfo) String() string {
	return fmt.Sprintf("StructInfo{Name: %s, Package: %s, Fields: %d, Embedded: %d}",
		s.Name, s.Package, len(s.Fields), len(s.EmbeddedTypes))
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

// parseJSONTag 解析 JSON 标签
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

// GetFieldByName 根据名称查找字段（使用缓存的 Map，O(1) 复杂度）
func (s *StructInfo) GetFieldByName(name string) *FieldInfo {
	if s.fieldByNameMap == nil {
		s.fieldByNameMap = make(map[string]*FieldInfo, len(s.Fields))
		for i := range s.Fields {
			s.fieldByNameMap[s.Fields[i].Name] = &s.Fields[i]
		}
	}
	return s.fieldByNameMap[name]
}

// GetFieldByJSONName 根据 JSON 名称查找字段（使用缓存的 Map，O(1) 复杂度）
func (s *StructInfo) GetFieldByJSONName(jsonName string) *FieldInfo {
	if jsonName == "" {
		return nil
	}
	if s.fieldByJSONNameMap == nil {
		s.fieldByJSONNameMap = make(map[string]*FieldInfo, len(s.Fields))
		for i := range s.Fields {
			if s.Fields[i].JSONName != "" {
				s.fieldByJSONNameMap[s.Fields[i].JSONName] = &s.Fields[i]
			}
		}
	}
	return s.fieldByJSONNameMap[jsonName]
}

// GetMapStructField 根据 mapstruct 标签查找映射字段
func (s *StructInfo) GetMapStructField(sourceFieldName string) *FieldInfo {
	for _, field := range s.Fields {
		if hasMapStructTag(field.Tag, sourceFieldName) {
			return &field
		}
	}
	return nil
}

// hasMapStructTag 检查字段是否有对应的 mapstruct 标签
func hasMapStructTag(tag, fieldName string) bool {
	if tag == "" {
		return false
	}

	structTag := reflect.StructTag(tag)
	mapStructTag := structTag.Get("mapstruct")
	return mapStructTag == fieldName
}
