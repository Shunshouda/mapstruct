package main

import (
	"flag"
	"fmt"
	"github.com/shunshouda/mapstruct/generator"
	parser2 "github.com/shunshouda/mapstruct/parser"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var (
	typeNames   = flag.String("type", "", "逗号分隔的结构体类型名称对，格式为:package.Source:package.Dest")
	output      = flag.String("output", "", "输出文件名")
	packageName = flag.String("package", "", "生成的包名")
	includeDirs = flag.String("include", "", "逗号分隔的要包含的目录")
	verbose     = flag.Bool("verbose", false, "显示详细日志")
	modulePath  = flag.String("module", "", "Go Module 路径，如: github.com/user/project")
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

	// 自动检测 module path
	if *modulePath == "" {
		*modulePath = detectModulePath()
	}
	if *verbose && *modulePath != "" {
		log.Printf("检测到模块路径: %s", *modulePath)
	}

	// 收集所有结构体信息
	structInfos := collectStructInfos(scanDirs, *modulePath)

	// 生成代码
	gen := generator.NewGenerator(*packageName)
	for _, pair := range typePairs {
		// 尝试多种键格式来查找结构体
		sourceKeys := []string{
			fmt.Sprintf("%s.%s", pair.SourcePkg, pair.SourceType),
			pair.SourceType, // 仅类型名
		}

		destKeys := []string{
			fmt.Sprintf("%s.%s", pair.DestPkg, pair.DestType),
			pair.DestType, // 仅类型名
		}

		var sourceInfo, destInfo *parser2.StructInfo
		var sourceExists, destExists bool

		// 查找源结构体
		for _, key := range sourceKeys {
			if info, exists := structInfos[key]; exists {
				sourceInfo = info
				sourceExists = true
				if *verbose {
					log.Printf("找到源结构体 %s (键: %s)", pair.SourceType, key)
				}
				break
			}
		}

		// 查找目标结构体
		for _, key := range destKeys {
			if info, exists := structInfos[key]; exists {
				destInfo = info
				destExists = true
				if *verbose {
					log.Printf("找到目标结构体 %s (键: %s)", pair.DestType, key)
				}
				break
			}
		}

		if !sourceExists {
			log.Printf("警告: 未找到源结构体 %s (尝试的键: %v)", pair.SourceType, sourceKeys)
			// 打印可用的结构体用于调试
			if *verbose {
				log.Printf("可用的结构体:")
				for key := range structInfos {
					log.Printf("  - %s", key)
				}
			}
			continue
		}
		if !destExists {
			log.Printf("警告: 未找到目标结构体 %s (尝试的键: %v)", pair.DestType, destKeys)
			continue
		}

		if *verbose {
			log.Printf("映射: %s.%s -> %s.%s",
				sourceInfo.Package, sourceInfo.Name,
				destInfo.Package, destInfo.Name)
		}

		gen.AddMapping(sourceInfo, destInfo)
	}

	// 写入文件
	code, err := gen.Generate()
	if err != nil {
		log.Fatal("生成代码失败:", err)
	}

	if err := os.WriteFile(outputFile, []byte(code), 0644); err != nil {
		log.Fatal("写入文件失败:", err)
	}

	fmt.Printf("成功生成映射代码到: %s\n", outputFile)
}

// 解析类型对 (支持 package.Type 格式)
func parseTypePairs(input string) []generator.TypePair {
	var pairs []generator.TypePair
	typeStrs := strings.Split(input, ",")

	for _, typeStr := range typeStrs {
		parts := strings.Split(typeStr, ":")
		if len(parts) == 2 {
			sourceParts := strings.Split(strings.TrimSpace(parts[0]), ".")
			destParts := strings.Split(strings.TrimSpace(parts[1]), ".")

			if len(sourceParts) == 2 && len(destParts) == 2 {
				// 完整格式: package.Source:package.Dest
				pairs = append(pairs, generator.TypePair{
					SourcePkg:  sourceParts[0],
					SourceType: sourceParts[1],
					DestPkg:    destParts[0],
					DestType:   destParts[1],
				})
			} else if len(sourceParts) == 2 && len(destParts) == 1 {
				// 混合格式: package.Source:Dest (目标类型在当前包)
				pairs = append(pairs, generator.TypePair{
					SourcePkg:  sourceParts[0],
					SourceType: sourceParts[1],
					DestPkg:    "", // 空表示当前包
					DestType:   destParts[0],
				})
			} else if len(sourceParts) == 1 && len(destParts) == 2 {
				// 混合格式: Source:package.Dest (源类型在当前包)
				pairs = append(pairs, generator.TypePair{
					SourcePkg:  "", // 空表示当前包
					SourceType: sourceParts[0],
					DestPkg:    destParts[0],
					DestType:   destParts[1],
				})
			} else if len(sourceParts) == 1 && len(destParts) == 1 {
				// 简单格式: Source:Dest (都在当前包)
				pairs = append(pairs, generator.TypePair{
					SourceType: sourceParts[0],
					DestType:   destParts[0],
				})
			}
		}
	}
	return pairs
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

// 检测 Go Module 路径
func detectModulePath() string {
	// 查找当前目录或父目录中的 go.mod 文件
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
			break // 到达根目录
		}
		dir = parent
	}

	return ""
}

// 收集所有目录的结构体信息
func collectStructInfos(scanDirs []string, modulePath string) map[string]*parser2.StructInfo {
	structInfos := make(map[string]*parser2.StructInfo)
	fset := token.NewFileSet()

	for _, dir := range scanDirs {
		if *verbose {
			log.Printf("扫描目录: %s", dir)
		}

		// 解析目录下的所有Go文件
		pkgs, err := parser.ParseDir(fset, dir, nil, parser.ParseComments)
		if err != nil {
			log.Printf("警告: 解析目录 %s 失败: %v", dir, err)
			continue
		}

		for pkgName, pkg := range pkgs {
			for fileName, file := range pkg.Files {
				if *verbose {
					log.Printf("  解析文件: %s", fileName)
				}

				// 获取文件的包名（可能和目录名不同）
				filePkg := pkgName
				if file.Name != nil {
					filePkg = file.Name.Name
				}

				// 计算完整的导入路径
				importPath := calculateImportPath(fileName, modulePath, dir)

				ast.Inspect(file, func(n ast.Node) bool {
					switch x := n.(type) {
					case *ast.TypeSpec:
						if structType, ok := x.Type.(*ast.StructType); ok {
							structName := x.Name.Name

							// 创建多个键来支持不同的查找方式
							keys := []string{
								fmt.Sprintf("%s.%s", filePkg, structName), // 包名.类型名
								structName, // 仅类型名（用于同包内的类型）
							}

							structInfo := parser2.ParseStruct(structName, structType, file)
							structInfo.Package = filePkg
							structInfo.FilePath = fileName
							structInfo.ImportPath = importPath

							// 为每个键存储结构体信息
							for _, key := range keys {
								if existing, exists := structInfos[key]; exists {
									if *verbose {
										log.Printf("    警告: 键 %s 已存在 (%s)，覆盖为 %s",
											key, existing.Package, filePkg)
									}
								}
								structInfos[key] = structInfo
							}

							if *verbose {
								log.Printf("    找到结构体: %s (包: %s, 导入路径: %s)", structName, filePkg, importPath)
							}
						}
					}
					return true
				})
			}
		}
	}

	return structInfos
}

// 计算完整的导入路径
func calculateImportPath(filePath, modulePath, baseDir string) string {
	if modulePath == "" {
		// 如果没有模块路径，返回相对路径
		relPath, err := filepath.Rel(baseDir, filepath.Dir(filePath))
		if err != nil {
			return filepath.Dir(filePath)
		}
		if relPath == "." {
			return ""
		}
		return relPath
	}

	// 计算相对于模块根目录的路径
	absFilePath, err := filepath.Abs(filepath.Dir(filePath))
	if err != nil {
		return modulePath
	}

	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return modulePath
	}

	// 找到模块根目录（包含 go.mod 的目录）
	moduleRoot := findModuleRoot(absBaseDir)
	if moduleRoot == "" {
		return modulePath
	}

	// 计算相对于模块根目录的路径
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
			break // 到达根目录
		}
		dir = parent
	}
	return ""
}
