package core

import (
	"strings"
	"testing"
)

func TestStartProgressOmitsSecretsAndNeverClaimsConnection(t *testing.T) {
	f := prepared(t)
	var messages []string
	f.app.Progress = func(s string) { messages = append(messages, s) }
	code, err := f.app.Start(f.o)
	if err != nil || code != 42 {
		t.Fatal(code, err)
	}
	joined := strings.Join(messages, "\n")
	for _, required := range []string{"保存設定", "起動入口", "二重起動", "資格情報ストア", "保存済み資格情報", "子プロセスを準備"} {
		if !strings.Contains(joined, required) {
			t.Fatal("stage missing", required)
		}
	}
	for _, forbidden := range []string{"synthetic-test-key", "tunnel_fixture", "credentialRef", "接続済み"} {
		if strings.Contains(joined, forbidden) {
			t.Fatal("secret or unobserved claim in progress")
		}
	}
}

func TestStartProgressFailureIdentifiesStageBeforeKeyLoad(t *testing.T) {
	f := prepared(t)
	f.r.bad = true
	var messages []string
	f.app.Progress = func(s string) { messages = append(messages, s) }
	if _, err := f.app.Start(f.o); err == nil {
		t.Fatal("fixture failure missing")
	}
	if f.v.loads != 0 || f.r.runs != 0 {
		t.Fatal("failed integrity allowed credential load or launch")
	}
	if len(messages) == 0 || !strings.Contains(messages[len(messages)-1], "起動入口") || !strings.Contains(messages[len(messages)-1], "失敗") {
		t.Fatal(messages)
	}
}
