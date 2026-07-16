package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"text/template"
)

// ModuleDef 描述一个被标记的 Section 结构体
type ModuleDef struct {
	StructName string
	Key        string
	Fields     []FieldDef
}

// FieldDef 描述 Section 中一个被标记的字段
type FieldDef struct {
	Name      string
	Type      string
	JSONKey   string
	BitIndex  int
	HasGet    bool
	HasSet    bool
	HasAdd    bool
	IsMap     bool
	MapKey    string
	MapVal    string
	HasMapSet bool
	HasMapDel bool
	HasMapGet bool
}

func main() {
	inputFile := "module_def.go"
	outputFile := "module_gen.go"

	// 支持命令行参数
	args := os.Args[1:]
	for i := 0; i < len(args)-1; i++ {
		switch args[i] {
		case "-input":
			inputFile = args[i+1]
		case "-output":
			outputFile = args[i+1]
		}
	}

	// 解析源文件
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, inputFile, nil, parser.ParseComments)
	if err != nil {
		log.Fatalf("解析文件失败 %s: %v", inputFile, err)
	}

	// 提取 Section 定义
	modules := extractModules(node)
	if len(modules) == 0 {
		log.Fatalf("未找到任何 modgen:data 标记的结构体")
	}

	// 生成代码
	pkgName := node.Name.Name
	code := generateCode(modules, pkgName)

	// 写入输出文件
	outPath := filepath.Join(filepath.Dir(inputFile), outputFile)
	if err := os.WriteFile(outPath, []byte(code), 0644); err != nil {
		log.Fatalf("写入生成文件失败: %v", err)
	}

	fmt.Printf("[modgen] 成功生成 %s (%d 个 Section)\n", outPath, len(modules))
}

// extractModules 从 AST 中提取带有 modgen:data 注释的结构体
func extractModules(node *ast.File) []ModuleDef {
	var modules []ModuleDef

	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		// 检查注释中是否包含 modgen:data 或 modgen:section/module
		var moduleKey string
		if genDecl.Doc != nil {
			for _, comment := range genDecl.Doc.List {
				if strings.Contains(comment.Text, "modgen:data") || strings.Contains(comment.Text, "modgen:section") || strings.Contains(comment.Text, "modgen:module") {
					moduleKey = extractKeyFromComment(comment.Text)
				}
			}
		}
		if moduleKey == "" {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}

			mod := ModuleDef{
				StructName: typeSpec.Name.Name,
				Key:        moduleKey,
			}

			bitIndex := 0
			for _, field := range structType.Fields.List {
				if len(field.Names) == 0 {
					continue
				}
				name := field.Names[0].Name
				// 跳过 dirty 字段
				if name == "dirty" {
					continue
				}

				// 解析 mod tag
				if field.Tag == nil {
					continue
				}
				tagValue := reflect.StructTag(strings.Trim(field.Tag.Value, "`"))
				modTag := tagValue.Get("mod")
				if modTag == "" {
					continue
				}

				jsonTag := tagValue.Get("json")
				if jsonTag == "" || jsonTag == "-" {
					jsonTag = strings.ToLower(name)
				}
				// 去掉 json tag 中的 options (如 omitempty)
				if idx := strings.Index(jsonTag, ","); idx != -1 {
					jsonTag = jsonTag[:idx]
				}

				fieldType := typeToString(field.Type)
				opts := strings.Split(modTag, ",")

				fd := FieldDef{
					Name:     name,
					Type:     fieldType,
					JSONKey:  jsonTag,
					BitIndex: bitIndex,
				}

				// 检查是否为 map 类型
				if strings.HasPrefix(fieldType, "map[") {
					fd.IsMap = true
					fd.MapKey, fd.MapVal = parseMapType(fieldType)
				}

				for _, opt := range opts {
					switch strings.TrimSpace(opt) {
					case "getter":
						fd.HasGet = true
					case "setter":
						fd.HasSet = true
					case "add":
						fd.HasAdd = true
					case "map_set":
						fd.HasMapSet = true
					case "map_del":
						fd.HasMapDel = true
					case "map_get":
						fd.HasMapGet = true
					}
				}

				mod.Fields = append(mod.Fields, fd)
				bitIndex++
			}

			modules = append(modules, mod)
		}
	}
	return modules
}

// extractKeyFromComment 从注释中提取 key=xxx
func extractKeyFromComment(comment string) string {
	idx := strings.Index(comment, "key=")
	if idx == -1 {
		return ""
	}
	rest := comment[idx+4:]
	end := strings.IndexAny(rest, " \t\n\r")
	if end == -1 {
		return rest
	}
	return rest[:end]
}

// typeToString 将 AST 类型节点转为字符串
func typeToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.MapType:
		return "map[" + typeToString(t.Key) + "]" + typeToString(t.Value)
	case *ast.ArrayType:
		return "[]" + typeToString(t.Elt)
	case *ast.StarExpr:
		return "*" + typeToString(t.X)
	case *ast.SelectorExpr:
		return typeToString(t.X) + "." + t.Sel.Name
	default:
		return "interface{}"
	}
}

// parseMapType 解析 map[K]V 返回 K 和 V 的类型字符串
func parseMapType(mapType string) (string, string) {
	// map[int32]int32 → "int32", "int32"
	inner := mapType[4:] // remove "map["
	depth := 0
	for i, ch := range inner {
		if ch == '[' {
			depth++
		} else if ch == ']' {
			if depth == 0 {
				return inner[:i], inner[i+1:]
			}
			depth--
		}
	}
	return "string", "interface{}"
}

// generateCode 根据 Section 定义列表生成完整的 Go 源码
func generateCode(modules []ModuleDef, pkgName string) string {
	tmpl := template.Must(template.New("gen").Funcs(template.FuncMap{
		"title": strings.Title,
	}).Parse(genTemplate))

	var buf bytes.Buffer
	data := struct {
		Package string
		Modules []ModuleDef
	}{Package: pkgName, Modules: modules}

	if err := tmpl.Execute(&buf, data); err != nil {
		log.Fatalf("模板执行失败: %v", err)
	}
	return buf.String()
}

const genTemplate = `// Code generated by modgen. DO NOT EDIT.
package {{.Package}}

import "encoding/json"

// 确保 json 包被使用
var _ = json.Marshal
{{range .Modules}}{{$mod := .}}
// ==================== {{$mod.StructName}} 生成代码 ====================

// 位掩码常量
const (
{{- range $mod.Fields}}
	_{{$mod.StructName}}_{{.Name}}_Bit uint64 = 1 << {{.BitIndex}}
{{- end}}
)

func (m *{{$mod.StructName}}) Key() string    { return "{{$mod.Key}}" }
func (m *{{$mod.StructName}}) IsDirty() bool  { return m.dirty != 0 }
func (m *{{$mod.StructName}}) ClearDirty()    { m.dirty = 0 }

func (m *{{$mod.StructName}}) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

func (m *{{$mod.StructName}}) Unmarshal(data []byte) error {
	return json.Unmarshal(data, m)
}

// DirtyFields 返回脏字段的 JSON key 和当前值（用于字段级 $set）
func (m *{{$mod.StructName}}) DirtyFields() map[string]interface{} {
	fields := make(map[string]interface{})
{{- range $mod.Fields}}
	if m.dirty&_{{$mod.StructName}}_{{.Name}}_Bit != 0 {
		fields["{{.JSONKey}}"] = m.{{.Name}}
	}
{{- end}}
	return fields
}
{{range $mod.Fields}}
{{- if .HasGet}}
func (m *{{$mod.StructName}}) Get{{.Name}}() {{.Type}} { return m.{{.Name}} }
{{- end}}
{{- if .HasSet}}
func (m *{{$mod.StructName}}) Set{{.Name}}(v {{.Type}}) {
	m.{{.Name}} = v
	m.dirty |= _{{$mod.StructName}}_{{.Name}}_Bit
}
{{- end}}
{{- if .HasAdd}}
func (m *{{$mod.StructName}}) Add{{.Name}}(delta {{.Type}}) {
	m.{{.Name}} += delta
	m.dirty |= _{{$mod.StructName}}_{{.Name}}_Bit
}
{{- end}}
{{- if .HasMapGet}}
func (m *{{$mod.StructName}}) Get{{.Name}}Key(key {{.MapKey}}) {{.MapVal}} {
	if m.{{.Name}} == nil { return {{.MapVal}}(0) }
	return m.{{.Name}}[key]
}
{{- end}}
{{- if .HasMapSet}}
func (m *{{$mod.StructName}}) Set{{.Name}}Key(key {{.MapKey}}, val {{.MapVal}}) {
	if m.{{.Name}} == nil { m.{{.Name}} = make({{.Type}}) }
	m.{{.Name}}[key] = val
	m.dirty |= _{{$mod.StructName}}_{{.Name}}_Bit
}
{{- end}}
{{- if .HasMapDel}}
func (m *{{$mod.StructName}}) Del{{.Name}}Key(key {{.MapKey}}) {
	if m.{{.Name}} != nil {
		delete(m.{{.Name}}, key)
		m.dirty |= _{{$mod.StructName}}_{{.Name}}_Bit
	}
}
{{- end}}
{{end}}
{{end}}
`
