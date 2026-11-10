package api

import (
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/route"
)

// startStoryMediaBinary exercises the same formal-journal fixture through an
// independently built executable and real TCP. It never calls a model or writes
// to the supplied installation; only the test-owned workspace is writable.
func startStoryMediaBinary(t *testing.T, executable, workspace string) func(*route.Engine, string, string, *ut.Body, ...ut.Header) *ut.ResponseRecorder {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err = listener.Close(); err != nil {
		t.Fatal(err)
	}
	log, err := os.Create(filepath.Join(t.TempDir(), "binary.log"))
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(executable, "--no-open", "--port="+strconv.Itoa(port), "--workspace", workspace)
	command.Dir = t.TempDir()
	command.Env = append(os.Environ(), "TA_DIR="+workspace, "DENOVA_DIR=", "TA_WORKSPACE="+workspace, "DENOVA_WORKSPACE=", "TA_WEB_DIR=")
	command.Stdout, command.Stderr = log, log
	if err = command.Start(); err != nil {
		_ = log.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = command.Process.Kill()
		_ = command.Wait()
		_ = log.Close()
	})
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	client := &http.Client{Timeout: 10 * time.Second}
	deadline := time.Now().Add(10 * time.Second)
	for {
		response, requestErr := client.Get(base + "/api/status")
		if requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("packaged media server did not start: %v", requestErr)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Log("Using the packaged executable over real TCP; scene and knowledge facts remain deterministic fixtures")
	return func(_ *route.Engine, method, target string, body *ut.Body, headers ...ut.Header) *ut.ResponseRecorder {
		t.Helper()
		var input io.Reader
		if body != nil {
			input = body.Body
		}
		request, err := http.NewRequest(method, base+target, input)
		if err != nil {
			t.Fatal(err)
		}
		for _, header := range headers {
			request.Header.Set(header.Key, header.Value)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		recorder := ut.NewRecorder()
		for name, values := range response.Header {
			for _, value := range values {
				recorder.Header().Add(name, value)
			}
		}
		recorder.WriteHeader(response.StatusCode)
		if _, err = io.Copy(recorder, response.Body); err != nil {
			t.Fatal(err)
		}
		return recorder
	}
}
