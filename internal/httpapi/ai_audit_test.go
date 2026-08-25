package httpapi

import "testing"

func TestUsageCounterReadsAStreamOnItsWayPast(t *testing.T) {
	counter := &usageCounter{}
	for _, line := range []string{
		": keep-alive\n",
		`data: {"choices":[{"delta":{"content":"안녕"}}]}` + "\n",
		`data: {"choices":[{"delta":{"content":"하세요"}}]}` + "\n",
		`data: {"choices":[{"delta":{}}],"usage":{"prompt_tokens":120,"completion_tokens":8}}` + "\n",
		"data: [DONE]\n",
	} {
		counter.feed([]byte(line))
	}
	if counter.chars != 5 {
		t.Fatalf("answer size = %d, want 5 runes", counter.chars)
	}
	if counter.usage.PromptTokens != 120 || counter.usage.CompletionTokens != 8 {
		t.Fatalf("usage = %+v", counter.usage)
	}
	if counter.usage.TotalTokens != 128 {
		t.Fatalf("total should be filled in when the provider omits it: %d", counter.usage.TotalTokens)
	}
}

func TestUsageCounterReadsAWholeCompletion(t *testing.T) {
	counter := &usageCounter{}
	counter.feedJSON([]byte(`{"choices":[{"message":{"role":"assistant","content":"네 글자"}}],
		"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14}}`))
	if counter.chars != 4 {
		t.Fatalf("answer size = %d", counter.chars)
	}
	if counter.usage.TotalTokens != 14 {
		t.Fatalf("usage = %+v", counter.usage)
	}
}

func TestUsageIsAbsentRatherThanZeroWhenTheProviderSaysNothing(t *testing.T) {
	counter := &usageCounter{}
	counter.feedJSON([]byte(`{"choices":[{"message":{"content":"답"}}]}`))
	if counter.usage.TotalTokens != 0 {
		t.Fatalf("expected no usage, got %+v", counter.usage)
	}
	// A missing count must not be stored as a real zero, or the totals lie.
	if nullInt64(counter.usage.PromptTokens) != nil {
		t.Fatal("a zero token count should be recorded as unknown")
	}
}

func TestInvocationStartsAsCompletedAndCanFail(t *testing.T) {
	invocation := startAIInvocation(User{}, "", "test-model", nil, 42)
	if invocation.action != "chat" {
		t.Fatalf("an unnamed call should be recorded as chat, got %q", invocation.action)
	}
	if invocation.status != aiStatusCompleted {
		t.Fatalf("status = %q", invocation.status)
	}
	invocation.fail("AI_UPSTREAM_ERROR", errFake{})
	if invocation.status != aiStatusFailed || invocation.errorCode != "AI_UPSTREAM_ERROR" {
		t.Fatalf("failure was not recorded: %+v", invocation)
	}
	if invocation.errorText == "" {
		t.Fatal("the failure message should be kept")
	}
}

func TestMessageCharsCountsWhatWasSent(t *testing.T) {
	size := messageChars([]aiMessage{
		{Role: "system", Content: "지침"},
		{Role: "user", Content: "질문입니다"},
	})
	if size != 7 {
		t.Fatalf("prompt size = %d, want 7 runes", size)
	}
}

type errFake struct{}

func (errFake) Error() string { return "gateway said no" }
