package team

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncTaskWorktreeLockedPrefersExplicitWorkspacePathMention(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	origCurrentTaskRepoRoot := currentTaskRepoRoot
	origPrepareTaskWorktree := prepareTaskWorktree
	defer func() {
		currentTaskRepoRoot = origCurrentTaskRepoRoot
		prepareTaskWorktree = origPrepareTaskWorktree
	}()

	workspaceRoot := t.TempDir()
	currentRepo := filepath.Join(workspaceRoot, "dunderia")
	targetRepo := filepath.Join(workspaceRoot, "ConveniosWebBNB_Antigo")
	targetDir := filepath.Join(targetRepo, "BNB")
	targetFile := filepath.Join(targetDir, "RecursoHumano.Cadastro.aspx.cs")

	initUsableGitWorktree(t, currentRepo)
	initUsableGitWorktree(t, targetRepo)
	if err := ensureDir(targetDir); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	if err := os.WriteFile(targetFile, []byte("// fixture\n"), 0o644); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	currentTaskRepoRoot = func() (string, error) { return currentRepo, nil }
	prepareTaskWorktree = func(taskID string) (string, string, error) {
		t.Fatalf("prepareTaskWorktree should not run when an explicit external workspace is inferred")
		return "", "", nil
	}

	task := &teamTask{
		ID:            "task-1",
		Channel:       "convenios-legacy",
		Owner:         "builder",
		Status:        "in_progress",
		ExecutionMode: "local_worktree",
		Title:         "Aplicar hotfix no legado",
		Details:       "Executar diretamente em `" + targetFile + "` sem tocar no WUPHF.",
	}

	b := NewBroker()
	if err := b.syncTaskWorktreeLocked(task); err != nil {
		t.Fatalf("syncTaskWorktreeLocked: %v", err)
	}

	if task.ExecutionMode != "external_workspace" {
		t.Fatalf("execution mode = %q, want external_workspace", task.ExecutionMode)
	}
	gotInfo, err := os.Stat(task.WorkspacePath)
	if err != nil {
		t.Fatalf("stat inferred workspace: %v", err)
	}
	wantInfo, err := os.Stat(targetDir)
	if err != nil {
		t.Fatalf("stat target dir: %v", err)
	}
	if got := filepath.Clean(task.WorkspacePath); !os.SameFile(gotInfo, wantInfo) {
		t.Fatalf("workspace path = %q, want %q", got, targetDir)
	}
	if task.WorktreePath != "" || task.WorktreeBranch != "" {
		t.Fatalf("unexpected local worktree left on external task: %+v", task)
	}
}

func TestInferExplicitWorkspacePathForTaskPreservesExplicitSubdirectory(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	origCurrentTaskRepoRoot := currentTaskRepoRoot
	defer func() { currentTaskRepoRoot = origCurrentTaskRepoRoot }()

	workspaceRoot := t.TempDir()
	currentRepo := filepath.Join(workspaceRoot, "dunderia")
	targetRepo := filepath.Join(workspaceRoot, "ConveniosWebBNB_Antigo")
	targetDir := filepath.Join(targetRepo, "WSConvenio")

	initUsableGitWorktree(t, currentRepo)
	initUsableGitWorktree(t, targetRepo)
	if err := ensureDir(targetDir); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}

	currentTaskRepoRoot = func() (string, error) { return currentRepo, nil }

	task := &teamTask{
		Channel:       "convenios-legacy",
		ExecutionMode: "local_worktree",
		Title:         "Atacar slice do servico legado",
		Details:       "Workspace confirmado: `" + targetDir + "` para editar somente o servico alvo.",
	}

	got := inferExplicitWorkspacePathForTask(task)
	gotInfo, err := os.Stat(got)
	if err != nil {
		t.Fatalf("stat inferred workspace: %v", err)
	}
	wantInfo, err := os.Stat(targetDir)
	if err != nil {
		t.Fatalf("stat target dir: %v", err)
	}
	if !os.SameFile(gotInfo, wantInfo) {
		t.Fatalf("workspace path = %q, want %q", got, targetDir)
	}
}

func TestInferSiblingWorkspacePathForTaskSkipsDotDirectories(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	origCurrentTaskRepoRoot := currentTaskRepoRoot
	defer func() { currentTaskRepoRoot = origCurrentTaskRepoRoot }()

	workspaceRoot := t.TempDir()
	currentRepo := filepath.Join(workspaceRoot, "dunderia")
	dotDir := filepath.Join(workspaceRoot, ".wuphf")

	initUsableGitWorktree(t, currentRepo)
	if err := ensureDir(dotDir); err != nil {
		t.Fatalf("mkdir dot dir: %v", err)
	}

	currentTaskRepoRoot = func() (string, error) { return currentRepo, nil }

	task := &teamTask{
		Channel:       "convenios-legacy",
		Owner:         "builder",
		Status:        "in_progress",
		ExecutionMode: "local_worktree",
		Title:         "Validar handoff sem reabrir o slice",
		Details:       "O worktree atual `" + dotDir + "` nao contem o legado e nao deve virar workspace de execucao.",
	}

	if got := inferSiblingWorkspacePathForTask(task); got != "" {
		t.Fatalf("inferSiblingWorkspacePathForTask returned %q for dot directory false positive", got)
	}
}

func TestInferExplicitWorkspacePathForTaskRejectsSameRepoSubdirectory(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	origCurrentTaskRepoRoot := currentTaskRepoRoot
	defer func() { currentTaskRepoRoot = origCurrentTaskRepoRoot }()

	workspaceRoot := t.TempDir()
	currentRepo := filepath.Join(workspaceRoot, "dunderia")
	sameRepoSubdir := filepath.Join(currentRepo, "internal", "team")

	initUsableGitWorktree(t, currentRepo)
	if err := ensureDir(sameRepoSubdir); err != nil {
		t.Fatalf("mkdir same-repo subdir: %v", err)
	}

	currentTaskRepoRoot = func() (string, error) { return currentRepo, nil }

	task := &teamTask{
		Channel:       "general",
		ExecutionMode: "local_worktree",
		Title:         "Aplicar hotfix no broker",
		Details:       "Editar direto em `" + sameRepoSubdir + "` sem worktree dedicado.",
	}

	if got := inferExplicitWorkspacePathForTask(task); got != "" {
		t.Fatalf("expected same-repo subdirectory to be rejected, got %q", got)
	}
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}
