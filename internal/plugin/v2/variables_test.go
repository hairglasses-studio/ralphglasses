package v2

import (
	"testing"
)

func TestResolveBasicSubstitution(t *testing.T) {
	ctx := VarContext{
		Repo:      "myrepo",
		Session:   "sess-1",
		Namespace: "core",
		Provider:  "claude",
		Model:     "opus",
		WorkDir:   "/tmp/work",
		HomeDir:   "/home/user",
	}

	tests := []struct {
		name     string
		template string
		want     string
	}{
		{"repo", "clone $REPO", "clone myrepo"},
		{"session", "attach $SESSION", "attach sess-1"},
		{"namespace", "ns=$NAMESPACE", "ns=core"},
		{"provider", "use $PROVIDER", "use claude"},
		{"model", "run $MODEL", "run opus"},
		{"workdir", "cd $WORKDIR", "cd /tmp/work"},
		{"home", "ls $HOME", "ls /home/user"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(tt.template, ctx)
			if got != tt.want {
				t.Errorf("Resolve(%q) = %q, want %q", tt.template, got, tt.want)
			}
		})
	}
}

func TestResolveBraceForm(t *testing.T) {
	ctx := VarContext{Repo: "myrepo", Session: "s1"}

	tests := []struct {
		template string
		want     string
	}{
		{"${REPO}-build", "myrepo-build"},
		{"${SESSION}log", "s1log"},
		{"${REPO}/${SESSION}", "myrepo/s1"},
	}

	for _, tt := range tests {
		t.Run(tt.template, func(t *testing.T) {
			got := Resolve(tt.template, ctx)
			if got != tt.want {
				t.Errorf("Resolve(%q) = %q, want %q", tt.template, got, tt.want)
			}
		})
	}
}

func TestResolveEscaping(t *testing.T) {
	ctx := VarContext{Repo: "myrepo"}

	tests := []struct {
		template string
		want     string
	}{
		{"$$REPO", "$REPO"},
		{"cost is $$5", "cost is $5"},
		{"$$$$REPO", "$$REPO"},
		{"$$$REPO", "$myrepo"},
	}

	for _, tt := range tests {
		t.Run(tt.template, func(t *testing.T) {
			got := Resolve(tt.template, ctx)
			if got != tt.want {
				t.Errorf("Resolve(%q) = %q, want %q", tt.template, got, tt.want)
			}
		})
	}
}

func TestResolveUnknownVariables(t *testing.T) {
	ctx := VarContext{Repo: "myrepo"}

	// Default: leave unknown as-is
	got := Resolve("$UNKNOWN stays", ctx)
	want := "$UNKNOWN stays"
	if got != want {
		t.Errorf("Resolve unknown = %q, want %q", got, want)
	}

	got = Resolve("${UNKNOWN} stays", ctx)
	want = "${UNKNOWN} stays"
	if got != want {
		t.Errorf("Resolve unknown brace = %q, want %q", got, want)
	}

	// WithEmptyUndefined: replace with empty
	got = Resolve("$UNKNOWN gone", ctx, WithEmptyUndefined())
	want = " gone"
	if got != want {
		t.Errorf("Resolve unknown empty = %q, want %q", got, want)
	}

	got = Resolve("${UNKNOWN}gone", ctx, WithEmptyUndefined())
	want = "gone"
	if got != want {
		t.Errorf("Resolve unknown brace empty = %q, want %q", got, want)
	}
}

func TestResolveEmptyContextValues(t *testing.T) {
	ctx := VarContext{} // all zero values

	got := Resolve("repo=$REPO session=$SESSION", ctx)
	want := "repo= session="
	if got != want {
		t.Errorf("Resolve empty ctx = %q, want %q", got, want)
	}
}

func TestResolveMultipleSubstitutions(t *testing.T) {
	ctx := VarContext{
		Repo:    "myrepo",
		Session: "s1",
		WorkDir: "/work",
	}

	got := Resolve("cd $WORKDIR && git clone $REPO && ralph start $SESSION", ctx)
	want := "cd /work && git clone myrepo && ralph start s1"
	if got != want {
		t.Errorf("Resolve multi = %q, want %q", got, want)
	}
}

func TestResolveNoVariables(t *testing.T) {
	ctx := VarContext{Repo: "myrepo"}

	got := Resolve("just plain text", ctx)
	if got != "just plain text" {
		t.Errorf("Resolve plain = %q", got)
	}
}

func TestResolveUnterminatedBrace(t *testing.T) {
	ctx := VarContext{Repo: "myrepo"}

	got := Resolve("${REPO", ctx)
	want := "${REPO"
	if got != want {
		t.Errorf("Resolve unterminated = %q, want %q", got, want)
	}
}

func TestResolveBareDoller(t *testing.T) {
	ctx := VarContext{}

	got := Resolve("$ alone", ctx)
	want := "$ alone"
	if got != want {
		t.Errorf("Resolve bare dollar = %q, want %q", got, want)
	}
}

func TestResolveAll(t *testing.T) {
	ctx := VarContext{
		Repo:    "r",
		Session: "s",
	}

	templates := []string{"$REPO", "$SESSION", "plain"}
	got := ResolveAll(templates, ctx)

	want := []string{"r", "s", "plain"}
	if len(got) != len(want) {
		t.Fatalf("ResolveAll len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ResolveAll[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResolveAllNil(t *testing.T) {
	got := ResolveAll(nil, VarContext{})
	if len(got) != 0 {
		t.Errorf("ResolveAll(nil) = %v, want empty", got)
	}
}

func TestResolveAllWithOptions(t *testing.T) {
	ctx := VarContext{Repo: "r"}
	got := ResolveAll([]string{"$REPO", "$UNKNOWN"}, ctx, WithEmptyUndefined())

	if got[0] != "r" {
		t.Errorf("got[0] = %q, want %q", got[0], "r")
	}
	if got[1] != "" {
		t.Errorf("got[1] = %q, want empty", got[1])
	}
}
