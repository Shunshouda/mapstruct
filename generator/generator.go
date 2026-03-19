package generator

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/shunshouda/mapstruct/parser"
)

// TypePair 类型对
type TypePair struct {
	SourcePkg       string
	SourceType      string
	DestPkg         string
	DestType        string
	IsMapConversion bool   // 是否是 map 转换
	Direction       string // to-map, from-map, both
	MapValueType    string // map 值类型: any, interface{} 等
}

// FieldMapping 字段映射
type FieldMapping struct {
	SourceField parser.FieldInfo
	DestField   parser.FieldInfo
	MappingType string // "name", "json", "tag"
}

// StructMapping 结构体映射
type StructMapping struct {
	Source      *parser.StructInfo
	Dest        *parser.StructInfo
	FieldMaps   []FieldMapping
	MethodName  string
	NeedImports map[string]string // 包名 -> 导入路径
}

// MapMapping map 映射配置
type MapMapping struct {
	Struct       *parser.StructInfo
	Direction    string // "to-map", "from-map"
	MapValueType string // map 值类型
	MethodName   string
}

// Generator 代码生成器
type Generator struct {
	PackageName       string
	StructMappings    []StructMapping
	MapMappings       []MapMapping
	UsedPackages      map[string]string             // 包名 -> 导入路径
	allStructs        map[string]*parser.StructInfo // 所有结构体信息，用于递归生成
	generatedMappings map[string]bool               // 记录已生成的映射，避免重复
	mapDirection      string                        // map 转换方向配置
	mapValueType      string                        // map 值类型配置
}

// NewGenerator 创建新的生成器
func NewGenerator(packageName string) *Generator {
	return &Generator{
		PackageName:       packageName,
		StructMappings:    make([]StructMapping, 0),
		MapMappings:       make([]MapMapping, 0),
		UsedPackages:      make(map[string]string),
		allStructs:        make(map[string]*parser.StructInfo),
		generatedMappings: make(map[string]bool),
		mapDirection:      "both",
		mapValueType:      "any",
	}
}

// SetAllStructs 设置所有结构体信息，用于递归生成嵌套映射
func (g *Generator) SetAllStructs(allStructs map[string]*parser.StructInfo) {
	g.allStructs = allStructs
}

// SetMapConfig 设置 map 转换配置
func (g *Generator) SetMapConfig(direction, valueType string) {
	if direction != "" {
		g.mapDirection = direction
	}
	if valueType != "" {
		g.mapValueType = valueType
	}
}

// AddMapping 添加结构体映射
func (g *Generator) AddMapping(source, dest *parser.StructInfo) {
	mapping := StructMapping{
		Source:      source,
		Dest:        dest,
		FieldMaps:   g.buildFieldMappings(source, dest),
		MethodName:  fmt.Sprintf("%sTo%s", source.Name, dest.Name),
		NeedImports: make(map[string]string),
	}

	// 记录需要导入的包
	if source.Package != g.PackageName && source.ImportPath != "" {
		importAlias := g.getImportAlias(source.Package, source.ImportPath)
		mapping.NeedImports[importAlias] = source.ImportPath
		g.UsedPackages[importAlias] = source.ImportPath
	}

	if dest.Package != g.PackageName && dest.ImportPath != "" {
		importAlias := g.getImportAlias(dest.Package, dest.ImportPath)
		mapping.NeedImports[importAlias] = dest.ImportPath
		g.UsedPackages[importAlias] = dest.ImportPath
	}

	g.StructMappings = append(g.StructMappings, mapping)

	// 标记此映射为已生成
	mappingKey := fmt.Sprintf("%s.%s->%s.%s",
		source.Package, source.Name,
		dest.Package, dest.Name)
	g.generatedMappings[mappingKey] = true
}

// AddStructToMapMapping 添加结构体转 map 映射
func (g *Generator) AddStructToMapMapping(structInfo *parser.StructInfo) {
	mapping := MapMapping{
		Struct:       structInfo,
		Direction:    "to-map",
		MapValueType: g.mapValueType,
		MethodName:   fmt.Sprintf("%sToMap", structInfo.Name),
	}

	g.MapMappings = append(g.MapMappings, mapping)

	// 记录需要导入的包
	if structInfo.Package != g.PackageName && structInfo.ImportPath != "" {
		importAlias := g.getImportAlias(structInfo.Package, structInfo.ImportPath)
		g.UsedPackages[importAlias] = structInfo.ImportPath
	}
}

// AddMapToStructMapping 添加 map 转结构体映射
func (g *Generator) AddMapToStructMapping(structInfo *parser.StructInfo) {
	mapping := MapMapping{
		Struct:       structInfo,
		Direction:    "from-map",
		MapValueType: g.mapValueType,
		MethodName:   fmt.Sprintf("MapTo%s", structInfo.Name),
	}

	g.MapMappings = append(g.MapMappings, mapping)

	// 记录需要导入的包
	if structInfo.Package != g.PackageName && structInfo.ImportPath != "" {
		importAlias := g.getImportAlias(structInfo.Package, structInfo.ImportPath)
		g.UsedPackages[importAlias] = structInfo.ImportPath
	}
}

// getImportAlias 获取导入别名
func (g *Generator) getImportAlias(pkgName, importPath string) string {
	base := filepath.Base(importPath)
	if base == pkgName {
		return pkgName
	}

	for alias, path := range g.UsedPackages {
		if alias == pkgName && path != importPath {
			parts := strings.Split(importPath, "/")
			if len(parts) >= 2 {
				return parts[len(parts)-2] + "_" + parts[len(parts)-1]
			}
			return pkgName + "_ext"
		}
	}

	return pkgName
}

// buildFieldMappings 构建字段映射关系
func (g *Generator) buildFieldMappings(source, dest *parser.StructInfo) []FieldMapping {
	var mappings []FieldMapping

	// 1. 首先处理显式标签映射
	for _, sourceField := range source.Fields {
		if destField := dest.GetMapStructField(sourceField.Name); destField != nil {
			mappings = append(mappings, FieldMapping{
				SourceField: sourceField,
				DestField:   *destField,
				MappingType: "tag",
			})
		}
	}

	// 2. 处理 JSON 名称映射
	for _, sourceField := range source.Fields {
		if sourceField.JSONName == "" {
			continue
		}

		alreadyMapped := false
		for _, mapping := range mappings {
			if mapping.SourceField.Name == sourceField.Name {
				alreadyMapped = true
				break
			}
		}
		if alreadyMapped {
			continue
		}

		if destField := dest.GetFieldByJSONName(sourceField.JSONName); destField != nil {
			mappings = append(mappings, FieldMapping{
				SourceField: sourceField,
				DestField:   *destField,
				MappingType: "json",
			})
		}
	}

	// 3. 然后处理同名字段
	for _, sourceField := range source.Fields {
		alreadyMapped := false
		for _, mapping := range mappings {
			if mapping.SourceField.Name == sourceField.Name {
				alreadyMapped = true
				break
			}
		}
		if alreadyMapped {
			continue
		}

		if destField := dest.GetFieldByName(sourceField.Name); destField != nil {
			mappings = append(mappings, FieldMapping{
				SourceField: sourceField,
				DestField:   *destField,
				MappingType: "name",
			})
		}
	}

	return mappings
}

// Generate 生成代码
func (g *Generator) Generate() (string, error) {
	var builder strings.Builder

	// 文件头
	builder.WriteString("// Code generated by mapstruct. DO NOT EDIT.\n")
	builder.WriteString("// versions:\n")
	builder.WriteString("//   mapstruct v1.0.0\n\n")

	// 包声明
	if g.PackageName != "" {
		builder.WriteString(fmt.Sprintf("package %s\n\n", g.PackageName))
	} else if len(g.StructMappings) > 0 {
		builder.WriteString(fmt.Sprintf("package %s\n\n", g.StructMappings[0].Source.Package))
	} else if len(g.MapMappings) > 0 {
		builder.WriteString(fmt.Sprintf("package %s\n\n", g.MapMappings[0].Struct.Package))
	} else {
		builder.WriteString("package main\n\n")
	}

	// 导入语句
	if len(g.UsedPackages) > 0 {
		builder.WriteString("import (\n")
		for alias, importPath := range g.UsedPackages {
			if alias != "" && importPath != "" {
				basePkg := filepath.Base(importPath)
				if alias == basePkg {
					builder.WriteString(fmt.Sprintf("\t\"%s\"\n", importPath))
				} else {
					builder.WriteString(fmt.Sprintf("\t%s \"%s\"\n", alias, importPath))
				}
			}
		}
		builder.WriteString(")\n\n")
	}

	// 生成结构体映射函数
	for _, mapping := range g.StructMappings {
		builder.WriteString(g.generateMappingFunction(mapping))
		builder.WriteString("\n\n")
	}

	// 生成嵌套对象映射函数
	g.generateNestedMappings(&builder)

	// 生成 map 映射函数
	for _, mapping := range g.MapMappings {
		if mapping.Direction == "to-map" {
			builder.WriteString(g.generateStructToMapFunction(mapping))
		} else {
			builder.WriteString(g.generateMapToStructFunction(mapping))
		}
		builder.WriteString("\n\n")
	}

	return builder.String(), nil
}

// generateNestedMappings 生成所有嵌套对象的映射函数
func (g *Generator) generateNestedMappings(builder *strings.Builder) {
	toGenerate := make([]struct {
		source *parser.StructInfo
		dest   *parser.StructInfo
	}, 0)

	for _, mapping := range g.StructMappings {
		g.collectNestedMappings(mapping.Source, mapping.Dest, &toGenerate)
	}

	for _, pair := range toGenerate {
		mappingKey := fmt.Sprintf("%s.%s->%s.%s",
			pair.source.Package, pair.source.Name,
			pair.dest.Package, pair.dest.Name)

		if g.generatedMappings[mappingKey] {
			continue
		}

		newMapping := StructMapping{
			Source:      pair.source,
			Dest:        pair.dest,
			FieldMaps:   g.buildFieldMappings(pair.source, pair.dest),
			MethodName:  fmt.Sprintf("%sTo%s", pair.source.Name, pair.dest.Name),
			NeedImports: make(map[string]string),
		}

		if pair.source.Package != g.PackageName && pair.source.ImportPath != "" {
			importAlias := g.getImportAlias(pair.source.Package, pair.source.ImportPath)
			newMapping.NeedImports[importAlias] = pair.source.ImportPath
			g.UsedPackages[importAlias] = pair.source.ImportPath
		}

		if pair.dest.Package != g.PackageName && pair.dest.ImportPath != "" {
			importAlias := g.getImportAlias(pair.dest.Package, pair.dest.ImportPath)
			newMapping.NeedImports[importAlias] = pair.dest.ImportPath
			g.UsedPackages[importAlias] = pair.dest.ImportPath
		}

		builder.WriteString(g.generateMappingFunction(newMapping))
		builder.WriteString("\n\n")

		g.generatedMappings[mappingKey] = true
		g.collectNestedMappings(pair.source, pair.dest, &toGenerate)
	}
}

// collectNestedMappings 收集嵌套对象的映射对
func (g *Generator) collectNestedMappings(source, dest *parser.StructInfo, toGenerate *[]struct {
	source *parser.StructInfo
	dest   *parser.StructInfo
}) {
	existingMap := make(map[string]bool, len(*toGenerate))
	for _, pair := range *toGenerate {
		key := fmt.Sprintf("%s.%s->%s.%s",
			pair.source.Package, pair.source.Name,
			pair.dest.Package, pair.dest.Name)
		existingMap[key] = true
	}

	for _, sourceField := range source.Fields {
		if !sourceField.IsNestedObject || sourceField.NestedStructInfo == nil {
			continue
		}

		destField := dest.GetFieldByName(sourceField.Name)
		if destField == nil {
			if sourceField.JSONName != "" {
				destField = dest.GetFieldByJSONName(sourceField.JSONName)
			}
		}

		if destField == nil || !destField.IsNestedObject || destField.NestedStructInfo == nil {
			continue
		}

		nestedSource := sourceField.NestedStructInfo
		nestedDest := destField.NestedStructInfo

		if nestedSource != nil && nestedDest != nil {
			mappingKey := fmt.Sprintf("%s.%s->%s.%s",
				nestedSource.Package, nestedSource.Name,
				nestedDest.Package, nestedDest.Name)

			if existingMap[mappingKey] || g.generatedMappings[mappingKey] {
				continue
			}

			*toGenerate = append(*toGenerate, struct {
				source *parser.StructInfo
				dest   *parser.StructInfo
			}{
				source: nestedSource,
				dest:   nestedDest,
			})
			existingMap[mappingKey] = true
		}
	}
}

// generateMappingFunction 生成单个结构体映射函数
func (g *Generator) generateMappingFunction(mapping StructMapping) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("// %s 将 %s.%s 映射到 %s.%s\n",
		mapping.MethodName, mapping.Source.Package, mapping.Source.Name,
		mapping.Dest.Package, mapping.Dest.Name))

	sourceType := g.getQualifiedType(mapping.Source)
	destType := g.getQualifiedType(mapping.Dest)

	builder.WriteString(fmt.Sprintf("func %s(src *%s) *%s {\n",
		mapping.MethodName, sourceType, destType))

	builder.WriteString("\tif src == nil {\n")
	builder.WriteString("\t\treturn nil\n")
	builder.WriteString("\t}\n\n")

	builder.WriteString(fmt.Sprintf("\tdst := &%s{}\n\n", destType))

	for _, fieldMap := range mapping.FieldMaps {
		assignment := g.generateFieldAssignment(mapping, fieldMap)
		if assignment != "" {
			builder.WriteString(fmt.Sprintf("\t%s\n", assignment))
		}
	}

	builder.WriteString("\n\treturn dst\n")
	builder.WriteString("}\n")

	return builder.String()
}

// generateStructToMapFunction 生成结构体转 map 函数
func (g *Generator) generateStructToMapFunction(mapping MapMapping) string {
	var builder strings.Builder

	structType := g.getQualifiedTypeForMap(mapping.Struct)

	builder.WriteString(fmt.Sprintf("// %s 将 %s.%s 转换为 map[string]%s\n",
		mapping.MethodName, mapping.Struct.Package, mapping.Struct.Name, mapping.MapValueType))

	builder.WriteString(fmt.Sprintf("func %s(src *%s) map[string]%s {\n",
		mapping.MethodName, structType, mapping.MapValueType))

	builder.WriteString("\tif src == nil {\n")
	builder.WriteString("\t\treturn nil\n")
	builder.WriteString("\t}\n\n")

	builder.WriteString(fmt.Sprintf("\treturn map[string]%s{\n", mapping.MapValueType))

	for _, field := range mapping.Struct.Fields {
		mapKey := g.getMapKey(field)
		fieldAccess := g.getFieldAccess(field)
		valueExpr := g.generateMapValueExpr(field, "src."+fieldAccess)
		builder.WriteString(fmt.Sprintf("\t\t\"%s\": %s,\n", mapKey, valueExpr))
	}

	builder.WriteString("\t}\n")
	builder.WriteString("}\n")

	return builder.String()
}

// generateMapToStructFunction 生成 map 转结构体函数
func (g *Generator) generateMapToStructFunction(mapping MapMapping) string {
	var builder strings.Builder

	structType := g.getQualifiedTypeForMap(mapping.Struct)

	builder.WriteString(fmt.Sprintf("// %s 将 map[string]%s 转换为 %s.%s\n",
		mapping.MethodName, mapping.MapValueType, mapping.Struct.Package, mapping.Struct.Name))

	builder.WriteString(fmt.Sprintf("func %s(src map[string]%s) *%s {\n",
		mapping.MethodName, mapping.MapValueType, structType))

	builder.WriteString("\tif src == nil {\n")
	builder.WriteString("\t\treturn nil\n")
	builder.WriteString("\t}\n\n")

	builder.WriteString(fmt.Sprintf("\tdst := &%s{}\n\n", structType))

	for _, field := range mapping.Struct.Fields {
		mapKey := g.getMapKey(field)
		builder.WriteString(g.generateMapToFieldAssignment(field, mapKey))
	}

	builder.WriteString("\n\treturn dst\n")
	builder.WriteString("}\n")

	return builder.String()
}

// getMapKey 获取字段对应的 map key
// 优先使用 json tag，其次使用字段名（首字母小写）
func (g *Generator) getMapKey(field parser.FieldInfo) string {
	// 优先使用 json tag
	if field.JSONName != "" {
		return field.JSONName
	}

	// 使用字段名，首字母小写
	return toLowerFirst(field.Name)
}

// toLowerFirst 将字符串首字母小写
func toLowerFirst(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

// getFieldAccess 获取字段访问路径
func (g *Generator) getFieldAccess(field parser.FieldInfo) string {
	if len(field.EmbeddedPath) > 0 {
		return strings.Join(field.EmbeddedPath, ".") + "." + field.Name
	}
	return field.Name
}

// generateMapValueExpr 生成 map 值表达式
func (g *Generator) generateMapValueExpr(field parser.FieldInfo, fieldRef string) string {
	fieldType := field.Type

	// 指针类型处理
	if strings.HasPrefix(fieldType, "*") {
		baseType := strings.TrimPrefix(fieldType, "*")
		return fmt.Sprintf("func() %s { if %s != nil { return *%s }; return %s }()", g.mapValueType, fieldRef, fieldRef, g.getZeroValue(baseType))
	}

	// 基本类型直接返回
	return fieldRef
}

// generateMapToFieldAssignment 生成从 map 到字段的赋值代码
func (g *Generator) generateMapToFieldAssignment(field parser.FieldInfo, mapKey string) string {
	var builder strings.Builder

	fieldType := field.Type
	fieldRef := "dst." + g.getFieldAccess(field)

	// 检查 map 中是否存在该 key
	builder.WriteString(fmt.Sprintf("\tif val, ok := src[\"%s\"]; ok {\n", mapKey))

	// 根据字段类型生成转换代码
	switch {
	case fieldType == "string":
		builder.WriteString(fmt.Sprintf("\t\tif s, ok := val.(string); ok {\n"))
		builder.WriteString(fmt.Sprintf("\t\t\t%s = s\n", fieldRef))
		builder.WriteString("\t\t}\n")

	case fieldType == "int":
		builder.WriteString(fmt.Sprintf("\t\t%s = toInt(val)\n", fieldRef))
	case fieldType == "int8":
		builder.WriteString(fmt.Sprintf("\t\t%s = int8(toInt(val))\n", fieldRef))
	case fieldType == "int16":
		builder.WriteString(fmt.Sprintf("\t\t%s = int16(toInt(val))\n", fieldRef))
	case fieldType == "int32":
		builder.WriteString(fmt.Sprintf("\t\t%s = int32(toInt(val))\n", fieldRef))
	case fieldType == "int64":
		builder.WriteString(fmt.Sprintf("\t\t%s = int64(toInt(val))\n", fieldRef))

	case fieldType == "float32":
		builder.WriteString(fmt.Sprintf("\t\t%s = float32(toFloat(val))\n", fieldRef))
	case fieldType == "float64":
		builder.WriteString(fmt.Sprintf("\t\t%s = toFloat(val)\n", fieldRef))

	case fieldType == "bool":
		builder.WriteString(fmt.Sprintf("\t\tif b, ok := val.(bool); ok {\n"))
		builder.WriteString(fmt.Sprintf("\t\t\t%s = b\n", fieldRef))
		builder.WriteString("\t\t}\n")

	case strings.HasPrefix(fieldType, "[]"):
		builder.WriteString(fmt.Sprintf("\t\tif arr, ok := val.([]%s); ok {\n", g.mapValueType))
		builder.WriteString(fmt.Sprintf("\t\t\t%s = toStringSlice(arr)\n", fieldRef))
		builder.WriteString("\t\t}\n")

	case strings.HasPrefix(fieldType, "*"):
		// 指针类型
		baseType := strings.TrimPrefix(fieldType, "*")
		builder.WriteString(fmt.Sprintf("\t\t%s = toPointer(%s, val)\n", fieldRef, g.getTypeConversion(baseType)))

	default:
		builder.WriteString(fmt.Sprintf("\t\t%s = val\n", fieldRef))
	}

	builder.WriteString("\t}\n")

	return builder.String()
}

// getZeroValue 获取类型的零值
func (g *Generator) getZeroValue(typeName string) string {
	switch typeName {
	case "int", "int8", "int16", "int32", "int64":
		return "0"
	case "float32", "float64":
		return "0"
	case "bool":
		return "false"
	case "string":
		return "\"\""
	default:
		return "nil"
	}
}

// getTypeConversion 获取类型转换函数
func (g *Generator) getTypeConversion(typeName string) string {
	switch typeName {
	case "int":
		return "toInt"
	case "int8", "int16", "int32", "int64":
		return "toInt"
	case "float32", "float64":
		return "toFloat"
	case "string":
		return "toString"
	case "bool":
		return "toBool"
	default:
		return ""
	}
}

// getQualifiedType 获取限定类型名（包含包名）
func (g *Generator) getQualifiedType(structInfo *parser.StructInfo) string {
	if structInfo.Package == g.PackageName || structInfo.Package == "" {
		return structInfo.Name
	}

	for alias, importPath := range g.UsedPackages {
		basePkg := filepath.Base(importPath)
		if alias == structInfo.Package || basePkg == structInfo.Package {
			return alias + "." + structInfo.Name
		}
	}

	return structInfo.Package + "." + structInfo.Name
}

// getQualifiedTypeForMap 获取用于 map 转换的限定类型名
func (g *Generator) getQualifiedTypeForMap(structInfo *parser.StructInfo) string {
	return g.getQualifiedType(structInfo)
}

// generateFieldAssignment 生成字段赋值语句
func (g *Generator) generateFieldAssignment(mapping StructMapping, fieldMap FieldMapping) string {
	sourceType := fieldMap.SourceField.Type
	destType := fieldMap.DestField.Type

	var sourceFieldPath []string
	if len(fieldMap.SourceField.EmbeddedPath) > 0 {
		sourceFieldPath = append(sourceFieldPath, fieldMap.SourceField.EmbeddedPath...)
	}
	sourceFieldPath = append(sourceFieldPath, fieldMap.SourceField.Name)
	sourceFieldRef := "src." + strings.Join(sourceFieldPath, ".")

	var destFieldPath []string
	if len(fieldMap.DestField.EmbeddedPath) > 0 {
		destFieldPath = append(destFieldPath, fieldMap.DestField.EmbeddedPath...)
	}
	destFieldPath = append(destFieldPath, fieldMap.DestField.Name)
	destFieldRef := "dst." + strings.Join(destFieldPath, ".")

	// 如果是嵌套对象，生成递归调用
	if fieldMap.SourceField.IsNestedObject && fieldMap.DestField.IsNestedObject {
		methodName := fmt.Sprintf("%sTo%s",
			fieldMap.SourceField.NestedStructInfo.Name,
			fieldMap.DestField.NestedStructInfo.Name)

		if strings.HasPrefix(sourceType, "*") && strings.HasPrefix(destType, "*") {
			return fmt.Sprintf("%s = %s(%s)", destFieldRef, methodName, sourceFieldRef)
		}

		if strings.HasPrefix(sourceType, "*") && !strings.HasPrefix(destType, "*") {
			return fmt.Sprintf("if %s != nil {\n\t\ttmp := %s(%s)\n\t\tif tmp != nil {\n\t\t\t%s = *tmp\n\t\t}\n\t}",
				sourceFieldRef, methodName, sourceFieldRef, destFieldRef)
		}

		if !strings.HasPrefix(sourceType, "*") && strings.HasPrefix(destType, "*") {
			return fmt.Sprintf("%s = %s(&%s)", destFieldRef, methodName, sourceFieldRef)
		}

		return fmt.Sprintf("%s = *%s(&%s)", destFieldRef, methodName, sourceFieldRef)
	}

	// 类型完全匹配
	if sourceType == destType {
		return fmt.Sprintf("%s = %s", destFieldRef, sourceFieldRef)
	}

	// 指针类型处理
	if strings.HasPrefix(sourceType, "*") && !strings.HasPrefix(destType, "*") {
		baseType := strings.TrimPrefix(sourceType, "*")
		if baseType == destType {
			return fmt.Sprintf("if %s != nil {\n\t\t%s = *%s\n\t}",
				sourceFieldRef, destFieldRef, sourceFieldRef)
		}
	}

	if !strings.HasPrefix(sourceType, "*") && strings.HasPrefix(destType, "*") {
		baseType := strings.TrimPrefix(destType, "*")
		if sourceType == baseType {
			return fmt.Sprintf("%s = &%s", destFieldRef, sourceFieldRef)
		}
	}

	// 基本类型转换
	if g.isConvertibleType(sourceType, destType) {
		return fmt.Sprintf("%s = %s(%s)", destFieldRef, destType, sourceFieldRef)
	}

	// 时间类型转换
	if sourceType == "time.Time" && destType == "string" {
		return fmt.Sprintf("%s = %s.Format(\"2006-01-02 15:04:05\")",
			destFieldRef, sourceFieldRef)
	}

	if sourceType == "string" && destType == "time.Time" {
		return fmt.Sprintf("if t, err := time.Parse(\"2006-01-02 15:04:05\", %s); err == nil {\n\t\t%s = t\n\t}",
			sourceFieldRef, destFieldRef)
	}

	// 跨包类型转换
	if g.isSameTypeDifferentPackage(sourceType, destType, mapping) {
		return fmt.Sprintf("%s = %s", destFieldRef, sourceFieldRef)
	}

	// 无法自动转换，生成注释
	return fmt.Sprintf("// TODO: 手动实现字段映射 %s.%s -> %s.%s: %s -> %s",
		mapping.Source.Package, fieldMap.SourceField.Name,
		mapping.Dest.Package, fieldMap.DestField.Name,
		sourceType, destType)
}

// isConvertibleType 检查是否可转换的基本类型
func (g *Generator) isConvertibleType(sourceType, destType string) bool {
	convertiblePairs := map[string][]string{
		"int":     {"int8", "int16", "int32", "int64", "float32", "float64"},
		"int8":    {"int", "int16", "int32", "int64", "float32", "float64"},
		"int16":   {"int", "int8", "int32", "int64", "float32", "float64"},
		"int32":   {"int", "int8", "int16", "int64", "float32", "float64"},
		"int64":   {"int", "int8", "int16", "int32", "float32", "float64"},
		"float32": {"int", "int8", "int16", "int32", "int64", "float64"},
		"float64": {"int", "int8", "int16", "int32", "int64", "float32"},
		"string":  {"[]byte"},
		"[]byte":  {"string"},
	}

	allowedDests, exists := convertiblePairs[sourceType]
	if !exists {
		return false
	}

	for _, allowed := range allowedDests {
		if allowed == destType {
			return true
		}
	}
	return false
}

// isSameTypeDifferentPackage 检查是否是同类型但在不同包
func (g *Generator) isSameTypeDifferentPackage(sourceType, destType string, mapping StructMapping) bool {
	sourceTypeName := g.getTypeNameWithoutPackage(sourceType)
	destTypeName := g.getTypeNameWithoutPackage(destType)

	return sourceTypeName == destTypeName && sourceTypeName != ""
}

// getTypeNameWithoutPackage 获取不包含包名的类型名
func (g *Generator) getTypeNameWithoutPackage(typeName string) string {
	parts := strings.Split(typeName, ".")
	if len(parts) > 1 {
		return parts[1]
	}
	return parts[0]
}
