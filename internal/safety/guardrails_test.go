package safety

import "testing"

func TestGuardrails_OperationAllowlist(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		level GuardrailLevel
		op    OperationType
		want  bool
	}{
		{"L0-read", LevelObserve, OpRead, true},
		{"L0-write", LevelObserve, OpWrite, false},
		{"L0-execute", LevelObserve, OpExecute, false},
		{"L1-read", LevelAutoRecover, OpRead, true},
		{"L1-execute", LevelAutoRecover, OpExecute, true},
		{"L1-write", LevelAutoRecover, OpWrite, false},
		{"L1-launch", LevelAutoRecover, OpLaunch, true},
		{"L2-write", LevelAutoOptimize, OpWrite, true},
		{"L2-push", LevelAutoOptimize, OpGitPush, true},
		{"L2-force", LevelAutoOptimize, OpGitForce, false},
		{"L3-delete", LevelFullAutonomy, OpDelete, true},
		{"L3-force", LevelFullAutonomy, OpGitForce, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewGuardrails(tc.level)
			err := g.CheckOperation(tc.op)
			got := err == nil
			if got != tc.want {
				t.Errorf("CheckOperation(%q) at L%d: got allowed=%v, want %v (err=%v)",
					tc.op, tc.level, got, tc.want, err)
			}
		})
	}
}

func TestGuardrails_GitForceNeverAllowed(t *testing.T) {
	t.Parallel()
	for level := LevelObserve; level <= LevelFullAutonomy; level++ {
		g := NewGuardrails(level)
		if err := g.CheckOperation(OpGitForce); err == nil {
			t.Errorf("git force should be blocked at L%d", level)
		}
	}
}

func TestGuardrails_SensitiveFileAccess(t *testing.T) {
	t.Parallel()

	g := NewGuardrails(LevelFullAutonomy)

	blocked := []string{
		"/repo/.env",
		"/repo/.env.production",
		"/repo/certs/server.pem",
		"/repo/ssl/private.key",
		"/repo/credentials.json",
		"/repo/api-secret.json",
		"/home/user/.ssh/id_rsa",
		"/home/user/.ssh/id_ed25519",
	}
	for _, path := range blocked {
		if err := g.CheckFileAccess(path); err == nil {
			t.Errorf("expected %q to be blocked", path)
		}
	}

	allowed := []string{
		"/repo/main.go",
		"/repo/README.md",
		"/repo/internal/session/types.go",
		"/repo/.gitignore",
	}
	for _, path := range allowed {
		if err := g.CheckFileAccess(path); err != nil {
			t.Errorf("expected %q to be allowed: %v", path, err)
		}
	}
}

func TestGuardrails_GitSafety(t *testing.T) {
	t.Parallel()

	g := NewGuardrails(LevelAutoOptimize)

	// Force push blocked.
	if err := g.CheckGitSafety([]string{"push", "--force", "origin", "main"}); err == nil {
		t.Error("expected force push to be blocked")
	}

	// Push to main blocked below L3.
	if err := g.CheckGitSafety([]string{"push", "origin", "main"}); err == nil {
		t.Error("expected push to main to be blocked at L2")
	}

	// Push to feature branch allowed.
	if err := g.CheckGitSafety([]string{"push", "origin", "feature/foo"}); err != nil {
		t.Errorf("expected push to feature branch to be allowed: %v", err)
	}

	// Push to main allowed at L3.
	g.SetLevel(LevelFullAutonomy)
	if err := g.CheckGitSafety([]string{"push", "origin", "main"}); err != nil {
		t.Errorf("expected push to main at L3: %v", err)
	}
}

func TestGuardrails_BlastRadius(t *testing.T) {
	t.Parallel()

	g := NewGuardrails(LevelFullAutonomy)
	g.SetBlastRadiusLimits(5, 100)

	// Within limits.
	if err := g.RecordModification(3, 50); err != nil {
		t.Errorf("expected within limits: %v", err)
	}

	// Exceeds file limit.
	if err := g.RecordModification(3, 10); err == nil {
		t.Error("expected file blast radius exceeded")
	}
}

func TestGuardrails_BlastRadiusLines(t *testing.T) {
	t.Parallel()

	g := NewGuardrails(LevelFullAutonomy)
	g.SetBlastRadiusLimits(1000, 50)

	if err := g.RecordModification(1, 51); err == nil {
		t.Error("expected line blast radius exceeded")
	}
}

func TestGuardrails_SetLevel(t *testing.T) {
	t.Parallel()

	g := NewGuardrails(LevelObserve)
	if g.Level() != LevelObserve {
		t.Errorf("expected L0, got L%d", g.Level())
	}

	g.SetLevel(LevelAutoOptimize)
	if g.Level() != LevelAutoOptimize {
		t.Errorf("expected L2, got L%d", g.Level())
	}

	// Write now allowed at L2.
	if err := g.CheckOperation(OpWrite); err != nil {
		t.Errorf("write should be allowed at L2: %v", err)
	}
}

func TestGuardrails_BlastRadiusCounts(t *testing.T) {
	t.Parallel()

	g := NewGuardrails(LevelFullAutonomy)
	_ = g.RecordModification(3, 50)
	_ = g.RecordModification(2, 30)

	files, lines := g.BlastRadius()
	if files != 5 {
		t.Errorf("expected 5 files, got %d", files)
	}
	if lines != 80 {
		t.Errorf("expected 80 lines, got %d", lines)
	}
}
