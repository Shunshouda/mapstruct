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

		// 如果 DestType 为空，说明配置无效
		if pair.DestType == "" {
			log.Printf("警告：无效的 map 转换配置，目标结构体类型为空")
			return
		}

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
			log.Printf("警告：未找到目标结构体 %s (尝试的键：%v)", pair.DestType, keys)
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
	// 优先使用带包名的完整路径查找
	sourceKeys := []string{}
	if pair.SourcePkg != "" {
		sourceKeys = append(sourceKeys, fmt.Sprintf("%s.%s", pair.SourcePkg, pair.SourceType))
	}
	sourceKeys = append(sourceKeys, pair.SourceType)

	destKeys := []string{}
	if pair.DestPkg != "" {
		destKeys = append(destKeys, fmt.Sprintf("%s.%s", pair.DestPkg, pair.DestType))
	}
	destKeys = append(destKeys, pair.DestType)

	var sourceInfo, destInfo *parser2.StructInfo
	var sourceExists, destExists bool

	for _, key := range sourceKeys {
		if info, ok := structInfos[key]; ok {
			sourceInfo = info
			sourceExists = true
			if *verbose {
				log.Printf("  找到源结构体：键=%s, 包=%s, 导入路径=%s", key, info.Package, info.ImportPath)
			}
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
		log.Printf("警告：未找到源结构体 %s (尝试的键：%v)", pair.SourceType, sourceKeys)
		return
	}
	if !destExists {
		log.Printf("警告：未找到目标结构体 %s", pair.DestType)
		return
	}

	if *verbose {
		log.Printf("映射：%s.%s (导入路径: %s) -> %s.%s (导入路径: %s)",
			sourceInfo.Package, sourceInfo.Name, sourceInfo.ImportPath,
			destInfo.Package, destInfo.Name, destInfo.ImportPath)
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

	// 解析 map 值类型，支持 map[string]any 和 map[string]interface{}
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

	// 解析 map 值类型，支持 map[string]any 和 map[string]interface{}
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

	// 检查 destPart 是否是有效的结构体类型（不是 map 类型）
	if isMapType(destPart) {
		// 如果目标也是 map 类型，这是无效的配置
		pair.DestType = ""
		return pair
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

	// 类型别名信息：别名 -> 包信息
	type typeAliasPkgInfo struct {
		Underlying string
		PkgName    string
		ImportPath string
		FilePath   string
	}
	typeAliases := make(map[string]typeAliasPkgInfo)

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
								log.Printf("    找到结构体：%s (包：%s, 导入路径：%s)", structName, filePkg, importPath)
							}
						} else {
							// 处理类型别名：type X = Y 或 type X Y
							aliasName := x.Name.Name
							if underlyingType := getTypeAliasUnderlying(x.Type, filePkg); underlyingType != "" {
								fullAlias := fmt.Sprintf("%s.%s", filePkg, aliasName)

								info := typeAliasPkgInfo{
									Underlying: underlyingType,
									PkgName:    filePkg,
									ImportPath: importPath,
									FilePath:   fileName,
								}

								typeAliases[fullAlias] = info
								typeAliases[aliasName] = info

								if *verbose {
									log.Printf("    发现类型别名：%s -> %s (包：%s, 导入路径：%s)",
										fullAlias, underlyingType, filePkg, importPath)
								}
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

	// 处理类型别名：为别名创建指向实际结构体的引用
	for alias, info := range typeAliases {
		if targetInfo, ok := structInfos[info.Underlying]; ok {
			// 为别名创建一个新的 StructInfo 副本，使用别名自己的包信息
			aliasInfo := &parser2.StructInfo{
				Name:          alias,
				Package:       info.PkgName,
				FilePath:      info.FilePath,
				ImportPath:    info.ImportPath,
				Fields:        targetInfo.Fields,
				EmbeddedTypes: targetInfo.EmbeddedTypes,
			}
			structInfos[alias] = aliasInfo

			// 同时注册不带包名的别名（如果别名包含包名）
			if strings.Contains(alias, ".") {
				parts := strings.SplitN(alias, ".", 2)
				shortName := parts[1]
				structInfos[shortName] = aliasInfo
			}

			if *verbose {
				log.Printf("  注册类型别名映射：%s (包：%s, 导入路径：%s) -> %s",
					alias, info.PkgName, info.ImportPath, info.Underlying)
			}
		} else {
			// 尝试解析带包名的别名
			parts := strings.Split(alias, ".")
			if len(parts) == 2 {
				aliasPkg := parts[0]
				aliasName := parts[1]

				// 尝试找到实际类型的包名
				for key, targetInfo := range structInfos {
					if key == info.Underlying || strings.HasSuffix(key, "."+info.Underlying) {
						// 创建别名结构体信息，使用别名自己的包信息
						aliasInfo := &parser2.StructInfo{
							Name:          aliasName,
							Package:       aliasPkg,
							FilePath:      info.FilePath,
							ImportPath:    info.ImportPath,
							Fields:        targetInfo.Fields,
							EmbeddedTypes: targetInfo.EmbeddedTypes,
						}
						structInfos[alias] = aliasInfo
						structInfos[aliasName] = aliasInfo

						if *verbose {
							log.Printf("  注册类型别名映射：%s (包：%s, 导入路径：%s) -> %s",
								alias, aliasPkg, info.ImportPath, key)
						}
						break
					}
				}
			}
		}
	}

	return structInfos
}

// loadDependencyStructs 从依赖包中加载结构体信息
func loadDependencyStructs(dependencies []string, structInfos map[string]*parser2.StructInfo, fset *token.FileSet, verbose bool) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedSyntax | packages.NeedImports,
		Fset: fset,
	}

	// 记录已加载的包，避免重复加载
	loadedPkgs := make(map[string]bool)
	// 记录包名到导入路径的映射（用于解析类型别名中的包名）
	pkgNameToImportPath := make(map[string]string)

	// 收集所有类型别名，等所有包加载后再处理
	allTypeAliases := make(map[string]TypeAliasInfo)

	// 收集需要加载的包列表（包括按需加载的）
	pkgsToLoad := make([]string, 0, len(dependencies))
	for _, dep := range dependencies {
		pkgsToLoad = append(pkgsToLoad, dep)
	}

	// 循环加载包，直到没有新的包需要加载
	for i := 0; i < len(pkgsToLoad); i++ {
		dep := pkgsToLoad[i]

		// 跳过已加载的包
		if loadedPkgs[dep] {
			continue
		}
		loadedPkgs[dep] = true

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
				log.Printf("  解析包：%s (包名: %s)", pkg.PkgPath, pkg.Name)
			}

			// 记录包名到导入路径的映射
			pkgNameToImportPath[pkg.Name] = pkg.PkgPath
			pkgNameToImportPath[pkg.PkgPath] = pkg.PkgPath // 自身映射

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
							if len(pkg.GoFiles) > 0 {
								structInfo.FilePath = pkg.GoFiles[0]
							}
							structInfo.ImportPath = importPath

							for _, key := range keys {
								structInfos[key] = structInfo
							}

							if verbose {
								log.Printf("    找到依赖结构体：%s", structName)
							}
						} else {
							// 收集类型别名信息，稍后统一处理
							aliasName := x.Name.Name
							if underlyingType := getTypeAliasUnderlying(x.Type, filePkg); underlyingType != "" {
								fullAlias := fmt.Sprintf("%s.%s", filePkg, aliasName)
								importPathAlias := fmt.Sprintf("%s.%s", importPath, aliasName)

								goFiles := []string{}
								if len(pkg.GoFiles) > 0 {
									goFiles = pkg.GoFiles
								}

								// 解析底层类型的包名
								underlyingPkg := ""
								if strings.Contains(underlyingType, ".") {
									parts := strings.SplitN(underlyingType, ".", 2)
									underlyingPkg = parts[0]
								}

								allTypeAliases[fullAlias] = TypeAliasInfo{
									AliasName:       aliasName,
									FullAlias:       fullAlias,
									ImportPathAlias: importPathAlias,
									Underlying:      underlyingType,
									FilePkg:         filePkg,
									ImportPath:      importPath,
									GoFiles:         goFiles,
									UnderlyingPkg:   underlyingPkg, // 新增：记录底层类型的包名
								}

								// 如果底层类型是外部包，按需加载
								if underlyingPkg != "" {
									// 尝试从当前包的导入中查找
									for _, imp := range pkg.Imports {
										if imp.Name == underlyingPkg || filepath.Base(imp.PkgPath) == underlyingPkg {
											pkgNameToImportPath[underlyingPkg] = imp.PkgPath
											if !loadedPkgs[imp.PkgPath] {
												pkgsToLoad = append(pkgsToLoad, imp.PkgPath)
												if verbose {
													log.Printf("    按需加载依赖包：%s (用于类型 %s)", imp.PkgPath, underlyingType)
												}
											}
											break
										}
									}
								}

								if verbose {
									log.Printf("    发现依赖包类型别名：%s -> %s", fullAlias, underlyingType)
								}
							}
						}
					}
					return true
				})
			}
		}
	}

	// 所有包加载完成后，统一处理类型别名
	if verbose {
		log.Printf("所有依赖包加载完成，开始处理类型别名...")
	}

	for _, aliasInfo := range allTypeAliases {
		underlyingWithPkg := aliasInfo.Underlying

		// 如果底层类型包含包名前缀（如 dto.DictItemResponse）
		if strings.Contains(aliasInfo.Underlying, ".") {
			parts := strings.SplitN(aliasInfo.Underlying, ".", 2)
			underlyingPkgName := parts[0]
			underlyingTypeName := parts[1]

			// 尝试通过包名找到导入路径
			if importPath, ok := pkgNameToImportPath[underlyingPkgName]; ok {
				underlyingWithPkg = fmt.Sprintf("%s.%s", importPath, underlyingTypeName)
			}
		} else {
			// 如果底层类型没有包名前缀，添加当前包名
			underlyingWithPkg = fmt.Sprintf("%s.%s", aliasInfo.FilePkg, aliasInfo.Underlying)
		}

		// 尝试多种方式查找底层类型
		var targetInfo *parser2.StructInfo
		var found bool

		// 尝试直接查找
		if info, ok := structInfos[underlyingWithPkg]; ok {
			targetInfo = info
			found = true
		} else {
			// 尝试其他可能的键
			for key, info := range structInfos {
				if key == aliasInfo.Underlying ||
					strings.HasSuffix(key, "."+aliasInfo.Underlying) ||
					key == aliasInfo.FilePkg+"."+aliasInfo.Underlying {
					targetInfo = info
					found = true
					break
				}
			}
		}

		// 如果找到了底层类型，创建别名映射
		if found && targetInfo != nil {
			filePath := ""
			if len(aliasInfo.GoFiles) > 0 {
				filePath = aliasInfo.GoFiles[0]
			}
			newAliasInfo := &parser2.StructInfo{
				Name:          aliasInfo.AliasName,
				Package:       aliasInfo.FilePkg,
				FilePath:      filePath,
				ImportPath:    aliasInfo.ImportPath,
				Fields:        targetInfo.Fields,
				EmbeddedTypes: targetInfo.EmbeddedTypes,
			}
			structInfos[aliasInfo.FullAlias] = newAliasInfo
			structInfos[aliasInfo.ImportPathAlias] = newAliasInfo
			structInfos[aliasInfo.AliasName] = newAliasInfo

			if verbose {
				log.Printf("    注册依赖包类型别名：%s (包：%s, 导入路径：%s) -> %s",
					aliasInfo.FullAlias, aliasInfo.FilePkg, aliasInfo.ImportPath, underlyingWithPkg)
			}
		} else {
			if verbose {
				log.Printf("    警告：无法找到类型别名 %s 的底层类型 %s", aliasInfo.FullAlias, underlyingWithPkg)
			}
		}
	}
}

// TypeAliasInfo 类型别名信息
type TypeAliasInfo struct {
	AliasName       string
	FullAlias       string
	ImportPathAlias string
	Underlying      string
	FilePkg         string
	ImportPath      string
	GoFiles         []string
	UnderlyingPkg   string // 底层类型的包名（如果有）
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

// getTypeAliasUnderlying 获取类型别名的底层类型名称
// 支持：
//   - type X = Y (类型别名)
//   - type X pkg.Y (外部包类型)
func getTypeAliasUnderlying(expr ast.Expr, pkgName string) string {
	switch t := expr.(type) {
	case *ast.Ident:
		// 简单类型别名：type X = Y
		return t.Name
	case *ast.SelectorExpr:
		// 外部包类型：type X = pkg.Y
		if ident, ok := t.X.(*ast.Ident); ok {
			return fmt.Sprintf("%s.%s", ident.Name, t.Sel.Name)
		}
	case *ast.StarExpr:
		// 指针类型：type X = *Y
		return getTypeAliasUnderlying(t.X, pkgName)
	}
	return ""
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
