// Package patterns provides AST-based detection of common Go idioms
// (HTTP handlers, context.Context, time.Now, env vars, file I/O, SQL/DB)
// and returns structured hints that enrich the LLM prompt.
package patterns

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/gizatulin/testgen-agent/internal/analyzer"
)

// PatternKind identifies the type of detected pattern.
type PatternKind string

const (
	PatternHTTPHandler  PatternKind = "http_handler"
	PatternContext      PatternKind = "context"
	PatternTimeNow     PatternKind = "time_now"
	PatternEnvVar      PatternKind = "env_var"
	PatternFileIO      PatternKind = "file_io"
	PatternDatabase    PatternKind = "database"
	PatternErrorsWrap  PatternKind = "errors_wrap"
	PatternHTTPClient  PatternKind = "http_client"
	PatternJSON        PatternKind = "json_encoding"
	PatternIOReadWrite PatternKind = "io_readwrite"
)

// PatternHint is a single piece of guidance injected into the LLM prompt.
type PatternHint struct {
	Kind    PatternKind
	Summary string // one-line description shown as a heading
	Guide   string // multi-line testing guidance for the LLM
}

// Detect runs all pattern detectors on a function and returns any hints found.
func Detect(fn analyzer.FuncInfo, fileImports []string, allTypes []analyzer.TypeInfo) []PatternHint {
	var hints []PatternHint

	if h := detectHTTPHandler(fn, fileImports); h != nil {
		hints = append(hints, *h)
	}
	if h := detectContext(fn); h != nil {
		hints = append(hints, *h)
	}
	if h := detectTimeNow(fn); h != nil {
		hints = append(hints, *h)
	}
	if h := detectEnvVar(fn); h != nil {
		hints = append(hints, *h)
	}
	if h := detectFileIO(fn); h != nil {
		hints = append(hints, *h)
	}
	if h := detectDatabase(fn, allTypes); h != nil {
		hints = append(hints, *h)
	}
	if h := detectErrorsWrap(fn); h != nil {
		hints = append(hints, *h)
	}
	if h := detectHTTPClient(fn, fileImports); h != nil {
		hints = append(hints, *h)
	}
	if h := detectJSON(fn, fileImports); h != nil {
		hints = append(hints, *h)
	}
	if h := detectIOReadWrite(fn, fileImports); h != nil {
		hints = append(hints, *h)
	}

	return hints
}

// DetectAll runs detection across multiple functions and deduplicates by kind.
func DetectAll(funcs []analyzer.FuncInfo, fileImports []string, allTypes []analyzer.TypeInfo) map[string][]PatternHint {
	result := make(map[string][]PatternHint)
	for _, fn := range funcs {
		hints := Detect(fn, fileImports, allTypes)
		if len(hints) > 0 {
			result[fn.Name] = hints
		}
	}
	return result
}

func detectHTTPHandler(fn analyzer.FuncInfo, imports []string) *PatternHint {
	hasNetHTTP := containsImport(imports, "net/http")
	hasGin := containsImport(imports, "github.com/gin-gonic/gin")
	hasEcho := containsImport(imports, "github.com/labstack/echo")
	hasFiber := containsImport(imports, "github.com/gofiber/fiber")

	for _, p := range fn.Params {
		typ := p.Type
		if hasNetHTTP && (typ == "http.ResponseWriter" || typ == "*http.Request") {
			return httpHandlerHint("net/http")
		}
		if hasGin && typ == "*gin.Context" {
			return httpHandlerHint("gin")
		}
		if (hasEcho && typ == "echo.Context") || strings.Contains(typ, "echo.Context") {
			return httpHandlerHint("echo")
		}
		if hasFiber && (typ == "*fiber.Ctx" || strings.Contains(typ, "fiber.Ctx")) {
			return httpHandlerHint("fiber")
		}
	}

	if hasNetHTTP {
		for _, r := range fn.Returns {
			if r == "http.Handler" || r == "http.HandlerFunc" {
				return httpHandlerHint("net/http")
			}
		}
	}

	return nil
}

func httpHandlerHint(framework string) *PatternHint {
	switch framework {
	case "gin":
		return &PatternHint{
			Kind:    PatternHTTPHandler,
			Summary: "HTTP handler (gin framework)",
			Guide: `This function is a Gin HTTP handler. Use gin's test utilities:
- Create a test router: r := gin.New(); r.GET("/path", handler)
- Use httptest: req := httptest.NewRequest("GET", "/path", nil)
- rec := httptest.NewRecorder(); r.ServeHTTP(rec, req)
- Check rec.Code, rec.Body
Do NOT make real HTTP requests. Do NOT start a real server.`,
		}
	case "echo":
		return &PatternHint{
			Kind:    PatternHTTPHandler,
			Summary: "HTTP handler (echo framework)",
			Guide: `This function is an Echo HTTP handler. Use echo's test helpers:
- e := echo.New()
- req := httptest.NewRequest(method, path, body)
- rec := httptest.NewRecorder()
- c := e.NewContext(req, rec)
- Call the handler with c, then check rec.Code and rec.Body
Do NOT make real HTTP requests.`,
		}
	case "fiber":
		return &PatternHint{
			Kind:    PatternHTTPHandler,
			Summary: "HTTP handler (fiber framework)",
			Guide: `This function is a Fiber HTTP handler. Use fiber's testing:
- app := fiber.New()
- app.Get("/path", handler)
- req := httptest.NewRequest("GET", "/path", nil)
- resp, _ := app.Test(req)
- Check resp.StatusCode and body
Do NOT make real HTTP requests.`,
		}
	default:
		return &PatternHint{
			Kind:    PatternHTTPHandler,
			Summary: "HTTP handler (net/http)",
			Guide: `This function is an HTTP handler. Use the httptest pattern:
- req := httptest.NewRequest("METHOD", "/path", body)
- Set headers if needed: req.Header.Set("Content-Type", "application/json")
- rec := httptest.NewRecorder()
- Call the handler: handler.ServeHTTP(rec, req) or handlerFunc(rec, req)
- Check rec.Code for status and rec.Body.String() for response body
Do NOT make real HTTP requests. Do NOT start a real server.`,
		}
	}
}

func detectContext(fn analyzer.FuncInfo) *PatternHint {
	for _, p := range fn.Params {
		if p.Type == "context.Context" {
			usesCancel := bodyContains(fn.Body, "ctx.Done()") ||
				bodyContains(fn.Body, "ctx.Err()") ||
				bodyContains(fn.Body, "ctx.Deadline()") ||
				bodyContains(fn.Body, "<-ctx.Done()")
			usesValue := bodyContains(fn.Body, "ctx.Value(")

			guide := `This function accepts context.Context. Generate tests for:
1. Normal context: context.Background()
2. Already-cancelled context:
   ctx, cancel := context.WithCancel(context.Background())
   cancel()
   // pass ctx to the function`
			if usesCancel {
				guide += `
3. Expired timeout:
   ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
   defer cancel()
   time.Sleep(5 * time.Millisecond)
   // pass ctx — it should be expired`
			}
			if usesValue {
				guide += `
- The function reads values from context via ctx.Value(). Test with and without expected keys.`
			}
			return &PatternHint{
				Kind:    PatternContext,
				Summary: "Uses context.Context",
				Guide:   guide,
			}
		}
	}
	return nil
}

func detectTimeNow(fn analyzer.FuncInfo) *PatternHint {
	if !bodyContainsCall(fn.Body, "time", "Now") {
		return nil
	}
	return &PatternHint{
		Kind:    PatternTimeNow,
		Summary: "Uses time.Now()",
		Guide: `This function calls time.Now(). When writing tests:
- Use RELATIVE time values: time.Now().Add(-1 * time.Hour) for the past, time.Now().Add(1 * time.Hour) for the future
- Do NOT hardcode absolute dates like time.Date(2025, 1, 1, ...) — they will become stale
- For expiry/deadline tests: set times relative to now so the test is stable over time`,
	}
}

func detectEnvVar(fn analyzer.FuncInfo) *PatternHint {
	hasGetenv := bodyContainsCall(fn.Body, "os", "Getenv")
	hasLookup := bodyContainsCall(fn.Body, "os", "LookupEnv")
	if !hasGetenv && !hasLookup {
		return nil
	}
	return &PatternHint{
		Kind:    PatternEnvVar,
		Summary: "Reads environment variables",
		Guide: `This function reads environment variables (os.Getenv / os.LookupEnv). In tests:
- Use t.Setenv("KEY", "value") to set env vars — it is automatically cleaned up after the test
- Test BOTH cases: env var present and env var absent/empty
- Do NOT use os.Setenv directly — it leaks between parallel tests`,
	}
}

func detectFileIO(fn analyzer.FuncInfo) *PatternHint {
	fileOps := []struct{ pkg, fn string }{
		{"os", "ReadFile"}, {"os", "WriteFile"}, {"os", "Open"}, {"os", "Create"},
		{"os", "Remove"}, {"os", "MkdirAll"}, {"os", "Stat"}, {"os", "Mkdir"},
		{"os", "OpenFile"}, {"os", "ReadDir"},
	}
	found := false
	for _, op := range fileOps {
		if bodyContainsCall(fn.Body, op.pkg, op.fn) {
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	return &PatternHint{
		Kind:    PatternFileIO,
		Summary: "Performs file I/O",
		Guide: `This function performs file system operations. In tests:
- Use t.TempDir() for temporary directories — automatically cleaned up after the test
- Create test files: os.WriteFile(filepath.Join(tmpDir, "test.txt"), data, 0644)
- Do NOT use hardcoded paths like "/tmp/test" — breaks on Windows
- Do NOT use paths relative to the working directory — flaky in CI
- Test error cases: non-existent files, permission errors (if applicable)`,
	}
}

func detectDatabase(fn analyzer.FuncInfo, allTypes []analyzer.TypeInfo) *PatternHint {
	dbTypes := []string{
		"*sql.DB", "*sql.Tx", "sql.DB", "sql.Tx",
		"*gorm.DB", "gorm.DB",
		"*sqlx.DB", "*sqlx.Tx", "sqlx.DB", "sqlx.Tx",
		"*pgx.Conn", "*pgxpool.Pool", "pgx.Conn", "pgxpool.Pool",
	}

	isDBType := func(t string) bool {
		for _, dbt := range dbTypes {
			if t == dbt {
				return true
			}
		}
		return false
	}

	for _, p := range fn.Params {
		if isDBType(p.Type) {
			return databaseHint()
		}
	}

	if fn.Receiver != "" {
		recvName := strings.TrimPrefix(fn.Receiver, "*")
		for _, ti := range allTypes {
			if ti.Name == recvName && ti.Kind == "struct" {
				for _, field := range ti.Fields {
					if isDBType(field.Type) {
						return databaseHint()
					}
				}
			}
		}
	}

	return nil
}

func databaseHint() *PatternHint {
	return &PatternHint{
		Kind:    PatternDatabase,
		Summary: "Uses database connection",
		Guide: `This function uses a database connection (*sql.DB / *gorm.DB / *sqlx.DB / pgx).
Direct database calls cannot be unit-tested without a mock or test double.

Preferred approach:
1. If the function accepts an interface parameter — create a mock implementation in the test
2. If the receiver struct stores the DB in a field behind an interface — mock that interface
3. If the DB is a concrete type (*sql.DB) — note this in a comment and test what you can
   (input validation, error path logic, etc.) without calling the database

Do NOT attempt to connect to a real database in unit tests.`,
	}
}

func detectErrorsWrap(fn analyzer.FuncInfo) *PatternHint {
	hasErrorsIs := bodyContainsCall(fn.Body, "errors", "Is")
	hasErrorsAs := bodyContainsCall(fn.Body, "errors", "As")
	hasErrorsNew := bodyContainsCall(fn.Body, "errors", "New")
	hasErrorf := bodyContainsCall(fn.Body, "fmt", "Errorf") && bodyContains(fn.Body, "%w")

	if !hasErrorsIs && !hasErrorsAs && !hasErrorsNew && !hasErrorf {
		return nil
	}
	return &PatternHint{
		Kind:    PatternErrorsWrap,
		Summary: "Uses error wrapping/inspection",
		Guide: `This function uses error wrapping or inspection (errors.Is/As/New, fmt.Errorf with %w).
In tests:
- Use errors.Is() and errors.As() for error comparison — do NOT use == for wrapped errors
- When testing fmt.Errorf with %w: the wrapped error is whatever the function ACTUALLY wraps.
  READ THE CODE to see which error is wrapped. Do NOT assume it wraps a package-level sentinel
  unless the code explicitly uses that sentinel (e.g. fmt.Errorf("...: %w", ErrNotFound)).
  If the function wraps an error from another call (e.g. json.Unmarshal), the chain contains
  THAT error, not a package-level sentinel.
- Only use errors.Is(err, SentinelVar) when the code path actually wraps that specific sentinel.
- Test both the error message and the underlying error type`,
	}
}

func detectHTTPClient(fn analyzer.FuncInfo, imports []string) *PatternHint {
	if !containsImport(imports, "net/http") {
		return nil
	}
	calls := []struct{ pkg, fn string }{
		{"http", "Get"}, {"http", "Post"}, {"http", "Do"}, {"http", "NewRequest"},
		{"client", "Do"}, {"client", "Get"},
	}
	found := false
	for _, c := range calls {
		if bodyContainsCall(fn.Body, c.pkg, c.fn) {
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	return &PatternHint{
		Kind:    PatternHTTPClient,
		Summary: "Makes outgoing HTTP requests",
		Guide: `This function makes outgoing HTTP requests (http.Get/Post/Do/NewRequest).
In tests:
- Use httptest.NewServer to create a fake server that returns controlled responses
- Do NOT make real HTTP requests to external services
- Test different response status codes (200, 400, 500)
- Test network errors by shutting down the test server before the call
- Replace the base URL with the test server URL`,
	}
}

func detectJSON(fn analyzer.FuncInfo, imports []string) *PatternHint {
	if !containsImport(imports, "encoding/json") {
		return nil
	}
	calls := []struct{ pkg, fn string }{
		{"json", "Marshal"}, {"json", "Unmarshal"},
		{"json", "NewEncoder"}, {"json", "NewDecoder"},
	}
	found := false
	for _, c := range calls {
		if bodyContainsCall(fn.Body, c.pkg, c.fn) {
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	return &PatternHint{
		Kind:    PatternJSON,
		Summary: "Uses JSON encoding/decoding",
		Guide: `This function uses JSON encoding or decoding (json.Marshal/Unmarshal/NewEncoder/NewDecoder).
In tests:
- Test with valid and invalid JSON input
- CAREFULLY read struct tags: json:"name,omitempty" means the field is OMITTED when zero-value.
  For example, if Email has tag json:"email,omitempty" and Email == "", the JSON output will NOT contain "email" at all.
  Do NOT expect {"email":""} — it will be absent from output.
- Test boundary values: nil, empty struct, nested objects
- For Unmarshal: test malformed JSON, missing fields, extra fields
- For Marshal: verify output matches expected JSON by tracing omitempty behavior for each field`,
	}
}

func detectIOReadWrite(fn analyzer.FuncInfo, imports []string) *PatternHint {
	if !containsImport(imports, "io") {
		return nil
	}
	hasIOCall := bodyContainsCall(fn.Body, "io", "ReadAll") ||
		bodyContainsCall(fn.Body, "io", "Copy")
	hasIOParam := false
	for _, p := range fn.Params {
		if p.Type == "io.Reader" || p.Type == "io.Writer" {
			hasIOParam = true
			break
		}
	}
	if !hasIOCall && !hasIOParam {
		return nil
	}
	return &PatternHint{
		Kind:    PatternIOReadWrite,
		Summary: "Uses io.Reader/Writer or io utilities",
		Guide: `This function uses io.Reader, io.Writer, or io utilities (io.ReadAll, io.Copy).
In tests:
- Use strings.NewReader("test data") to create an io.Reader
- Use &bytes.Buffer{} to create an io.Writer
- Test with empty input (strings.NewReader(""))
- Test with large input to verify no truncation or buffer issues
- For io.Copy: test that all bytes are transferred correctly`,
	}
}

// bodyContains does a simple substring search in the function body.
func bodyContains(body, substr string) bool {
	return strings.Contains(body, substr)
}

// bodyContainsCall checks if the function body contains a pkg.Func() style call
// using AST parsing for accuracy.
func bodyContainsCall(body, pkg, funcName string) bool {
	if body == "" {
		return false
	}
	if !strings.Contains(body, funcName) {
		return false
	}

	wrapped := "package _detect\nfunc _detect_() {\n" + body + "\n}"
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", wrapped, 0)
	if err != nil {
		return strings.Contains(body, pkg+"."+funcName)
	}

	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if ident.Name == pkg && sel.Sel.Name == funcName {
			found = true
		}
		return true
	})
	return found
}

func containsImport(imports []string, path string) bool {
	for _, imp := range imports {
		if imp == path || strings.HasSuffix(imp, "/"+path) {
			return true
		}
	}
	return false
}
