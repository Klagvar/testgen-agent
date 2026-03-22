package patterns

import (
	"testing"

	"github.com/gizatulin/testgen-agent/internal/analyzer"
)

func TestDetectHTTPHandler_NetHTTP(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name: "CreateUser",
		Params: []analyzer.Param{
			{Name: "w", Type: "http.ResponseWriter"},
			{Name: "r", Type: "*http.Request"},
		},
		Body: `func CreateUser(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }`,
	}
	hints := Detect(fn, []string{"net/http"}, nil)
	if len(hints) == 0 {
		t.Fatal("expected HTTP handler hint")
	}
	if hints[0].Kind != PatternHTTPHandler {
		t.Errorf("expected kind %s, got %s", PatternHTTPHandler, hints[0].Kind)
	}
}

func TestDetectHTTPHandler_Gin(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name:   "GetUser",
		Params: []analyzer.Param{{Name: "c", Type: "*gin.Context"}},
		Body:   `func GetUser(c *gin.Context) { c.JSON(200, nil) }`,
	}
	hints := Detect(fn, []string{"github.com/gin-gonic/gin"}, nil)
	if len(hints) == 0 {
		t.Fatal("expected gin handler hint")
	}
	if hints[0].Kind != PatternHTTPHandler {
		t.Errorf("expected HTTP handler, got %s", hints[0].Kind)
	}
	if hints[0].Summary != "HTTP handler (gin framework)" {
		t.Errorf("unexpected summary: %s", hints[0].Summary)
	}
}

func TestDetectHTTPHandler_Echo(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name:   "GetUser",
		Params: []analyzer.Param{{Name: "c", Type: "echo.Context"}},
		Body:   `func GetUser(c echo.Context) error { return c.JSON(200, nil) }`,
	}
	hints := Detect(fn, []string{"github.com/labstack/echo"}, nil)
	if len(hints) == 0 {
		t.Fatal("expected echo handler hint")
	}
	if hints[0].Kind != PatternHTTPHandler {
		t.Errorf("expected HTTP handler, got %s", hints[0].Kind)
	}
}

func TestDetectHTTPHandler_NoImport(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name:   "Process",
		Params: []analyzer.Param{{Name: "w", Type: "http.ResponseWriter"}},
		Body:   `func Process(w http.ResponseWriter) {}`,
	}
	hints := Detect(fn, []string{"fmt"}, nil)
	for _, h := range hints {
		if h.Kind == PatternHTTPHandler {
			t.Error("should not detect HTTP handler without net/http import")
		}
	}
}

func TestDetectContext(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name:   "Process",
		Params: []analyzer.Param{{Name: "ctx", Type: "context.Context"}, {Name: "id", Type: "int"}},
		Body:   `func Process(ctx context.Context, id int) error { return nil }`,
	}
	hints := Detect(fn, []string{"context"}, nil)
	found := false
	for _, h := range hints {
		if h.Kind == PatternContext {
			found = true
		}
	}
	if !found {
		t.Error("expected context pattern hint")
	}
}

func TestDetectContext_WithCancel(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name:   "Wait",
		Params: []analyzer.Param{{Name: "ctx", Type: "context.Context"}},
		Body: `func Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	}
}`,
	}
	hints := Detect(fn, nil, nil)
	found := false
	for _, h := range hints {
		if h.Kind == PatternContext {
			found = true
			if len(h.Guide) < 100 {
				t.Error("expected detailed guide for context with cancel")
			}
		}
	}
	if !found {
		t.Error("expected context hint")
	}
}

func TestDetectContext_NoContext(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name:   "Add",
		Params: []analyzer.Param{{Name: "a", Type: "int"}, {Name: "b", Type: "int"}},
		Body:   `func Add(a, b int) int { return a + b }`,
	}
	hints := Detect(fn, nil, nil)
	for _, h := range hints {
		if h.Kind == PatternContext {
			t.Error("should not detect context for plain function")
		}
	}
}

func TestDetectTimeNow(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name: "IsExpired",
		Body: `func (t *Token) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}`,
	}
	hints := Detect(fn, []string{"time"}, nil)
	found := false
	for _, h := range hints {
		if h.Kind == PatternTimeNow {
			found = true
		}
	}
	if !found {
		t.Error("expected time.Now hint")
	}
}

func TestDetectTimeNow_NoTimeCall(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name: "Format",
		Body: `func Format(t time.Time) string { return t.Format("2006-01-02") }`,
	}
	hints := Detect(fn, nil, nil)
	for _, h := range hints {
		if h.Kind == PatternTimeNow {
			t.Error("should not detect time.Now when it is not called")
		}
	}
}

func TestDetectEnvVar(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name: "GetDBURL",
		Body: `func GetDBURL() string { return os.Getenv("DATABASE_URL") }`,
	}
	hints := Detect(fn, []string{"os"}, nil)
	found := false
	for _, h := range hints {
		if h.Kind == PatternEnvVar {
			found = true
		}
	}
	if !found {
		t.Error("expected env var hint")
	}
}

func TestDetectEnvVar_LookupEnv(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name: "GetPort",
		Body: `func GetPort() string {
	if v, ok := os.LookupEnv("PORT"); ok { return v }
	return "8080"
}`,
	}
	hints := Detect(fn, nil, nil)
	found := false
	for _, h := range hints {
		if h.Kind == PatternEnvVar {
			found = true
		}
	}
	if !found {
		t.Error("expected env var hint for LookupEnv")
	}
}

func TestDetectFileIO(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name: "ReadConfig",
		Body: `func ReadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil { return nil, err }
	return parse(data)
}`,
	}
	hints := Detect(fn, nil, nil)
	found := false
	for _, h := range hints {
		if h.Kind == PatternFileIO {
			found = true
		}
	}
	if !found {
		t.Error("expected file I/O hint")
	}
}

func TestDetectFileIO_WriteFile(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name: "Save",
		Body: `func Save(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}`,
	}
	hints := Detect(fn, nil, nil)
	found := false
	for _, h := range hints {
		if h.Kind == PatternFileIO {
			found = true
		}
	}
	if !found {
		t.Error("expected file I/O hint for WriteFile")
	}
}

func TestDetectDatabase_Param(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name:   "GetUser",
		Params: []analyzer.Param{{Name: "db", Type: "*sql.DB"}, {Name: "id", Type: "int"}},
		Body:   `func GetUser(db *sql.DB, id int) (*User, error) { return nil, nil }`,
	}
	hints := Detect(fn, nil, nil)
	found := false
	for _, h := range hints {
		if h.Kind == PatternDatabase {
			found = true
		}
	}
	if !found {
		t.Error("expected database hint for *sql.DB param")
	}
}

func TestDetectDatabase_ReceiverField(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name:     "FindAll",
		Receiver: "*UserRepo",
		Params:   []analyzer.Param{{Name: "ctx", Type: "context.Context"}},
		Body:     `func (r *UserRepo) FindAll(ctx context.Context) ([]User, error) { return nil, nil }`,
	}
	types := []analyzer.TypeInfo{
		{
			Name: "UserRepo",
			Kind: "struct",
			Fields: []analyzer.FieldInfo{
				{Name: "db", Type: "*gorm.DB"},
			},
		},
	}
	hints := Detect(fn, nil, types)
	found := false
	for _, h := range hints {
		if h.Kind == PatternDatabase {
			found = true
		}
	}
	if !found {
		t.Error("expected database hint for receiver with *gorm.DB field")
	}
}

func TestDetectDatabase_NoDBType(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name:   "Add",
		Params: []analyzer.Param{{Name: "a", Type: "int"}},
		Body:   `func Add(a int) int { return a + 1 }`,
	}
	hints := Detect(fn, nil, nil)
	for _, h := range hints {
		if h.Kind == PatternDatabase {
			t.Error("should not detect database for plain function")
		}
	}
}

func TestDetectAll(t *testing.T) {
	funcs := []analyzer.FuncInfo{
		{
			Name:   "Handler",
			Params: []analyzer.Param{{Name: "w", Type: "http.ResponseWriter"}, {Name: "r", Type: "*http.Request"}},
			Body:   `func Handler(w http.ResponseWriter, r *http.Request) {}`,
		},
		{
			Name: "Pure",
			Body: `func Pure() int { return 42 }`,
		},
	}
	result := DetectAll(funcs, []string{"net/http"}, nil)
	if _, ok := result["Handler"]; !ok {
		t.Error("expected hints for Handler")
	}
	if _, ok := result["Pure"]; ok {
		t.Error("should not have hints for Pure")
	}
}

func TestDetectErrorsWrap_ErrorsIs(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name: "CheckErr",
		Body: `func CheckErr(err error) bool {
	return errors.Is(err, ErrNotFound)
}`,
	}
	hints := Detect(fn, []string{"errors"}, nil)
	found := false
	for _, h := range hints {
		if h.Kind == PatternErrorsWrap {
			found = true
		}
	}
	if !found {
		t.Error("expected errors_wrap hint for errors.Is")
	}
}

func TestDetectErrorsWrap_FmtErrorf(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name: "WrapErr",
		Body: `func WrapErr(err error) error {
	return fmt.Errorf("operation failed: %w", err)
}`,
	}
	hints := Detect(fn, nil, nil)
	found := false
	for _, h := range hints {
		if h.Kind == PatternErrorsWrap {
			found = true
		}
	}
	if !found {
		t.Error("expected errors_wrap hint for fmt.Errorf with %w")
	}
}

func TestDetectErrorsWrap_NoErrors(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name: "Add",
		Body: `func Add(a, b int) int { return a + b }`,
	}
	hints := Detect(fn, nil, nil)
	for _, h := range hints {
		if h.Kind == PatternErrorsWrap {
			t.Error("should not detect errors_wrap for plain function")
		}
	}
}

func TestDetectHTTPClient(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name: "FetchData",
		Body: `func FetchData(url string) (*http.Response, error) {
	return http.Get(url)
}`,
	}
	hints := Detect(fn, []string{"net/http"}, nil)
	found := false
	for _, h := range hints {
		if h.Kind == PatternHTTPClient {
			found = true
		}
	}
	if !found {
		t.Error("expected http_client hint for http.Get")
	}
}

func TestDetectHTTPClient_Do(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name: "Send",
		Body: `func Send(client *http.Client, req *http.Request) (*http.Response, error) {
	return client.Do(req)
}`,
	}
	hints := Detect(fn, []string{"net/http"}, nil)
	found := false
	for _, h := range hints {
		if h.Kind == PatternHTTPClient {
			found = true
		}
	}
	if !found {
		t.Error("expected http_client hint for client.Do")
	}
}

func TestDetectHTTPClient_NoImport(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name: "FetchData",
		Body: `func FetchData(url string) { http.Get(url) }`,
	}
	hints := Detect(fn, []string{"fmt"}, nil)
	for _, h := range hints {
		if h.Kind == PatternHTTPClient {
			t.Error("should not detect http_client without net/http import")
		}
	}
}

func TestDetectJSON_Marshal(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name: "ToJSON",
		Body: `func ToJSON(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}`,
	}
	hints := Detect(fn, []string{"encoding/json"}, nil)
	found := false
	for _, h := range hints {
		if h.Kind == PatternJSON {
			found = true
		}
	}
	if !found {
		t.Error("expected json_encoding hint for json.Marshal")
	}
}

func TestDetectJSON_Unmarshal(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name: "FromJSON",
		Body: `func FromJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}`,
	}
	hints := Detect(fn, []string{"encoding/json"}, nil)
	found := false
	for _, h := range hints {
		if h.Kind == PatternJSON {
			found = true
		}
	}
	if !found {
		t.Error("expected json_encoding hint for json.Unmarshal")
	}
}

func TestDetectJSON_NoImport(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name: "ToJSON",
		Body: `func ToJSON(v interface{}) ([]byte, error) { return json.Marshal(v) }`,
	}
	hints := Detect(fn, []string{"fmt"}, nil)
	for _, h := range hints {
		if h.Kind == PatternJSON {
			t.Error("should not detect json_encoding without encoding/json import")
		}
	}
}

func TestDetectIOReadWrite_ReadAll(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name: "ReadBody",
		Body: `func ReadBody(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}`,
	}
	hints := Detect(fn, []string{"io"}, nil)
	found := false
	for _, h := range hints {
		if h.Kind == PatternIOReadWrite {
			found = true
		}
	}
	if !found {
		t.Error("expected io_readwrite hint for io.ReadAll")
	}
}

func TestDetectIOReadWrite_Param(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name:   "Process",
		Params: []analyzer.Param{{Name: "r", Type: "io.Reader"}},
		Body:   `func Process(r io.Reader) error { return nil }`,
	}
	hints := Detect(fn, []string{"io"}, nil)
	found := false
	for _, h := range hints {
		if h.Kind == PatternIOReadWrite {
			found = true
		}
	}
	if !found {
		t.Error("expected io_readwrite hint for io.Reader param")
	}
}

func TestDetectIOReadWrite_NoImport(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name: "ReadBody",
		Body: `func ReadBody(r io.Reader) ([]byte, error) { return io.ReadAll(r) }`,
	}
	hints := Detect(fn, []string{"fmt"}, nil)
	for _, h := range hints {
		if h.Kind == PatternIOReadWrite {
			t.Error("should not detect io_readwrite without io import")
		}
	}
}

func TestMultiplePatterns(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name: "Handle",
		Params: []analyzer.Param{
			{Name: "ctx", Type: "context.Context"},
		},
		Body: `func Handle(ctx context.Context) error {
	url := os.Getenv("API_URL")
	_ = url
	return nil
}`,
	}
	hints := Detect(fn, nil, nil)
	kinds := make(map[PatternKind]bool)
	for _, h := range hints {
		kinds[h.Kind] = true
	}
	if !kinds[PatternContext] {
		t.Error("expected context hint")
	}
	if !kinds[PatternEnvVar] {
		t.Error("expected env var hint")
	}
}
