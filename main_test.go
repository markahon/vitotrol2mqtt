package main

import (
	"errors"
	"testing"

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

func TestSkipTimeout(t *testing.T) {
	fails := 0
	if skipTimeout(errors.New("Keine aktive/gültige Session"), &fails) || fails != 0 {
		t.Error("non-timeout error must not be skipped")
	}
	for i := 1; i <= maxTimeouts; i++ {
		if !skipTimeout(vitotrol.ErrTimeout, &fails) {
			t.Errorf("timeout %d should be skipped", i)
		}
	}
	if skipTimeout(vitotrol.ErrTimeout, &fails) {
		t.Error("timeout past maxTimeouts must exit")
	}
}
