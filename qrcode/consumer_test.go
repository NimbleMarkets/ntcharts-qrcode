package qrcode_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestDownstreamConsumer(t *testing.T) {
	// A separate main module ignores this library's replace directives, just
	// like a real consumer. In-package tests alone cannot catch that regression.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module consumer.example/qrcode\n\ngo 1.26.8\n\n" +
			"require github.com/NimbleMarkets/ntcharts-qrcode v0.0.0\n\n" +
			"replace github.com/NimbleMarkets/ntcharts-qrcode => " + strconv.Quote(filepath.Dir(cwd)) + "\n",
		"main.go": `package main
import (
    "errors"
    "strings"
    "time"
    "github.com/NimbleMarkets/ntcharts-qrcode/qrcode"
)
func main() {
    time.AfterFunc(10*time.Second, func() { panic("encoding did not terminate") })
    code, err := qrcode.Encode(strings.Repeat("1", 7089), qrcode.Options{Level: qrcode.Low})
    if err != nil { panic(err) }
    if len(code.Matrix()) != 185 { panic("expected version 40") }
    if _, err := qrcode.Encode(strings.Repeat("x", 3000), qrcode.Options{}); !errors.Is(err, qrcode.ErrCapacity) {
        panic("expected capacity error")
    }
}
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", "-mod=mod", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("downstream consumer: %v (context: %v)\n%s", err, ctx.Err(), output)
	}
}
