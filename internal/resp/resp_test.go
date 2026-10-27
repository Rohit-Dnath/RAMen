package resp

import (
	"bytes"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestReadCommandArray(t *testing.T) {
	in := "*3\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n"
	got, err := NewReader(strings.NewReader(in)).ReadCommand()
	if err != nil {
		t.Fatalf("ReadCommand: %v", err)
	}
	want := []string{"SET", "foo", "bar"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestReadCommandInline(t *testing.T) {
	got, err := NewReader(strings.NewReader("PING\r\n")).ReadCommand()
	if err != nil {
		t.Fatalf("ReadCommand: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"PING"}) {
		t.Fatalf("got %v", got)
	}
}

func TestReadCommandInlineArgs(t *testing.T) {
	got, err := NewReader(strings.NewReader("SET   foo bar\r\n")).ReadCommand()
	if err != nil {
		t.Fatalf("ReadCommand: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"SET", "foo", "bar"}) {
		t.Fatalf("got %v", got)
	}
}

func TestReadEmptyBulk(t *testing.T) {
	got, err := NewReader(strings.NewReader("*1\r\n$0\r\n\r\n")).ReadCommand()
	if err != nil {
		t.Fatalf("ReadCommand: %v", err)
	}
	if !reflect.DeepEqual(got, []string{""}) {
		t.Fatalf("got %v", got)
	}
}

func TestReadCommandRejectsOversizedLengths(t *testing.T) {
	for _, in := range []string{
		"*9223372036854775807\r\n", // would panic in make
		"*833333333\r\n",           // would allocate ~13 GB up front
		"*1048577\r\n",             // one past maxArrayLen
		"*1\r\n$536870913\r\n",     // one past maxBulkLen
	} {
		_, err := NewReader(strings.NewReader(in)).ReadCommand()
		if !errors.Is(err, ErrInvalidSyntax) {
			t.Errorf("%q: got err %v, want ErrInvalidSyntax", in, err)
		}
	}
}

func TestReadCommandTruncatedLargeBulk(t *testing.T) {
	// Claims 2 MB but sends 5 bytes: must fail without allocating 2 MB
	// up front, and must not return a partial command.
	in := "*1\r\n$2097152\r\nhello"
	got, err := NewReader(strings.NewReader(in)).ReadCommand()
	if err == nil {
		t.Fatalf("got %v, want error", got)
	}
}

func TestReadCommandMissingCRLF(t *testing.T) {
	for _, in := range []string{
		"*1\n$3\r\nfoo\r\n",   // array header ends in bare LF
		"*1\r\n$3\nfoo\r\n",   // bulk header ends in bare LF
		"*1\r\n$3\r\nfooXY",   // bulk body not followed by CRLF
		"*1\r\n$3\r\nfoo\r\r", // bulk body ends in CR CR
		"*1\r\n$2097152\r\n",  // large bulk, body missing entirely
		"PING\n",              // inline command with bare LF
	} {
		got, err := NewReader(strings.NewReader(in)).ReadCommand()
		if err == nil {
			t.Errorf("%q: got %v, want error", in, got)
		}
	}
}

func TestReadCommandLargeBulk(t *testing.T) {
	body := strings.Repeat("x", 2*1024*1024)
	in := "*2\r\n$3\r\nSET\r\n$" + strconv.Itoa(len(body)) + "\r\n" + body + "\r\n"
	got, err := NewReader(strings.NewReader(in)).ReadCommand()
	if err != nil {
		t.Fatalf("ReadCommand: %v", err)
	}
	if len(got) != 2 || got[1] != body {
		t.Fatalf("large bulk not read back intact (got %d args)", len(got))
	}
}

func TestWriterRoundtrip(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	w.WriteSimpleString("OK")
	w.WriteInteger(42)
	w.WriteBulkString("hi")
	w.WriteNull()
	w.WriteStringArray([]string{"a", "b"})
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	want := "+OK\r\n:42\r\n$2\r\nhi\r\n$-1\r\n*2\r\n$1\r\na\r\n$1\r\nb\r\n"
	if buf.String() != want {
		t.Fatalf("got %q want %q", buf.String(), want)
	}
}

// FuzzReadCommand feeds arbitrary bytes to the parser. It must never panic or
// hang, whatever a client sends: errors are fine, crashes are not.
func FuzzReadCommand(f *testing.F) {
	seeds := []string{
		"*3\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n",
		"*1\r\n$0\r\n\r\n",
		"*1\r\n$-1\r\n",
		"*-1\r\n",
		"*0\r\n",
		"PING\r\n",
		"SET   foo bar\r\n",
		"\r\n",
		"*2\r\n$3\r\nGET\r\n",
		"*1\r\n:1\r\n",
		"*1\r\n$3\r\nfoo\r\n",
		"*abc\r\n",
		"*1\r\n$3\r\nfooXY",
		"*1\n$3\r\nfoo\r\n",
		"*9223372036854775807\r\n",
		"*1\r\n$9223372036854775807\r\n",
		"*1\r\n$536870912\r\n",
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		r := NewReader(bytes.NewReader(data))
		// Keep reading until the input runs out, like serveConn does. Every
		// successful read consumes input, so this always terminates.
		for {
			if _, err := r.ReadCommand(); err != nil {
				return
			}
		}
	})
}
