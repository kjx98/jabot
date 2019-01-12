package jabot

import (
	"testing"
)

var cfg = NewConfig("")

func TestTuling(t *testing.T) {
	w, err := NewJabot(&cfg)
	if err != nil {
		t.Error("NewJabot", err)
		return
	}
	if ss, err := w.getTulingReply("你好", "123123123"); err != nil {
		t.Error("getTulingReply", err)
		return
	} else {
		t.Log("got reply:", ss)
	}
}
