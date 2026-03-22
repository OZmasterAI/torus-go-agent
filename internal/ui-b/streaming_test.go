package uib

import "testing"

func TestStreamEventChannel(t *testing.T) {
	ch := make(chan StreamEventMsg, 16)
	ch <- StreamEventMsg{Type: StreamTextDelta, Delta: "hello"}
	ev := <-ch
	if ev.Delta != "hello" {
		t.Fatalf("expected 'hello', got %q", ev.Delta)
	}
}

func TestStreamEventChannelMultipleTypes(t *testing.T) {
	ch := make(chan StreamEventMsg, 16)
	ch <- StreamEventMsg{Type: StreamToolStart}
	ch <- StreamEventMsg{Type: StreamToolEnd, Tool: ToolEvent{Name: "bash"}}
	ch <- StreamEventMsg{Type: StreamStatusUpdate, StatusHook: "before_llm_call"}

	ev1 := <-ch
	if ev1.Type != StreamToolStart {
		t.Fatal("expected StreamToolStart")
	}
	ev2 := <-ch
	if ev2.Type != StreamToolEnd {
		t.Fatal("expected StreamToolEnd")
	}
	if ev2.Tool.Name != "bash" {
		t.Fatal("expected bash tool")
	}
	ev3 := <-ch
	if ev3.StatusHook != "before_llm_call" {
		t.Fatal("expected before_llm_call")
	}
}

func TestWaitForStreamEventNilChannel(t *testing.T) {
	cmd := waitForStreamEvent(nil)
	if cmd != nil {
		t.Fatal("nil channel should return nil cmd")
	}
}
