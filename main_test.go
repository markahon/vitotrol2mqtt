package main

import (
	"testing"
	"time"

	"github.com/benvanmierloo/go-vitotrol"
)

func TestGetAttrValue(t *testing.T) {
	const id = vitotrol.AttrID(0xfff0)
	vitotrol.AddAttributeRef(id, vitotrol.AttrRef{Type: vitotrol.TypeDouble, Name: "TestAttr"})

	vdev := &vitotrol.Device{Attributes: map[vitotrol.AttrID]*vitotrol.Value{}}
	if got := getAttrValue(vdev, id); got != nil {
		t.Errorf("missing attribute: got %v", got)
	}
	for raw, want := range map[string]interface{}{
		"21,5":                21.5,
		"2026-11-06 09:00:00": "2026-11-06 09:00:00",
		"":                    "",
	} {
		vdev.Attributes[id] = &vitotrol.Value{Value: raw}
		if got := getAttrValue(vdev, id); got != want {
			t.Errorf("%q: got %#v, want %#v", raw, got, want)
		}
	}
}

func TestTimeoutAction(t *testing.T) {
	for _, c := range []struct {
		elapsed time.Duration
		sent    bool
		want    string
	}{
		{0, false, "skip"},
		{wakeupAfter - time.Second, false, "skip"},
		{wakeupAfter, false, "wakeup"},
		{wakeupAfter + time.Minute, true, "skip"}, // nudged once only
		{giveUpAfter, true, "exit"},
		{giveUpAfter, false, "exit"},
	} {
		if got := timeoutAction(c.elapsed, c.sent); got != c.want {
			t.Errorf("timeoutAction(%s, %v) = %q, want %q", c.elapsed, c.sent, got, c.want)
		}
	}
}
