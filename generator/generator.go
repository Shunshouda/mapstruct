package generator

import (
	"fmt"
	"path/filepath"
	"reflect"
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
	methodNames       map[string]string             // 映射键 -> 最终函数名
	usedMethodNames   map[string]string             // 函数名 -> 映射键(冲突检测)
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
		methodNames:       make(map[string]string),
		usedMethodNames:   make(map[string]string),
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

// resolveMethodName 生成全局唯一的转换函数名。默认 SrcToDst;不同映射对
// (源/目标包不同但类型同名)撞名时,用目标包名(首字母大写)限定消歧,
// 如 ShopOpenCloseTimeRangeToInfraShopOpenCloseTimeRange。同一映射键
// 多次调用返回同一名字(定义处与调用处一致)。
func (g *Generator) resolveMethodName(source, dest *parser.StructInfo) string {
	key := fmt.Sprintf("%s.%s->%s.%s",
		source.Package, source.Name,
		dest.Package, dest.Name)
	if name, ok := g.methodNames[key]; ok {
		return name
	}
	name := fmt.Sprintf("%sTo%s", source.Name, dest.Name)
	if other, taken := g.usedMethodNames[name]; taken && other != key {
		name = fmt.Sprintf("%sTo%s%s", source.Name, exportPackageName(dest.Package), dest.Name)
	}
	if other, taken := g.usedMethodNames[name]; taken && other != key {
		name = fmt.Sprintf("%s%sTo%s%s",
			exportPackageName(source.Package), source.Name,
			exportPackageName(dest.Package), dest.Name)
	}
	g.methodNames[key] = name
	g.usedMethodNames[name] = key
	return name
}

// exportPackageName 包名首字母大写(用于函数名限定)。
func exportPackageName(pkg string) string {
	if pkg == "" {
		return ""
	}
	r := []rune(pkg)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// AddMapping 添加结构体映射
func (g *Generator) AddMapping(source, dest *parser.StructInfo) {
	mapping := StructMapping{
		Source:      source,
		Dest:        dest,
		FieldMaps:   g.buildFieldMappings(source, dest),
		MethodName:  g.resolveMethodName(source, dest),
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

	// destIgnored dest 字段带 mapstruct:"-"(派生字段由服务层回填)时不参与匹配,
	// 与 struct↔map 方向的忽略语义保持一致。
	destIgnored := func(destField *parser.FieldInfo) *parser.FieldInfo {
		if destField != nil && hasMapStructIgnoreTag(destField.Tag) {
			return nil
		}
		return destField
	}

	// 1. 首先处理显式标签映射
	for _, sourceField := range source.Fields {
		if destField := destIgnored(dest.GetMapStructField(sourceField.Name)); destField != nil {
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

		if destField := destIgnored(dest.GetFieldByJSONName(sourceField.JSONName)); destField != nil {
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

		if destField := destIgnored(dest.GetFieldByName(sourceField.Name)); destField != nil {
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
	// 先生成全部函数体：generateNestedMappings 会随着深入注册新的依赖包，
	// 必须在输出 import 块之前完成，否则会漏 import
	var body strings.Builder

	// 生成结构体映射函数
	for _, mapping := range g.StructMappings {
		body.WriteString(g.generateMappingFunction(mapping))
		body.WriteString("\n\n")
	}

	// 生成嵌套对象映射函数
	g.generateNestedMappings(&body)

	// 生成 map 映射函数
	for _, mapping := range g.MapMappings {
		if mapping.Direction == "to-map" {
			body.WriteString(g.generateStructToMapFunction(mapping))
		} else {
			body.WriteString(g.generateMapToStructFunction(mapping))
		}
		body.WriteString("\n\n")
	}

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

	// 函数体
	builder.WriteString(body.String())

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

	// 用下标遍历而不是 range：collectNestedMappings 会在遍历过程中向 toGenerate
	// 追加更深层嵌套的映射对（range 在开始时求值一次，追加的项不会被访问）
	for i := 0; i < len(toGenerate); i++ {
		pair := toGenerate[i]

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
			MethodName:  g.resolveMethodName(pair.source, pair.dest),
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

// resolveNested 按包名.类型名重新解析嵌套结构体的最新定义。
// 解析器的两轮细化会替换 allStructs 中的条目，字段上缓存的 NestedStructInfo
// 可能是细化前的旧快照（其嵌套字段未解析），重新查一次拿到最终版本
func (g *Generator) resolveNested(info *parser.StructInfo) *parser.StructInfo {
	if info == nil {
		return nil
	}

	keys := make([]string, 0, 3)
	if info.Package != "" {
		keys = append(keys, info.Package+"."+info.Name)
	}
	if info.ImportPath != "" {
		keys = append(keys, info.ImportPath+"."+info.Name)
	}
	keys = append(keys, info.Name)

	for _, key := range keys {
		if st, ok := g.allStructs[key]; ok {
			return st
		}
	}

	return info
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

		nestedSource := g.resolveNested(sourceField.NestedStructInfo)
		nestedDest := g.resolveNested(destField.NestedStructInfo)

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

	builder.WriteString(fmt.Sprintf("\tresult := make(map[string]%s)\n", mapping.MapValueType))

	for _, field := range mapping.Struct.Fields {
		mapKey := g.getMapKey(field)
		// 如果 mapKey 为空，表示该字段被忽略（mapstruct:"-"）
		if mapKey == "" {
			continue
		}
		fieldRef := "src." + g.getFieldAccess(field)
		// 指针字段：nil 表示"未提供"，key 不写入 map。ent 的 SetField 不接受 nil，
		// 显式清空须由调用方走 ClearField 通道，故仅拷贝非空值。
		if strings.HasPrefix(field.Type, "*") {
			builder.WriteString(fmt.Sprintf("\tif %s != nil {\n\t\tresult[\"%s\"] = *%s\n\t}\n", fieldRef, mapKey, fieldRef))
			continue
		}
		builder.WriteString(fmt.Sprintf("\tresult[\"%s\"] = %s\n", mapKey, fieldRef))
	}

	builder.WriteString("\treturn result\n")
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
		// 如果 mapKey 为空，表示该字段被忽略（mapstruct:"-"）
		if mapKey == "" {
			continue
		}
		builder.WriteString(g.generateMapToFieldAssignment(field, mapKey))
	}

	builder.WriteString("\n\treturn dst\n")
	builder.WriteString("}\n")

	return builder.String()
}

// getMapKey 获取字段对应的 map key
// 优先使用 mapstruct tag，其次是 json tag，最后使用字段名（首字母小写）
func (g *Generator) getMapKey(field parser.FieldInfo) string {
	// 优先使用 mapstruct tag
	if hasMapStructIgnoreTag(field.Tag) {
		// 如果 mapstruct 标签是 "-", 返回空字符串表示忽略该字段
		return ""
	}

	if mapStructKey := getMapStructKey(field.Tag); mapStructKey != "" {
		return mapStructKey
	}

	// 其次使用 json tag
	if field.JSONName != "" {
		return field.JSONName
	}

	// 使用字段名，首字母小写
	return toLowerFirst(field.Name)
}

// getMapStructKey 获取 mapstruct 标签的值
func getMapStructKey(tag string) string {
	if tag == "" {
		return ""
	}

	structTag := reflect.StructTag(tag)
	mapStructTag := structTag.Get("mapstruct")
	if mapStructTag == "" || mapStructTag == "-" {
		return ""
	}
	return mapStructTag
}

// hasMapStructIgnoreTag 检查字段是否有 mapstruct:"-" 标签
func hasMapStructIgnoreTag(tag string) bool {
	if tag == "" {
		return false
	}

	structTag := reflect.StructTag(tag)
	mapStructTag := structTag.Get("mapstruct")
	return mapStructTag == "-"
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
		// 与 collectNestedMappings 注册的定义名保持同源(resolveNested 取
		// 两轮细化后的最终定义,resolveMethodName 保证定义/调用一致)。
		methodName := g.resolveMethodName(
			g.resolveNested(fieldMap.SourceField.NestedStructInfo),
			g.resolveNested(fieldMap.DestField.NestedStructInfo))

		// 切片类型的嵌套对象：逐元素生成转换循环
		if isSliceType(sourceType) && isSliceType(destType) {
			return g.generateSliceNestedAssignment(destFieldRef, sourceFieldRef,
				sourceType, destType, methodName, fieldMap)
		}

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

// isSliceType 判断是否是切片类型
func isSliceType(t string) bool {
	return strings.HasPrefix(t, "[]")
}

// sliceElemIsPointer 判断切片元素是否为指针类型
func sliceElemIsPointer(sliceType string) bool {
	return strings.HasPrefix(strings.TrimPrefix(sliceType, "[]"), "*")
}

// generateSliceNestedAssignment 生成切片嵌套对象的逐元素转换代码，
// 覆盖源/目标元素指针形态的四种组合（nil 切片保持 nil）
func (g *Generator) generateSliceNestedAssignment(destFieldRef, sourceFieldRef,
	sourceType, destType, methodName string, fieldMap FieldMapping) string {

	srcElemPtr := sliceElemIsPointer(sourceType)
	dstElemPtr := sliceElemIsPointer(destType)

	destSliceType := "[]"
	if dstElemPtr {
		destSliceType += "*"
	}
	destSliceType += g.getQualifiedType(fieldMap.DestField.NestedStructInfo)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("if %s != nil {\n", sourceFieldRef))
	b.WriteString(fmt.Sprintf("\t\t%s = make(%s, len(%s))\n", destFieldRef, destSliceType, sourceFieldRef))

	switch {
	case srcElemPtr && dstElemPtr:
		// []*T -> []*T，nil 元素经转换函数的 nil 保护后仍为 nil
		b.WriteString(fmt.Sprintf("\t\tfor i, item := range %s {\n", sourceFieldRef))
		b.WriteString(fmt.Sprintf("\t\t\t%s[i] = %s(item)\n", destFieldRef, methodName))
		b.WriteString("\t\t}\n")
	case srcElemPtr && !dstElemPtr:
		// []*T -> []T，跳过 nil 元素
		b.WriteString(fmt.Sprintf("\t\tfor i, item := range %s {\n", sourceFieldRef))
		b.WriteString("\t\t\tif item != nil {\n")
		b.WriteString(fmt.Sprintf("\t\t\t\tif tmp := %s(item); tmp != nil {\n", methodName))
		b.WriteString(fmt.Sprintf("\t\t\t\t\t%s[i] = *tmp\n", destFieldRef))
		b.WriteString("\t\t\t\t}\n")
		b.WriteString("\t\t\t}\n")
		b.WriteString("\t\t}\n")
	case !srcElemPtr && dstElemPtr:
		// []T -> []*T
		b.WriteString(fmt.Sprintf("\t\tfor i := range %s {\n", sourceFieldRef))
		b.WriteString(fmt.Sprintf("\t\t\t%s[i] = %s(&%s[i])\n", destFieldRef, methodName, sourceFieldRef))
		b.WriteString("\t\t}\n")
	default:
		// []T -> []T
		b.WriteString(fmt.Sprintf("\t\tfor i := range %s {\n", sourceFieldRef))
		b.WriteString(fmt.Sprintf("\t\t\t%s[i] = *%s(&%s[i])\n", destFieldRef, methodName, sourceFieldRef))
		b.WriteString("\t\t}\n")
	}

	b.WriteString("\t}")
	return b.String()
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

// getTypeNameWithoutPackage 去掉命名类型的包限定符，保留指针和切片形状。
// []infra.Source 与 []Source 是同一切片；Source、*Source、[]Source彼此不同。
func (g *Generator) getTypeNameWithoutPackage(typeName string) string {
	prefix := ""
	for {
		switch {
		case strings.HasPrefix(typeName, "*"):
			prefix += "*"
			typeName = strings.TrimPrefix(typeName, "*")
		case strings.HasPrefix(typeName, "[]"):
			prefix += "[]"
			typeName = strings.TrimPrefix(typeName, "[]")
		default:
			parts := strings.Split(typeName, ".")
			return prefix + parts[len(parts)-1]
		}
	}
}
