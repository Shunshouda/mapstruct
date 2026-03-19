package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/shunshouda/mapstruct/generator"
	parser2 "github.com/shunshouda/mapstruct/parser"
	"golang.org/x/tools/go/packages"
)

var (
	typeNames    = flag.String("type", "", "逗号分隔的类型名称对，格式为:package.Source:package.Dest 或 package.Source:map 或 map:package.Dest")
	output       = flag.String("output", "", "输出文件名")
	packageName  = flag.String("package", "", "生成的包名")
	includeDirs  = flag.String("include", "", "逗号分隔的要包含的目录")
	dependencies = flag.String("dependency", "", "逗号分隔的需要解析的依赖包路径")
	verbose      = flag.Bool("verbose", false, "显示详细日志")
	modulePath   = flag.String("module", "", "Go Module 路径，如：github.com/user/project")
	mapDirection = flag.String("map-direction", "both", "map转换方向: to-map(结构体转map), from-map(map转结构体), both(双向)")
	mapValueType = flag.String("map-value-type", "any", "map的值类型: any, interface{}")
)

func main() {
	flag.Parse()

	if len(*typeNames) == 0 {
		flag.Usage()
		log.Fatal("必须指定 -type 参数")
	}

	// 解析类型对
	typePairs := parseTypePairs(*typeNames)
	if len(typePairs) == 0 {
		log.Fatal("未找到有效的类型对")
	}

	// 确定输出文件
	outputFile := getOutputFile(*output)

	// 获取要扫描的目录
	scanDirs := getScanDirs(*includeDirs)

	// 获取依赖包列表
	deps := getDependencies(*dependencies)

	// 自动检测 module path
	if *modulePath == "" {
		*modulePath = detectModulePath()
	}
	if *verbose && *modulePath != "" {
		log.Printf("检测到模块路径：%s", *modulePath)
	}

	// 收集所有结构体信息（包括依赖包）
	structInfos := collectStructInfos(scanDirs, deps, *modulePath)

	// 生成代码
	gen := generator.NewGenerator(*packageName)
	gen.SetAllStructs(structInfos)
	gen.SetMapConfig(*mapDirection, *mapValueType)

	for _, pair := range typePairs {
		// 检查是否是 map 转换
		if pair.IsMapConversion {
			handleMapConversion(gen, pair, structInfos)
			continue
		}

		// 普通结构体映射
		handleStructMapping(gen, pair, structInfos)
	}

	// 写入文件
	code, err := gen.Generate()
	if err != nil {
		log.Fatal("生成代码失败:", err)
	}

	if err := os.WriteFile(outputFile, []byte(code), 0644); err != nil {
		log.Fatal("写入文件失败:", err)
	}

	fmt.Printf("成功生成映射代码到：%s\n", outputFile)
}

// handleMapConversion 处理 map 转换
func handleMapConversion(gen *generator.Generator, pair generator.TypePair, structInfos map[string]*parser2.StructInfo) {
	if pair.Direction == "to-map" || pair.Direction == "both" {
		// 结构体转 map
		var structInfo *parser2.StructInfo
		var exists bool

		keys := []string{
			fmt.Sprintf("%s.%s", pair.SourcePkg, pair.SourceType),
			pair.SourceType,
		}

		for _, key := range keys {
			if info, ok := structInfos[key]; ok {
				structInfo = info
				exists = true
				break
			}
		}

		if !exists {
			log.Printf("警告：未找到源结构体 %s", pair.SourceType)
			return
		}

		if *verbose {
			log.Printf("映射：%s.%s -> map[string]%s",
				structInfo.Package, structInfo.Name, pair.MapValueType)
		}

		gen.AddStructToMapMapping(structInfo)
	}

	if pair.Direction == "from-map" || pair.Direction == "both" {
		// map 转结构体
		var structInfo *parser2.StructInfo
		var exists bool

		keys := []string{
			fmt.Sprintf("%s.%s", pair.DestPkg, pair.DestType),
			pair.DestType,
		}

		for _, key := range keys {
			if info, ok := structInfos[key]; ok {
				structInfo = info
				exists = true
				break
			}
		}

		if !exists {
			log.Printf("警告：未找到目标结构体 %s", pair.DestType)
			return
		}

		if *verbose {
			log.Printf("映射：map[string]%s -> %s.%s",
				pair.MapValueType, structInfo.Package, structInfo.Name)
		}

		gen.AddMapToStructMapping(structInfo)
	}
}

// handleStructMapping 处理普通结构体映射
func handleStructMapping(gen *generator.Generator, pair generator.TypePair, structInfos map[string]*parser2.StructInfo) {
	sourceKeys := []string{
		fmt.Sprintf("%s.%s", pair.SourcePkg, pair.SourceType),
		pair.SourceType,
	}

	destKeys := []string{
		fmt.Sprintf("%s.%s", pair.DestPkg, pair.DestType),
		pair.DestType,
	}

	var sourceInfo, destInfo *parser2.StructInfo
	var sourceExists, destExists bool

	for _, key := range sourceKeys {
		if info, ok := structInfos[key]; ok {
			sourceInfo = info
			sourceExists = true
			break
		}
	}

	for _, key := range destKeys {
		if info, ok := structInfos[key]; ok {
			destInfo = info
			destExists = true
			break
		}
	}

	if !sourceExists {
		log.Printf("警告：未找到源结构体 %s", pair.SourceType)
		return
	}
	if !destExists {
		log.Printf("警告：未找到目标结构体 %s", pair.DestType)
		return
	}

	if *verbose {
		log.Printf("映射：%s.%s -> %s.%s",
			sourceInfo.Package, sourceInfo.Name,
			destInfo.Package, destInfo.Name)
	}

	gen.AddMapping(sourceInfo, destInfo)
}

// 解析类型对 (支持 package.Type 格式 和 map 转换)
func parseTypePairs(input string) []generator.TypePair {
	var pairs []generator.TypePair
	typeStrs := strings.Split(input, ",")

	for _, typeStr := range typeStrs {
		parts := strings.Split(typeStr, ":")
		if len(parts) != 2 {
			continue
		}

		sourcePart := strings.TrimSpace(parts[0])
		destPart := strings.TrimSpace(parts[1])

		// 检查是否是 map 转换
		if isMapType(destPart) {
			// 结构体转 map: Source:map 或 Source:map[string]any
			pair := parseStructToMap(sourcePart, destPart)
			pairs = append(pairs, pair)
			continue
		}

		if isMapType(sourcePart) {
			// map 转结构体: map:Dest 或 map[string]any:Dest
			pair := parseMapToStruct(sourcePart, destPart)
			pairs = append(pairs, pair)
			continue
		}

		// 普通结构体映射
		pair := parseStructPair(sourcePart, destPart)
		if pair.SourceType != "" {
			pairs = append(pairs, pair)
		}
	}
	return pairs
}

// isMapType 检查是否是 map 类型
func isMapType(typeStr string) bool {
	return typeStr == "map" || strings.HasPrefix(typeStr, "map[")
}

// parseStructToMap 解析结构体转 map
func parseStructToMap(sourcePart, destPart string) generator.TypePair {
	sourceParts := strings.Split(sourcePart, ".")
	mapValueType := "any"

	// 解析 map 值类型
	if strings.HasPrefix(destPart, "map[string]") {
		mapValueType = strings.TrimPrefix(destPart, "map[string]")
		if mapValueType == "" {
			mapValueType = "any"
		}
	}

	pair := generator.TypePair{
		IsMapConversion: true,
		Direction:       "to-map",
		MapValueType:    mapValueType,
	}

	if len(sourceParts) == 2 {
		pair.SourcePkg = sourceParts[0]
		pair.SourceType = sourceParts[1]
	} else {
		pair.SourceType = sourceParts[0]
	}

	return pair
}

// parseMapToStruct 解析 map 转结构体
func parseMapToStruct(sourcePart, destPart string) generator.TypePair {
	destParts := strings.Split(destPart, ".")
	mapValueType := "any"

	// 解析 map 值类型
	if strings.HasPrefix(sourcePart, "map[string]") {
		mapValueType = strings.TrimPrefix(sourcePart, "map[string]")
		if mapValueType == "" {
			mapValueType = "any"
		}
	}

	pair := generator.TypePair{
		IsMapConversion: true,
		Direction:       "from-map",
		MapValueType:    mapValueType,
	}

	if len(destParts) == 2 {
		pair.DestPkg = destParts[0]
		pair.DestType = destParts[1]
	} else {
		pair.DestType = destParts[0]
	}

	return pair
}

// parseStructPair 解析普通结构体对
func parseStructPair(sourcePart, destPart string) generator.TypePair {
	sourceParts := strings.Split(sourcePart, ".")
	destParts := strings.Split(destPart, ".")

	pair := generator.TypePair{}

	if len(sourceParts) == 2 && len(destParts) == 2 {
		pair.SourcePkg = sourceParts[0]
		pair.SourceType = sourceParts[1]
		pair.DestPkg = destParts[0]
		pair.DestType = destParts[1]
	} else if len(sourceParts) == 2 && len(destParts) == 1 {
		pair.SourcePkg = sourceParts[0]
		pair.SourceType = sourceParts[1]
		pair.DestType = destParts[0]
	} else if len(sourceParts) == 1 && len(destParts) == 2 {
		pair.SourceType = sourceParts[0]
		pair.DestPkg = destParts[0]
		pair.DestType = destParts[1]
	} else if len(sourceParts) == 1 && len(destParts) == 1 {
		pair.SourceType = sourceParts[0]
		pair.DestType = destParts[0]
	}

	return pair
}

// 获取输出文件名
func getOutputFile(output string) string {
	if output != "" {
		return output
	}
	return "generated_mapstruct.go"
}

// 获取要扫描的目录
func getScanDirs(includeDirs string) []string {
	if includeDirs == "" {
		return []string{"."}
	}

	dirs := strings.Split(includeDirs, ",")
	for i, dir := range dirs {
		dirs[i] = strings.TrimSpace(dir)
	}
	return dirs
}

// 获取依赖包列表
func getDependencies(deps string) []string {
	if deps == "" {
		return []string{}
	}

	depList := strings.Split(deps, ",")
	for i, dep := range depList {
		depList[i] = strings.TrimSpace(dep)
	}
	return depList
}

// 检测 Go Module 路径
func detectModulePath() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}

	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			content, err := os.ReadFile(goModPath)
			if err != nil {
				return ""
			}

			lines := strings.Split(string(content), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "module ") {
					return strings.TrimSpace(strings.TrimPrefix(line, "module "))
				}
			}
			return ""
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return ""
}

// 收集所有目录的结构体信息
func collectStructInfos(scanDirs []string, dependencies []string, modulePath string) map[string]*parser2.StructInfo {
	structInfos := make(map[string]*parser2.StructInfo)
	fset := token.NewFileSet()

	parsedDirs := make(map[string]map[string]*ast.Package)

	for _, dir := range scanDirs {
		if *verbose {
			log.Printf("扫描目录：%s", dir)
		}

		if _, exists := parsedDirs[dir]; exists {
			continue
		}

		pkgs, err := parser.ParseDir(fset, dir, nil, parser.ParseComments)
		if err != nil {
			log.Printf("警告：解析目录 %s 失败：%v", dir, err)
			continue
		}
		parsedDirs[dir] = pkgs

		for pkgName, pkg := range pkgs {
			for fileName, file := range pkg.Files {
				if *verbose {
					log.Printf("  解析文件：%s", fileName)
				}

				filePkg := pkgName
				if file.Name != nil {
					filePkg = file.Name.Name
				}

				importPath := calculateImportPath(fileName, modulePath, dir)

				ast.Inspect(file, func(n ast.Node) bool {
					switch x := n.(type) {
					case *ast.TypeSpec:
						if structType, ok := x.Type.(*ast.StructType); ok {
							structName := x.Name.Name

							keys := []string{
								fmt.Sprintf("%s.%s", filePkg, structName),
								structName,
							}

							structInfo := parser2.ParseStruct(structName, structType, file, nil)
							structInfo.Package = filePkg
							structInfo.FilePath = fileName
							structInfo.ImportPath = importPath

							for _, key := range keys {
								structInfos[key] = structInfo
							}

							if *verbose {
								log.Printf("    找到结构体：%s (包：%s)", structName, filePkg)
							}
						}
					}
					return true
				})
			}
		}
	}

	if len(dependencies) > 0 {
		loadDependencyStructs(dependencies, structInfos, fset, *verbose)
	}

	for key, info := range structInfos {
		if isDependencyStruct(info.ImportPath, modulePath) {
			continue
		}

		for _, dir := range scanDirs {
			pkgs, exists := parsedDirs[dir]
			if !exists {
				continue
			}

			for _, pkg := range pkgs {
				for fileName, file := range pkg.Files {
					if fileName != info.FilePath {
						continue
					}

					ast.Inspect(file, func(n ast.Node) bool {
						switch x := n.(type) {
						case *ast.TypeSpec:
							if structType, ok := x.Type.(*ast.StructType); ok {
								if x.Name.Name == info.Name {
									newInfo := parser2.ParseStruct(info.Name, structType, file, structInfos)
									newInfo.Package = info.Package
									newInfo.FilePath = info.FilePath
									newInfo.ImportPath = info.ImportPath
									structInfos[key] = newInfo
								}
							}
						}
						return true
					})
				}
			}
		}
	}

	return structInfos
}

// loadDependencyStructs 从依赖包中加载结构体信息
func loadDependencyStructs(dependencies []string, structInfos map[string]*parser2.StructInfo, fset *token.FileSet, verbose bool) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedSyntax,
		Fset: fset,
	}

	for _, dep := range dependencies {
		if verbose {
			log.Printf("加载依赖包：%s", dep)
		}

		pkgs, err := packages.Load(cfg, dep)
		if err != nil {
			log.Printf("警告：加载依赖包 %s 失败：%v", dep, err)
			continue
		}

		for _, pkg := range pkgs {
			if verbose {
				log.Printf("  解析包：%s", pkg.PkgPath)
			}

			for _, syntax := range pkg.Syntax {
				filePkg := pkg.Name
				importPath := pkg.PkgPath

				ast.Inspect(syntax, func(n ast.Node) bool {
					switch x := n.(type) {
					case *ast.TypeSpec:
						if structType, ok := x.Type.(*ast.StructType); ok {
							structName := x.Name.Name

							keys := []string{
								fmt.Sprintf("%s.%s", filePkg, structName),
								structName,
								importPath + "." + structName,
							}

							structInfo := parser2.ParseStruct(structName, structType, syntax, structInfos)
							structInfo.Package = filePkg
							structInfo.FilePath = pkg.GoFiles[0]
							structInfo.ImportPath = importPath

							for _, key := range keys {
								structInfos[key] = structInfo
							}

							if verbose {
								log.Printf("    找到依赖结构体：%s", structName)
							}
						}
					}
					return true
				})
			}
		}
	}
}

// isDependencyStruct 判断是否是依赖包的结构体
func isDependencyStruct(importPath, modulePath string) bool {
	if importPath == "" || modulePath == "" {
		return false
	}
	return !strings.HasPrefix(importPath, modulePath)
}

// 计算完整的导入路径
func calculateImportPath(filePath, modulePath, baseDir string) string {
	if modulePath == "" {
		relPath, err := filepath.Rel(baseDir, filepath.Dir(filePath))
		if err != nil {
			return filepath.Dir(filePath)
		}
		if relPath == "." {
			return ""
		}
		return relPath
	}

	absFilePath, err := filepath.Abs(filepath.Dir(filePath))
	if err != nil {
		return modulePath
	}

	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return modulePath
	}

	moduleRoot := findModuleRoot(absBaseDir)
	if moduleRoot == "" {
		return modulePath
	}

	relPath, err := filepath.Rel(moduleRoot, absFilePath)
	if err != nil {
		return modulePath
	}

	if relPath == "." {
		return modulePath
	}

	return modulePath + "/" + relPath
}

// 查找包含 go.mod 的模块根目录
func findModuleRoot(startDir string) string {
	dir := startDir
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}
