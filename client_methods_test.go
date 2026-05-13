package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestHandleFSRead(t *testing.T) {
	old := enableFS
	enableFS = true
	defer func() { enableFS = old }()

	tmp := t.TempDir()
	testFile := filepath.Join(tmp, "hello.txt")
	os.WriteFile(testFile, []byte("hello world"), 0644)

	t.Run("reads file successfully", func(t *testing.T) {
		params, _ := json.Marshal(FSReadParams{Path: testFile})
		result, rpcErr := handleFSRead(params, tmp)
		if rpcErr != nil {
			t.Fatalf("unexpected error: %v", rpcErr)
		}
		var out map[string]string
		json.Unmarshal(result, &out)
		if out["contents"] != "hello world" {
			t.Errorf("contents = %q, want %q", out["contents"], "hello world")
		}
	})

	t.Run("resolves relative path", func(t *testing.T) {
		params, _ := json.Marshal(FSReadParams{Path: "hello.txt"})
		result, rpcErr := handleFSRead(params, tmp)
		if rpcErr != nil {
			t.Fatalf("unexpected error: %v", rpcErr)
		}
		var out map[string]string
		json.Unmarshal(result, &out)
		if out["contents"] != "hello world" {
			t.Errorf("contents = %q, want %q", out["contents"], "hello world")
		}
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		params, _ := json.Marshal(FSReadParams{Path: filepath.Join(tmp, "nope.txt")})
		_, rpcErr := handleFSRead(params, tmp)
		if rpcErr == nil {
			t.Fatal("expected error")
		}
		if rpcErr.Code != -32000 {
			t.Errorf("code = %d, want -32000", rpcErr.Code)
		}
	})

	t.Run("returns error for empty path", func(t *testing.T) {
		params, _ := json.Marshal(FSReadParams{Path: ""})
		_, rpcErr := handleFSRead(params, tmp)
		if rpcErr == nil {
			t.Fatal("expected error")
		}
		if rpcErr.Code != -32602 {
			t.Errorf("code = %d, want -32602", rpcErr.Code)
		}
	})

	t.Run("disabled when flag off", func(t *testing.T) {
		enableFS = false
		params, _ := json.Marshal(FSReadParams{Path: testFile})
		_, rpcErr := handleFSRead(params, tmp)
		if rpcErr == nil {
			t.Fatal("expected error")
		}
		if rpcErr.Code != -32601 {
			t.Errorf("code = %d, want -32601", rpcErr.Code)
		}
		enableFS = true
	})
}

func TestHandleFSWrite(t *testing.T) {
	old := enableFS
	enableFS = true
	defer func() { enableFS = old }()

	tmp := t.TempDir()

	t.Run("writes file successfully", func(t *testing.T) {
		path := filepath.Join(tmp, "out.txt")
		params, _ := json.Marshal(FSWriteParams{Path: path, Contents: "written"})
		result, rpcErr := handleFSWrite(params, tmp)
		if rpcErr != nil {
			t.Fatalf("unexpected error: %v", rpcErr)
		}
		var out map[string]bool
		json.Unmarshal(result, &out)
		if !out["success"] {
			t.Error("expected success=true")
		}
		data, _ := os.ReadFile(path)
		if string(data) != "written" {
			t.Errorf("file contents = %q, want %q", string(data), "written")
		}
	})

	t.Run("creates parent directories", func(t *testing.T) {
		path := filepath.Join(tmp, "sub", "dir", "file.txt")
		params, _ := json.Marshal(FSWriteParams{Path: path, Contents: "nested"})
		_, rpcErr := handleFSWrite(params, tmp)
		if rpcErr != nil {
			t.Fatalf("unexpected error: %v", rpcErr)
		}
		data, _ := os.ReadFile(path)
		if string(data) != "nested" {
			t.Errorf("file contents = %q, want %q", string(data), "nested")
		}
	})

	t.Run("resolves relative path", func(t *testing.T) {
		params, _ := json.Marshal(FSWriteParams{Path: "relative.txt", Contents: "rel"})
		_, rpcErr := handleFSWrite(params, tmp)
		if rpcErr != nil {
			t.Fatalf("unexpected error: %v", rpcErr)
		}
		data, _ := os.ReadFile(filepath.Join(tmp, "relative.txt"))
		if string(data) != "rel" {
			t.Errorf("file contents = %q, want %q", string(data), "rel")
		}
	})

	t.Run("returns error for empty path", func(t *testing.T) {
		params, _ := json.Marshal(FSWriteParams{Path: "", Contents: "x"})
		_, rpcErr := handleFSWrite(params, tmp)
		if rpcErr == nil {
			t.Fatal("expected error")
		}
		if rpcErr.Code != -32602 {
			t.Errorf("code = %d, want -32602", rpcErr.Code)
		}
	})

	t.Run("disabled when flag off", func(t *testing.T) {
		enableFS = false
		params, _ := json.Marshal(FSWriteParams{Path: "x.txt", Contents: "x"})
		_, rpcErr := handleFSWrite(params, tmp)
		if rpcErr == nil {
			t.Fatal("expected error")
		}
		if rpcErr.Code != -32601 {
			t.Errorf("code = %d, want -32601", rpcErr.Code)
		}
		enableFS = true
	})
}

func TestHandleTerminalCreateAndWait(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	old := enableTerminal
	enableTerminal = true
	defer func() { enableTerminal = old }()

	tmp := t.TempDir()

	t.Run("create and wait for echo", func(t *testing.T) {
		params, _ := json.Marshal(TerminalCreateParams{
			Command: "echo",
			Args:    []string{"hello terminal"},
			Cwd:     tmp,
		})
		result, rpcErr := handleTerminalCreate(params, tmp)
		if rpcErr != nil {
			t.Fatalf("create error: %v", rpcErr)
		}
		var createOut map[string]string
		json.Unmarshal(result, &createOut)
		termID := createOut["terminalId"]
		if termID == "" {
			t.Fatal("empty terminalId")
		}

		waitParams, _ := json.Marshal(TerminalIDParams{TerminalID: termID})
		waitResult, rpcErr := handleTerminalWaitForExit(waitParams)
		if rpcErr != nil {
			t.Fatalf("wait error: %v", rpcErr)
		}
		var waitOut map[string]any
		json.Unmarshal(waitResult, &waitOut)
		if waitOut["exitCode"].(float64) != 0 {
			t.Errorf("exitCode = %v, want 0", waitOut["exitCode"])
		}
	})

	t.Run("create with non-zero exit", func(t *testing.T) {
		params, _ := json.Marshal(TerminalCreateParams{
			Command: "sh",
			Args:    []string{"-c", "exit 42"},
		})
		result, rpcErr := handleTerminalCreate(params, tmp)
		if rpcErr != nil {
			t.Fatalf("create error: %v", rpcErr)
		}
		var createOut map[string]string
		json.Unmarshal(result, &createOut)

		waitParams, _ := json.Marshal(TerminalIDParams{TerminalID: createOut["terminalId"]})
		waitResult, _ := handleTerminalWaitForExit(waitParams)
		var waitOut map[string]any
		json.Unmarshal(waitResult, &waitOut)
		if waitOut["exitCode"].(float64) != 42 {
			t.Errorf("exitCode = %v, want 42", waitOut["exitCode"])
		}
	})

	t.Run("disabled when flag off", func(t *testing.T) {
		enableTerminal = false
		params, _ := json.Marshal(TerminalCreateParams{Command: "echo", Args: []string{"x"}})
		_, rpcErr := handleTerminalCreate(params, tmp)
		if rpcErr == nil {
			t.Fatal("expected error")
		}
		if rpcErr.Code != -32601 {
			t.Errorf("code = %d, want -32601", rpcErr.Code)
		}
		enableTerminal = true
	})

	t.Run("not found terminal", func(t *testing.T) {
		params, _ := json.Marshal(TerminalIDParams{TerminalID: "term-nonexistent"})
		_, rpcErr := handleTerminalOutput(params)
		if rpcErr == nil {
			t.Fatal("expected error")
		}
		if rpcErr.Code != -32000 {
			t.Errorf("code = %d, want -32000", rpcErr.Code)
		}
	})
}

func TestHandleTerminalKill(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	old := enableTerminal
	enableTerminal = true
	defer func() { enableTerminal = old }()

	params, _ := json.Marshal(TerminalCreateParams{
		Command: "sleep",
		Args:    []string{"60"},
	})
	result, rpcErr := handleTerminalCreate(params, t.TempDir())
	if rpcErr != nil {
		t.Fatalf("create error: %v", rpcErr)
	}
	var createOut map[string]string
	json.Unmarshal(result, &createOut)
	termID := createOut["terminalId"]

	killParams, _ := json.Marshal(TerminalIDParams{TerminalID: termID})
	killResult, rpcErr := handleTerminalKill(killParams)
	if rpcErr != nil {
		t.Fatalf("kill error: %v", rpcErr)
	}
	var killOut map[string]bool
	json.Unmarshal(killResult, &killOut)
	if !killOut["success"] {
		t.Error("expected success=true")
	}
}

func TestHandleTerminalRelease(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	old := enableTerminal
	enableTerminal = true
	defer func() { enableTerminal = old }()

	params, _ := json.Marshal(TerminalCreateParams{
		Command: "echo",
		Args:    []string{"done"},
	})
	result, _ := handleTerminalCreate(params, t.TempDir())
	var createOut map[string]string
	json.Unmarshal(result, &createOut)

	releaseParams, _ := json.Marshal(TerminalIDParams{TerminalID: createOut["terminalId"]})
	releaseResult, rpcErr := handleTerminalRelease(releaseParams)
	if rpcErr != nil {
		t.Fatalf("release error: %v", rpcErr)
	}
	var releaseOut map[string]bool
	json.Unmarshal(releaseResult, &releaseOut)
	if !releaseOut["success"] {
		t.Error("expected success=true")
	}
}

func TestResolvePath(t *testing.T) {
	tests := []struct {
		path string
		cwd  string
		want string
	}{
		{"/absolute/path", "/cwd", "/absolute/path"},
		{"relative/path", "/cwd", "/cwd/relative/path"},
		{"../up", "/cwd/sub", "/cwd/up"},
		{".", "/cwd", "/cwd"},
	}
	for _, tt := range tests {
		got := resolvePath(tt.path, tt.cwd)
		if got != tt.want {
			t.Errorf("resolvePath(%q, %q) = %q, want %q", tt.path, tt.cwd, got, tt.want)
		}
	}
}
