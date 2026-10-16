package run

import (
	"errors"
	"strings"
	"testing"
)

func TestFakeRunnerPipe(t *testing.T) {
	f := &FakeRunner{Out: map[string]string{"docker dump": "DATA"}}
	var out strings.Builder
	if err := f.Pipe(strings.NewReader("IN"), &out, "docker", "dump"); err != nil {
		t.Fatal(err)
	}
	if out.String() != "DATA" || string(f.Stdins["docker dump"]) != "IN" || len(f.Calls) != 1 {
		t.Fatalf("out %q stdins %v calls %v", out.String(), f.Stdins, f.Calls)
	}
}

func TestExecRunnerPipe(t *testing.T) {
	var out strings.Builder
	if err := (ExecRunner{}).Pipe(strings.NewReader("hi"), &out, "cat"); err != nil || out.String() != "hi" {
		t.Fatalf("got %q %v", out.String(), err)
	}
}

func TestFakeRunnerRecordsAndReturns(t *testing.T) {
	f := &FakeRunner{
		Out: map[string]string{"docker ps -a": "x"},
		Err: map[string]error{"docker fail": errors.New("boom")},
	}
	out, err := f.Run("docker", "ps", "-a")
	if err != nil || out != "x" {
		t.Fatalf("got %q %v", out, err)
	}
	if _, err := f.Run("docker", "fail"); err == nil {
		t.Fatal("want error")
	}
	if out, err := f.Run("docker", "none"); out != "" || err != nil {
		t.Fatalf("got %q %v", out, err)
	}
	if err := f.Stream("docker", "logs"); err != nil {
		t.Fatal(err)
	}
	if len(f.Calls) != 4 || f.Calls[3][0] != "docker" || f.Calls[3][1] != "logs" {
		t.Fatalf("calls %v", f.Calls)
	}
}

func TestExecRunnerRun(t *testing.T) {
	out, err := ExecRunner{}.Run("echo", "hi")
	if err != nil || out != "hi\n" {
		t.Fatalf("got %q %v", out, err)
	}
	if _, err := (ExecRunner{}).Run("false"); err == nil {
		t.Fatal("want error")
	}
}

func TestExecRunnerStream(t *testing.T) {
	if err := (ExecRunner{}).Stream("true"); err != nil {
		t.Fatal(err)
	}
}

func TestExecRunnerRunIncludesStderr(t *testing.T) {
	_, err := (ExecRunner{}).Run("sh", "-c", "echo boom >&2; exit 3")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err %v", err)
	}
}
