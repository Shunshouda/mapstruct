package main

// -config 模式：从 Go 源文件的包级变量 MapstructConfig 读取批量生成配置。
// 一个进程加载全部 Include 目录的并集后按组输出，替代每条 //go:generate
// 独立起进程重复扫描。变量是带类型的切片字面量，字段名错误/类型不匹配
// 直接编译失败；工具只接受字符串字面量，不接受常量引用或表达式。

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
)

// mapstructConfigVar 是约定的配置变量名，generate.go 中形如：
//
//	//go:generate mapstruct -config=generate.go
//	var MapstructConfig = []struct {
//		Types   []string // 映射对："pkg.Source:pkg.Dest" 或 "...:map[string]any"
//		Include []string // 扫描目录，相对本文件
//		Output  string   // 输出文件名
//	}{...}
const mapstructConfigVar = "MapstructConfig"

type configGroup struct {
	Types   []string
	Include []string
	Output  string
	Package string
}

// loadConfigFile 解析 configPath 所指 Go 文件中的 MapstructConfig 变量。
func loadConfigFile(configPath string) ([]configGroup, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, configPath, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("解析配置文件 %s 失败: %w", configPath, err)
	}

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.VAR {
			continue
		}
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range valueSpec.Names {
				if name.Name != mapstructConfigVar || i >= len(valueSpec.Values) {
					continue
				}
				groups, err := evalConfigSlice(fset, valueSpec.Values[i])
				if err != nil {
					return nil, fmt.Errorf("%s: %w", configPath, err)
				}
				return groups, nil
			}
		}
	}
	return nil, fmt.Errorf("在 %s 中未找到包级变量 %s", configPath, mapstructConfigVar)
}

func evalConfigSlice(fset *token.FileSet, expr ast.Expr) ([]configGroup, error) {
	sliceLit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, posErrorf(fset, expr, "%s 必须是切片字面量", mapstructConfigVar)
	}

	var groups []configGroup
	for _, elt := range sliceLit.Elts {
		groupLit, ok := elt.(*ast.CompositeLit)
		if !ok {
			return nil, posErrorf(fset, elt, "%s 的元素必须是结构体字面量", mapstructConfigVar)
		}
		group, err := evalConfigGroup(fset, groupLit)
		if err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, nil
}

func evalConfigGroup(fset *token.FileSet, lit *ast.CompositeLit) (configGroup, error) {
	var group configGroup
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			return group, posErrorf(fset, elt, "字段必须以 key: value 形式书写")
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			return group, posErrorf(fset, kv.Key, "字段名必须是标识符")
		}
		var err error
		switch key.Name {
		case "Types":
			group.Types, err = evalStringSlice(fset, kv.Value)
		case "Include":
			group.Include, err = evalStringSlice(fset, kv.Value)
		case "Output":
			group.Output, err = evalString(fset, kv.Value)
		case "Package":
			group.Package, err = evalString(fset, kv.Value)
		default:
			return group, posErrorf(fset, key, "未知字段 %s（支持 Types/Include/Output/Package）", key.Name)
		}
		if err != nil {
			return group, err
		}
	}
	for _, check := range []struct {
		missing bool
		name    string
	}{
		{len(group.Types) == 0, "Types"},
		{len(group.Include) == 0, "Include"},
		{group.Output == "", "Output"},
	} {
		if check.missing {
			return group, posErrorf(fset, lit, "缺少 %s 字段", check.name)
		}
	}
	return group, nil
}

func evalStringSlice(fset *token.FileSet, expr ast.Expr) ([]string, error) {
	sliceLit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, posErrorf(fset, expr, "必须是 []string 字面量")
	}
	var out []string
	for _, elt := range sliceLit.Elts {
		s, err := evalString(fset, elt)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func evalString(fset *token.FileSet, expr ast.Expr) (string, error) {
	basicLit, ok := expr.(*ast.BasicLit)
	if !ok || basicLit.Kind != token.STRING {
		return "", posErrorf(fset, expr, "只支持字符串字面量（配置必须自描述，不接受常量或表达式）")
	}
	s, err := strconv.Unquote(basicLit.Value)
	if err != nil {
		return "", posErrorf(fset, basicLit, "非法字符串字面量: %v", err)
	}
	return s, nil
}

func posErrorf(fset *token.FileSet, node ast.Node, format string, args ...interface{}) error {
	return fmt.Errorf("%s: %s", fset.Position(node.Pos()), fmt.Sprintf(format, args...))
}
