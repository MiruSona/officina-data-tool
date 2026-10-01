package gen

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mirusona/officina-data-tool/internal/schema"
)

// Generate 는 스키마 하나에서 생성 C# 파일 묶음을 만든다.
//
// 키는 생성 폴더(`Assets/_Project/Scripts/Data/Generated/`) 기준 상대 경로이고
// 값은 파일 내용이다. 디스크에 쓰는 것은 Write 가 한다 — 만드는 것과 쓰는 것을 가른다.
func Generate(f *schema.File) (map[string]string, error) {
	if f == nil {
		return nil, fmt.Errorf("스키마가 nil 이다")
	}
	if strings.TrimSpace(f.Namespace) == "" {
		return nil, fmt.Errorf("스키마에 namespace 가 없다 — 생성 코드가 들어갈 자리를 모른다")
	}
	if err := checkNameClashes(f); err != nil {
		return nil, err
	}

	hash := f.Hash()
	files := make(map[string]string)

	for _, name := range sortedEnumNames(f) {
		files[pascal(name)+".cs"] = enumFile(f, name, hash)
	}
	for _, t := range f.Tables {
		files[className(t.Name)+".cs"] = rowFile(f, t, hash)
	}
	files["GameDataTables.cs"] = tablesFile(f, hash)

	// 손으로 쓴 Unity 셋도 같이 낸다 — 네임스페이스를 갈아 끼워서 (설계 8장).
	runtime, err := runtimeFiles(f.Namespace, hash)
	if err != nil {
		return nil, err
	}
	for name, content := range runtime {
		files[name] = content
	}
	return files, nil
}

// sortedEnumNames 는 enum 이름을 가나다 차례로 준다.
// 맵 순회 차례가 그대로 파일 내용이 되면 돌릴 때마다 결과가 달라진다.
func sortedEnumNames(f *schema.File) []string {
	names := make([]string, 0, len(f.Enums))
	for name := range f.Enums {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// checkNameClashes 는 서로 다른 것이 같은 C# 이름·같은 파일 이름으로 접히는지 본다.
//
// `grade`(enum) 와 `grade_name`(string) 이 한 표에 같이 있으면 둘 다 `GradeName` 이 된다.
// 표 이름도 마찬가지다 — `drop_rate` 와 `droprate` 는 파일이 둘 다 `DropRateRow.cs`·
// `DroprateRow.cs` 로 나오는데 **Windows 파일 시스템은 대소문자를 안 가려서 하나가 다른 하나를 덮는다.**
// 컴파일이 깨지거나 파일이 조용히 사라지기 전에 여기서 잡는다.
func checkNameClashes(f *schema.File) error {
	if err := checkEnumValueClashes(f); err != nil {
		return err
	}
	if err := checkColumnClashes(f); err != nil {
		return err
	}
	return checkGlobalClashes(f)
}

// checkEnumValueClashes 는 한 enum 안에서 값 둘이 같은 C# 이름이 되는지 본다.
//
// `foo_bar` 와 `foo__bar` 는 둘 다 `FooBar` 가 된다 — enum 멤버도 const 도 두 번 나와 컴파일이 깨진다.
// 이름표 클래스의 붙박이 메서드(Parse·ToName·ParseAll)와 겹치는 값도 같은 자리에서 막는다.
func checkEnumValueClashes(f *schema.File) error {
	for _, name := range sortedEnumNames(f) {
		p := newPot("enum " + name + " 의 C# 이름")
		for _, fixed := range []string{"Parse", "ToName", "ParseAll"} {
			if err := p.put(fixed, "이름표 클래스의 붙박이 메서드 "+fixed); err != nil {
				return err
			}
		}
		for _, v := range f.Enums[name] {
			if err := p.put(pascal(v), "값 "+v); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkColumnClashes 는 한 표 안에서 열 둘이 같은 속성 이름이 되는지 본다.
//
// 행 클래스 **안에서 부르는 이름표 클래스**도 같이 담는다. `list<enum>` 열 이름이 enum 이름과 같으면
// 속성이 `GradeNames` 로 나서 같은 이름의 도우미 클래스를 가리고, `GradeNames.ParseAll(GradeNames)`
// 가 컴파일이 안 된다.
func checkColumnClashes(f *schema.File) error {
	for _, t := range f.Tables {
		taken := make(map[string]string)
		for _, c := range t.Columns {
			if c.Base == schema.TypeEnum {
				taken[enumNamesClass(c.Enum)] = "enum " + c.Enum + " 의 이름표 클래스"
			}
		}
		for _, c := range t.Columns {
			used := []string{serializedProperty(c.Name, c.Base == schema.TypeEnum, c.IsList)}
			if c.Base == schema.TypeEnum {
				used = append(used, pascal(c.Name))
			}
			for _, name := range used {
				if before, ok := taken[name]; ok {
					return fmt.Errorf("표 %s: %s 와 열 %s 가 둘 다 C# 이름 %s 가 된다", t.Name, before, c.Name, name)
				}
				taken[name] = "열 " + c.Name
			}
		}
	}
	return nil
}

// checkGlobalClashes 는 스키마 하나에서 나오는 **이름 전부를 한 통에 모아** 겹치는지 본다.
//
// 통은 둘이다.
//
//	이름 통 — enum 타입·이름표 클래스·행 클래스·GameDataTables 의 속성. C# 컴파일러가 보는 자리다.
//	파일 통 — 만들 파일 이름을 **소문자로 접어서** 담는다. Windows 파일 시스템이 보는 자리다.
func checkGlobalClashes(f *schema.File) error {
	names := newPot("C# 이름")
	// 생성물이 아닌 붙박이 이름도 같이 담는다 — 같은 폴더·같은 네임스페이스에 사는 식구다.
	for _, fixed := range []string{"GameDataTables", "GameDataLoader", "GameDataException"} {
		if err := names.put(fixed, "붙박이 C# 이름 "+fixed); err != nil {
			return err
		}
	}
	files := newPot("생성 파일 이름")
	for _, fixed := range []string{"GameDataTables.cs", "GameDataLoader.cs", "GameDataException.cs"} {
		if err := files.put(strings.ToLower(fixed), "붙박이 파일 "+fixed); err != nil {
			return err
		}
	}

	for _, name := range sortedEnumNames(f) {
		from := "enum " + name
		if err := names.put(pascal(name), from); err != nil {
			return err
		}
		if err := names.put(enumNamesClass(name), from); err != nil {
			return err
		}
		if err := files.put(strings.ToLower(pascal(name)+".cs"), from); err != nil {
			return err
		}
	}
	for _, t := range f.Tables {
		from := "표 " + t.Name
		if err := names.put(className(t.Name), from); err != nil {
			return err
		}
		if err := names.put(pascal(t.Name), from); err != nil {
			return err
		}
		if err := names.put(pascal(t.Name)+"Rows", from); err != nil {
			return err
		}
		if err := files.put(strings.ToLower(className(t.Name)+".cs"), from); err != nil {
			return err
		}
	}
	return nil
}

// pot 은 「이 이름은 이미 누가 쓴다」를 기억하는 통이다.
type pot struct {
	what  string
	taken map[string]string
}

func newPot(what string) *pot {
	return &pot{what: what, taken: map[string]string{}}
}

func (p *pot) put(key, from string) error {
	if before, ok := p.taken[key]; ok {
		return fmt.Errorf("%s이 겹친다: %s 와 %s 가 둘 다 %q 가 된다", p.what, before, from, key)
	}
	p.taken[key] = from
	return nil
}

// header 는 생성 파일 머리의 경고다. 사람이 고치면 다음 gen 에서 날아간다는 것을 먼저 알린다.
func header(hash string) string {
	var b strings.Builder
	b.WriteString("// <auto-generated>\n")
	b.WriteString("// datatool gen 이 만든 파일이다. 손대지 마라 — 다음 gen 에서 통째로 덮인다.\n")
	b.WriteString("// 고칠 것이 있으면 GameData/schema.json 을 고치고 `datatool gen` 을 다시 돌린다.\n")
	b.WriteString("// schemaHash : " + hash + "\n")
	b.WriteString("// </auto-generated>\n\n")
	return b.String()
}

// csType 는 열 하나가 C# 에서 어떤 타입인지 알려준다.
//
// enum 은 구운 파일에 이름 문자열로 들어가므로 여기서도 string 이다.
// 리플렉션을 쓰는 enum 포매터를 피하려는 것 — IL2CPP 에서 그게 제일 잘 터진다.
// asset 도 address 문자열이다. 로드는 게임 코드가 한다.
func csType(c *schema.Column) (string, error) {
	var base string
	switch c.Base {
	case schema.TypeInt:
		base = "int"
	case schema.TypeFloat:
		base = "float"
	case schema.TypeBool:
		base = "bool"
	case schema.TypeString, schema.TypeRef, schema.TypeEnum, schema.TypeAsset:
		base = "string"
	default:
		return "", fmt.Errorf("모르는 타입 %q", c.Type)
	}
	if c.IsList {
		return base + "[]", nil
	}
	return base, nil
}

// enumFile 은 `<Enum>.cs` 한 장이다 — enum 하나와 이름표 클래스 하나.
func enumFile(f *schema.File, name string, hash string) string {
	values := f.Enums[name]
	typeName := pascal(name)
	namesClass := enumNamesClass(name)

	var b strings.Builder
	b.WriteString(header(hash))
	b.WriteString("namespace " + f.Namespace + "\n{\n")
	b.WriteString("    // schema.json 의 enums." + name + " 다.\n")
	b.WriteString("    public enum " + typeName + "\n    {\n")
	for i, v := range values {
		fmt.Fprintf(&b, "        %s = %d,\n", pascal(v), i)
	}
	b.WriteString("    }\n\n")

	b.WriteString("    // 구운 파일에는 enum 이 이름 문자열로 들어간다. 그 문자열과 enum 을 잇는 자리다.\n")
	b.WriteString("    public static class " + namesClass + "\n    {\n")
	for _, v := range values {
		fmt.Fprintf(&b, "        public const string %s = %s;\n", pascal(v), quote(v))
	}
	b.WriteString("\n        public static " + typeName + " Parse(string name)\n        {\n")
	b.WriteString("            switch (name)\n            {\n")
	for _, v := range values {
		fmt.Fprintf(&b, "                case %s:\n                    return %s.%s;\n", quote(v), typeName, pascal(v))
	}
	b.WriteString("            }\n\n")
	b.WriteString("            throw new System.FormatException(\"" + name + " 에 없는 값이다 : \" + name);\n")
	b.WriteString("        }\n\n")

	b.WriteString("        public static string ToName(" + typeName + " value)\n        {\n")
	b.WriteString("            switch (value)\n            {\n")
	for _, v := range values {
		fmt.Fprintf(&b, "                case %s.%s:\n                    return %s;\n", typeName, pascal(v), quote(v))
	}
	b.WriteString("            }\n\n")
	b.WriteString("            throw new System.ArgumentOutOfRangeException(\"value\");\n")
	b.WriteString("        }\n\n")

	b.WriteString("        public static " + typeName + "[] ParseAll(string[] names)\n        {\n")
	b.WriteString("            if (names == null)\n            {\n                return new " + typeName + "[0];\n            }\n\n")
	b.WriteString("            " + typeName + "[] values = new " + typeName + "[names.Length];\n")
	b.WriteString("            for (int i = 0; i < names.Length; i++)\n            {\n")
	b.WriteString("                values[i] = Parse(names[i]);\n            }\n\n")
	b.WriteString("            return values;\n")
	b.WriteString("        }\n")
	b.WriteString("    }\n}\n")
	return b.String()
}

// rowFile 은 `<Table>Row.cs` 한 장이다.
//
// [Key(n)] 의 n 은 스키마의 열 차례이고, 굽는 쪽이 행을 같은 차례의 배열로 쓴다.
// MessagePack-CSharp 는 키가 전부 정수면 객체를 배열로 읽는다 — 그 계약에 맞춘 것이다.
func rowFile(f *schema.File, t *schema.Table, hash string) string {
	class := className(t.Name)

	var b strings.Builder
	b.WriteString(header(hash))
	b.WriteString("using MessagePack;\n\n")
	b.WriteString("namespace " + f.Namespace + "\n{\n")
	b.WriteString("    // 표 " + t.Name + " 의 한 행이다. [Key(n)] 의 n 은 스키마의 열 차례다.\n")
	b.WriteString("    [MessagePackObject]\n")
	b.WriteString("    public sealed class " + class + "\n    {\n")

	for i, c := range t.Columns {
		typ, err := csType(c)
		if err != nil {
			// 스키마 검사를 이미 지난 값이라 여기 오면 안 된다. 와도 파일이 깨지지 않게 적어 둔다.
			typ = "object"
		}
		if c.Desc != "" {
			b.WriteString("        // " + oneLine(c.Desc) + "\n")
		}
		if c.Base == schema.TypeRef {
			b.WriteString("        // " + c.Ref + " 의 id 다. GameDataTables.Get<" + className(c.Ref) + ">(…) 로 잇는다.\n")
		}
		fmt.Fprintf(&b, "        [Key(%d)]\n", i)
		fmt.Fprintf(&b, "        public %s %s { get; set; }\n", typ, serializedProperty(c.Name, c.Base == schema.TypeEnum, c.IsList))
		if i < len(t.Columns)-1 {
			b.WriteString("\n")
		}
	}

	for _, c := range t.Columns {
		if c.Base != schema.TypeEnum {
			continue
		}
		raw := serializedProperty(c.Name, true, c.IsList)
		prop := pascal(c.Name)
		names := enumNamesClass(c.Enum)
		b.WriteString("\n        // 구운 값은 문자열이다. 쓸 때는 이 속성으로 enum 을 받는다.\n")
		b.WriteString("        [IgnoreMember]\n")
		if c.IsList {
			fmt.Fprintf(&b, "        public %s[] %s\n        {\n            get { return %s.ParseAll(%s); }\n        }\n", pascal(c.Enum), prop, names, raw)
		} else {
			fmt.Fprintf(&b, "        public %s %s\n        {\n            get { return %s.Parse(%s); }\n        }\n", pascal(c.Enum), prop, names, raw)
		}
	}

	b.WriteString("    }\n}\n")
	return b.String()
}

// tablesFile 은 통 `GameDataTables.cs` 다.
//
// 최상위 맵은 표마다 값 타입이 달라서 클래스 하나로 못 받는다. 그래서 MessagePackReader 로
// 맵을 손으로 훑고 표 이름을 만날 때마다 그 표의 배열만 Deserialize 한다 (설계 13장의 미확인 위험).
//
// 파일이 길어 **다섯 토막으로 나눠 쓴다** — 머리 · 속성 · Deserialize · Get<T> · 도우미.
// 나눈 것은 읽기 편하라고이지 내용이 아니다. 나온 바이트는 나누기 전과 같다 (골든 시험이 지킨다).
func tablesFile(f *schema.File, hash string) string {
	var b strings.Builder
	tablesHead(&b, f, hash)
	tablesProperties(&b, f)
	tablesDeserialize(&b, f)
	tablesGet(&b, f)
	tablesHelpers(&b, f)
	return b.String()
}

// tablesHead 는 파일 머리와 클래스 선언, 붙박이 속성 둘이다.
func tablesHead(b *strings.Builder, f *schema.File, hash string) {
	b.WriteString(header(hash))
	b.WriteString("using System;\n")
	b.WriteString("using System.Collections.Generic;\n")
	b.WriteString("using MessagePack;\n\n")
	b.WriteString("namespace " + f.Namespace + "\n{\n")
	b.WriteString("    // 구운 gamedata.bytes 의 본문을 읽어 담은 표 전부다.\n")
	b.WriteString("    public sealed class GameDataTables\n    {\n")
	b.WriteString("        // 이 코드를 만든 스키마의 해시다. 구운 파일의 _meta.schemaHash 와 다르면 바로 예외다.\n")
	b.WriteString("        public const string SchemaHash = " + quote(hash) + ";\n\n")
	b.WriteString("        private GameDataTables()\n        {\n        }\n\n")
	b.WriteString("        // 구운 시각(RFC3339). 화면에 찍어 보는 용도다.\n")
	b.WriteString("        public string BuiltAt { get; private set; }\n")
}

// tablesProperties 는 표마다 나는 속성 둘이다 — 적힌 차례 그대로인 목록과 id 로 찾는 사전.
func tablesProperties(b *strings.Builder, f *schema.File) {
	for _, t := range f.Tables {
		class := className(t.Name)
		prop := pascal(t.Name)
		b.WriteString("\n        // 표 " + t.Name + " — 적힌 차례 그대로.\n")
		fmt.Fprintf(b, "        public IReadOnlyList<%s> %sRows { get; private set; }\n\n", class, prop)
		b.WriteString("        // 표 " + t.Name + " — id 로 찾는다.\n")
		fmt.Fprintf(b, "        public IReadOnlyDictionary<string, %s> %s { get; private set; }\n", class, prop)
	}

}

// tablesDeserialize 는 본문 맵을 훑어 표를 채우는 static 메서드다.
func tablesDeserialize(b *strings.Builder, f *schema.File) {
	b.WriteString("\n        // 머리 16바이트를 뗀 본문을 읽는다. 머리 검사는 GameDataLoader 가 한다.\n")
	b.WriteString("        public static GameDataTables Deserialize(ReadOnlyMemory<byte> body)\n        {\n")
	b.WriteString("            return Deserialize(body, MessagePackSerializer.DefaultOptions);\n")
	b.WriteString("        }\n\n")
	b.WriteString("        public static GameDataTables Deserialize(ReadOnlyMemory<byte> body, MessagePackSerializerOptions options)\n        {\n")
	b.WriteString("            MessagePackReader reader = new MessagePackReader(body);\n")
	b.WriteString("            string schemaHash = null;\n")
	b.WriteString("            string builtAt = null;\n")
	for _, t := range f.Tables {
		fmt.Fprintf(b, "            %s[] %s = null;\n", className(t.Name), localName(t.Name))
	}
	b.WriteString("\n            int count = reader.ReadMapHeader();\n")
	b.WriteString("            for (int i = 0; i < count; i++)\n            {\n")
	b.WriteString("                string key = reader.ReadString();\n")
	b.WriteString("                switch (key)\n                {\n")
	b.WriteString("                    case \"_meta\":\n")
	b.WriteString("                        ReadMeta(ref reader, out schemaHash, out builtAt);\n")
	b.WriteString("                        break;\n")
	for _, t := range f.Tables {
		fmt.Fprintf(b, "                    case %s:\n", quote(t.Name))
		fmt.Fprintf(b, "                        %s = MessagePackSerializer.Deserialize<%s[]>(ref reader, options);\n", localName(t.Name), className(t.Name))
		b.WriteString("                        break;\n")
	}
	b.WriteString("                    default:\n")
	b.WriteString("                        // 모르는 표는 건너뛴다. 새 표가 늘어난 파일을 옛 코드가 읽어도 안 터지게.\n")
	b.WriteString("                        reader.Skip();\n")
	b.WriteString("                        break;\n")
	b.WriteString("                }\n            }\n\n")
	b.WriteString("            if (schemaHash != SchemaHash)\n            {\n")
	b.WriteString("                throw new GameDataException(\n")
	b.WriteString("                    \"스키마가 달라졌는데 안 구웠다. 구운 파일 : \" + (schemaHash ?? \"없음\") +\n")
	b.WriteString("                    \", 코드 : \" + SchemaHash + \" — datatool export 를 다시 돌려라\");\n")
	b.WriteString("            }\n\n")
	b.WriteString("            GameDataTables tables = new GameDataTables();\n")
	b.WriteString("            tables.BuiltAt = builtAt;\n")
	for _, t := range f.Tables {
		prop := pascal(t.Name)
		local := localName(t.Name)
		fmt.Fprintf(b, "            tables.%sRows = Require(%s, %s);\n", prop, local, quote(t.Name))
		fmt.Fprintf(b, "            tables.%s = Index(%s, %s);\n", prop, local, quote(t.Name))
	}
	b.WriteString("            return tables;\n")
	b.WriteString("        }\n")

}

// tablesGet 은 ref 열의 id 로 다른 표의 행을 집는 Get<T> 다.
func tablesGet(b *strings.Builder, f *schema.File) {
	b.WriteString("\n        // ref 열의 값(id)으로 다른 표의 행을 집는다. 없으면 null 이다.\n")
	b.WriteString("        public T Get<T>(string id) where T : class\n        {\n")
	b.WriteString("            if (id == null)\n            {\n                return null;\n            }\n")
	for _, t := range f.Tables {
		class := className(t.Name)
		prop := pascal(t.Name)
		b.WriteString("\n            if (typeof(T) == typeof(" + class + "))\n            {\n")
		fmt.Fprintf(b, "                %s row;\n", class)
		fmt.Fprintf(b, "                if (%s.TryGetValue(id, out row) == false)\n                {\n                    return null;\n                }\n\n", prop)
		b.WriteString("                return (T)(object)row;\n")
		b.WriteString("            }\n")
	}
	b.WriteString("\n            throw new GameDataException(typeof(T).Name + \" 은 이 스키마의 표가 아니다\");\n")
	b.WriteString("        }\n")

}

// tablesHelpers 는 ReadMeta · Require · Index · IdOf 넷과 닫는 괄호다.
func tablesHelpers(b *strings.Builder, f *schema.File) {
	b.WriteString(`
        private static void ReadMeta(ref MessagePackReader reader, out string schemaHash, out string builtAt)
        {
            schemaHash = null;
            builtAt = null;

            int count = reader.ReadMapHeader();
            for (int i = 0; i < count; i++)
            {
                string key = reader.ReadString();
                if (key == "schemaHash")
                {
                    schemaHash = reader.ReadString();
                }
                else if (key == "builtAt")
                {
                    builtAt = reader.ReadString();
                }
                else
                {
                    reader.Skip();
                }
            }
        }

        private static T[] Require<T>(T[] rows, string table)
        {
            if (rows == null)
            {
                throw new GameDataException("구운 파일에 표 " + table + " 이 없다 — 스키마와 구운 파일이 어긋났다");
            }

            return rows;
        }

        private static Dictionary<string, T> Index<T>(T[] rows, string table) where T : class
        {
            Dictionary<string, T> byId = new Dictionary<string, T>(rows.Length);
            for (int i = 0; i < rows.Length; i++)
            {
                string id = IdOf(rows[i], table, i);
                if (byId.ContainsKey(id))
                {
                    throw new GameDataException(table + " 에 같은 id 가 둘이다 : " + id);
                }

                byId[id] = rows[i];
            }

            return byId;
        }

        // 첫 열은 언제나 string id 다 (설계 4-3). 그 값을 꺼낸다.
        private static string IdOf<T>(T row, string table, int index) where T : class
        {
`)
	for _, t := range f.Tables {
		class := className(t.Name)
		fmt.Fprintf(b, "            if (row is %s)\n            {\n                return ((%s)(object)row).Id;\n            }\n\n", class, class)
	}
	b.WriteString("            throw new GameDataException(table + \" 의 \" + index + \"번째 행 타입을 모른다\");\n")
	b.WriteString("        }\n")
	b.WriteString("    }\n}\n")
}

// localName 은 생성 코드 안에서 쓰는 지역 변수 이름이다. C# 키워드와 겹치지 않게 뒤에 Rows 를 붙인다.
func localName(table string) string {
	name := pascal(table)
	return strings.ToLower(name[:1]) + name[1:] + "Rows"
}

// quote 는 C# 문자열 리터럴로 감싼다. 스키마 이름은 영소문자·숫자·밑줄뿐이라 이스케이프는 둘이면 된다.
func quote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// oneLine 은 주석에 넣을 설명을 한 줄로 만든다. 줄바꿈이 들어가면 주석이 끊긴다.
func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}
