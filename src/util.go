package src

import (
	"context"
	"fmt"
	"net/http"
	"io"
	"log"
	"runtime"
	"strings"
	"os"
	"os/exec"
	"path/filepath"
)

const IS_CLEAR = false
const IS_LOCAL = false

type ErrMissing struct {
	message string
}
func (e ErrMissing) Error() string { return e.message }

func Run(input io.Reader, output io.Writer, name string, arg ...string) error {
	// The command you want to execute (for example, "cat" which echoes input)
	L_DEBUG.Printf("%s %s", name, strings.Join(arg, " "))
	cmd := exec.Command(name, arg...)

	cmd.Stdin = input
	cmd.Stderr = os.Stderr
	cmd.Stdout = output

	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}


var http_client = &http.Client{}
func Request(ctx context.Context, method string, headers map[string]string, body io.Reader, target string, cache_id string) (io.ReadCloser, error) {
	shim_path := local_shim(cache_id)
	if (IS_LOCAL) {
		if (IS_CLEAR) {
			_ = os.Remove(shim_path)
		}
		if fh, err := os.Open(shim_path); err != nil {
			if !os.IsNotExist(err) {
				return nil, err
			} else {
				// allow the normal function to run
			}
		} else {
			L_TRACE.Printf("Reading from %s", shim_path)
			return fh, nil
		}
	}
	req := Must(http.NewRequest(method, target, body))
	req.Header.Set("User-Agent", USER_AGENT)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	L_DEBUG.Printf("%s %q", method, target)
	resp, err := http_client.Do(req);
	if err != nil {
		return nil, err
	}

    // Check if the request was successful
	if (resp.StatusCode >= 400) {
		data, _ := io.ReadAll(resp.Body)
		Must1(resp.Body.Close())
		return nil, fmt.Errorf("%s %s\nHTTP %d\n%s", method, target, resp.StatusCode, string(data))
	}
	if (IS_LOCAL) {
		if fh, err := os.Create(shim_path); err != nil {
			return nil, err
		} else {
			io.Copy(fh, resp.Body)
			_ = Must(fh.Seek(0, io.SeekStart))
			L_TRACE.Printf("Reading from %s", shim_path)
			return fh, nil
		}
	} else {
		return resp.Body, nil
	}
}


var L_TRACE = log.New(io.Discard, "", 0)
var L_DEBUG = log.New(io.Discard, "", 0)
var L_INFO = log.New(io.Discard, "", 0)
var L_ERROR = log.New(io.Discard, "", 0)
var L_FATAL = log.New(os.Stderr, "", 0)
const (
	TRACE uint = iota 
	DEBUG
	INFO
	WARN
	ERROR
	FATAL
	PANIC
)

func Set_log_level(writer io.Writer, log_level uint) {
	if log_level <= TRACE { L_TRACE = log.New(writer, "", log.Lshortfile) }
	if log_level <= DEBUG { L_DEBUG = log.New(writer, "", log.Lshortfile) }
	if log_level <= INFO { L_INFO = log.New(writer, "", 0) }
	if log_level <= ERROR { L_ERROR = log.New(writer, "", log.Lshortfile) }
}



func local_shim(filename string) string {
	root := Must(find_go_root())
	return filepath.Join(root, "tmp", strings.ReplaceAll(filename, "/", "-"))
}
func Must[T any](x T, err error) T {
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	return x
}
func Must1(err error) {
	if err != nil {
		fmt.Println(err.Error)
		os.Exit(1)
	}
}
func find_go_root() (string, error) {
	var dir string
	if x, err := os.Getwd(); err != nil {
		return "", err
	} else {
		dir = x
	}

	// Unlikely we will be 1000 folders deep
	for i := 0; i < 1000; i += 1 {
		_, err := os.Stat(filepath.Join(dir, "go.mod"))
		if os.IsNotExist(err) {
			dir = filepath.Dir(dir)
		} else {
			return dir, nil
		}
	}
	return "", fmt.Errorf("Could not find directory with go.mod")
	
}

func Assert(should_true bool) {
	if !should_true {
	    _, filename, line, ok := runtime.Caller(1)
	    if ok {
			L_FATAL.Fatalf("Failed at %s:%d\n", filename, line)
	    } else {
			L_ERROR.Fatalln("Could retrieve line info")
    	}
	}
}


