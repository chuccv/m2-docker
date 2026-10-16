package run

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type Runner interface {
	Run(name string, args ...string) (stdout string, err error)
	Stream(name string, args ...string) error
	Pipe(stdin io.Reader, stdout io.Writer, name string, args ...string) error
}

type ExecRunner struct{}

func (ExecRunner) Run(name string, args ...string) (string, error) {
	var out, errBuf bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		if msg := strings.TrimSpace(errBuf.String()); msg != "" {
			err = fmt.Errorf("%w: %s", err, msg)
		}
	}
	return out.String(), err
}

func (ExecRunner) Stream(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (ExecRunner) Pipe(stdin io.Reader, stdout io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type FakeRunner struct {
	Calls  [][]string
	Out    map[string]string
	Err    map[string]error
	Stdins map[string][]byte
}

func (f *FakeRunner) Pipe(stdin io.Reader, stdout io.Writer, name string, args ...string) error {
	out, err := f.Run(name, args...)
	key := strings.Join(append([]string{name}, args...), " ")
	if stdin != nil {
		data, _ := io.ReadAll(stdin)
		if f.Stdins == nil {
			f.Stdins = map[string][]byte{}
		}
		f.Stdins[key] = data
	}
	if stdout != nil {
		io.WriteString(stdout, out)
	}
	return err
}

func (f *FakeRunner) Run(name string, args ...string) (string, error) {
	call := append([]string{name}, args...)
	f.Calls = append(f.Calls, call)
	key := strings.Join(call, " ")
	return f.Out[key], f.Err[key]
}

func (f *FakeRunner) Stream(name string, args ...string) error {
	_, err := f.Run(name, args...)
	return err
}
