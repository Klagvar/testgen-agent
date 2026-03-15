package prompt

import (
	"strings"
	"testing"

	"github.com/gizatulin/testgen-agent/internal/analyzer"
)

func sampleRequest() TestGenRequest {
	return TestGenRequest{
		PackageName: "calc",
		FilePath:    "calc.go",
		Imports:     []string{"errors", "math"},
		TargetFuncs: []analyzer.FuncInfo{
			{
				Name:       "Add",
				Signature:  "func Add(a int, b int) (int, error)",
				Params:     []analyzer.Param{{Name: "a", Type: "int"}, {Name: "b", Type: "int"}},
				Returns:    []string{"int", "error"},
				DocComment: "Add складывает два числа. Поддерживает проверку на переполнение.\n",
				Body: `func Add(a, b int) (int, error) {
	result := a + b
	if (b > 0 && result < a) || (b < 0 && result > a) {
		return 0, errors.New("integer overflow")
	}
	return result, nil
}`,
				StartLine: 9,
				EndLine:   15,
			},
			{
				Name:       "Multiply",
				Signature:  "func Multiply(a int, b int) int",
				Params:     []analyzer.Param{{Name: "a", Type: "int"}, {Name: "b", Type: "int"}},
				Returns:    []string{"int"},
				DocComment: "Multiply умножает два числа.\n",
				Body: `func Multiply(a, b int) int {
	return a * b
}`,
				StartLine: 31,
				EndLine:   33,
			},
			{
				Name:      "Sqrt",
				Signature: "func Sqrt(x float64) (float64, error)",
				Params:    []analyzer.Param{{Name: "x", Type: "float64"}},
				Returns:   []string{"float64", "error"},
				Body: `func Sqrt(x float64) (float64, error) {
	if x < 0 {
		return 0, errors.New("negative number")
	}
	return math.Sqrt(x), nil
}`,
				StartLine: 36,
				EndLine:   41,
			},
		},
	}
}

func TestBuildSystemPrompt(t *testing.T) {
	sys := BuildSystemPrompt()

	if sys == "" {
		t.Fatal("системный промпт пуст")
	}

	// Проверяем ключевые инструкции
	checks := []string{
		"table-driven",
		"testing",
		"Граничные",
		"t.Errorf",
		"package",
	}
	for _, check := range checks {
		if !strings.Contains(sys, check) {
			t.Errorf("системный промпт не содержит %q", check)
		}
	}

	t.Logf("Системный промпт (%d символов):\n%s", len(sys), sys)
}

func TestBuildUserPrompt_ContainsFunctions(t *testing.T) {
	req := sampleRequest()
	prompt := BuildUserPrompt(req)

	// Должен содержать все функции
	for _, fn := range req.TargetFuncs {
		if !strings.Contains(prompt, fn.Name) {
			t.Errorf("промпт не содержит функцию %q", fn.Name)
		}
		if !strings.Contains(prompt, fn.Signature) {
			t.Errorf("промпт не содержит сигнатуру %q", fn.Signature)
		}
	}

	// Должен содержать имя пакета
	if !strings.Contains(prompt, "calc") {
		t.Error("промпт не содержит имя пакета")
	}

	// Должен содержать импорты
	if !strings.Contains(prompt, "errors") || !strings.Contains(prompt, "math") {
		t.Error("промпт не содержит импорты")
	}

	t.Logf("User промпт (%d символов)", len(prompt))
}

func TestBuildUserPrompt_ContainsBody(t *testing.T) {
	req := sampleRequest()
	prompt := BuildUserPrompt(req)

	// Должен содержать тела функций
	if !strings.Contains(prompt, "result := a + b") {
		t.Error("промпт не содержит тело Add")
	}
	if !strings.Contains(prompt, "return a * b") {
		t.Error("промпт не содержит тело Multiply")
	}
	if !strings.Contains(prompt, "math.Sqrt(x)") {
		t.Error("промпт не содержит тело Sqrt")
	}
}

func TestBuildUserPrompt_ContainsBranches(t *testing.T) {
	req := sampleRequest()
	prompt := BuildUserPrompt(req)

	// Add имеет ветвление (if), должно быть обнаружено
	if !strings.Contains(prompt, "Условие") {
		t.Error("промпт не содержит анализ ветвлений для Add")
	}

	// Sqrt тоже имеет if x < 0
	if !strings.Contains(prompt, "x < 0") {
		t.Error("промпт не содержит условие x < 0 для Sqrt")
	}
}

func TestBuildUserPrompt_ContainsDocComment(t *testing.T) {
	req := sampleRequest()
	prompt := BuildUserPrompt(req)

	if !strings.Contains(prompt, "переполнение") {
		t.Error("промпт не содержит документацию Add")
	}
	if !strings.Contains(prompt, "умножает") {
		t.Error("промпт не содержит документацию Multiply")
	}
}

func TestBuildUserPrompt_WithExistingTests(t *testing.T) {
	req := sampleRequest()
	req.ExistingTests = `func TestAdd(t *testing.T) {
	// existing test
}`
	prompt := BuildUserPrompt(req)

	if !strings.Contains(prompt, "Существующие тесты") {
		t.Error("промпт не содержит раздел существующих тестов")
	}
	if !strings.Contains(prompt, "не дублируй") {
		t.Error("промпт не содержит инструкцию не дублировать")
	}
	if !strings.Contains(prompt, "TestAdd") {
		t.Error("промпт не содержит текст существующего теста")
	}
}

func TestBuildUserPrompt_WithoutExistingTests(t *testing.T) {
	req := sampleRequest()
	req.ExistingTests = ""
	prompt := BuildUserPrompt(req)

	if strings.Contains(prompt, "Существующие тесты") {
		t.Error("промпт содержит раздел существующих тестов, хотя их нет")
	}
}

func TestBuildMessages(t *testing.T) {
	req := sampleRequest()
	msgs := BuildMessages(req)

	if len(msgs) != 2 {
		t.Fatalf("ожидалось 2 сообщения, получено %d", len(msgs))
	}

	if msgs[0].Role != "system" {
		t.Errorf("первое сообщение должно быть system, получено %q", msgs[0].Role)
	}
	if msgs[1].Role != "user" {
		t.Errorf("второе сообщение должно быть user, получено %q", msgs[1].Role)
	}

	// Проверяем что контент не пустой
	if msgs[0].Content == "" || msgs[1].Content == "" {
		t.Error("содержимое сообщений не должно быть пустым")
	}
}

func TestAnalyzeBranches(t *testing.T) {
	body := `func Example() {
	if x > 0 {
		doSomething()
	} else if x == 0 {
		doNothing()
	} else {
		doOther()
	}
	switch mode {
	case "fast":
		fast()
	case "slow":
		slow()
	}
	if err != nil {
		return err
	}
}`

	branches := analyzeBranches(body)
	t.Logf("Найденные ветвления: %v", branches)

	if len(branches) < 5 {
		t.Errorf("ожидалось >= 5 ветвлений, получено %d", len(branches))
	}

	// Проверяем типы
	hasIf := false
	hasElseIf := false
	hasElse := false
	hasSwitch := false
	hasCase := false
	hasErrCheck := false

	for _, b := range branches {
		if strings.HasPrefix(b, "Условие:") {
			hasIf = true
		}
		if strings.HasPrefix(b, "Иначе-если:") {
			hasElseIf = true
		}
		if b == "Ветка else" {
			hasElse = true
		}
		if b == "Switch-выражение" {
			hasSwitch = true
		}
		if strings.HasPrefix(b, "Case:") {
			hasCase = true
		}
		if strings.Contains(b, "err != nil") {
			hasErrCheck = true
		}
	}

	if !hasIf {
		t.Error("не найдено условие if")
	}
	if !hasElseIf {
		t.Error("не найдено else if")
	}
	if !hasElse {
		t.Error("не найдена ветка else")
	}
	if !hasSwitch {
		t.Error("не найден switch")
	}
	if !hasCase {
		t.Error("не найден case")
	}
	if !hasErrCheck {
		t.Error("не найдена проверка ошибки")
	}
}
