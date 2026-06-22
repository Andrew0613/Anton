package taskstate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Andrew0613/Anton/internal/adapter"
	runstate "github.com/Andrew0613/Anton/internal/run"
	"gopkg.in/yaml.v3"
)

func TestReadStatusParsesExpectedSchema(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "status.yaml")

	content := "" +
		"version: 1\n" +
		"stable:\n" +
		"  task_id: task-20260416T120000Z\n" +
		"  created_at: 2026-04-16T12:00:00Z\n" +
		"state:\n" +
		"  lifecycle: active\n" +
		"  updated_at: 2026-04-16T12:30:00Z\n" +
		"machine:\n" +
		"  host: devbox\n" +
		"  execution_target: local\n" +
		"  working_directory: /tmp/example\n" +
		"  workspace_kind: plain-directory\n" +
		"evidence:\n" +
		"  attempts: []\n" +
		"  validations: []\n" +
		"closure:\n" +
		"  finish_state: active\n" +
		"  next_step: continue implementation and update progress.md\n" +
		"  blockers: []\n" +
		"  expected_deliverables: []\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write status.yaml: %v", err)
	}

	snapshot, err := adapter.Default{}.ReadStatus(path)
	if err != nil {
		t.Fatalf("ReadStatus returned error: %v", err)
	}

	if snapshot.TaskID != "task-20260416T120000Z" {
		t.Fatalf("task id = %q", snapshot.TaskID)
	}
}

func TestReadStatusRejectsMissingRequiredFields(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "status.yaml")

	content := "" +
		"version: 1\n" +
		"stable:\n" +
		"  task_id: \n" +
		"  created_at: 2026-04-16T12:00:00Z\n" +
		"state:\n" +
		"  lifecycle: active\n" +
		"  updated_at: 2026-04-16T12:30:00Z\n" +
		"machine:\n" +
		"  host: devbox\n" +
		"  execution_target: local\n" +
		"  working_directory: /tmp/example\n" +
		"  workspace_kind: plain-directory\n" +
		"evidence:\n" +
		"  attempts: []\n" +
		"  validations: []\n" +
		"closure:\n" +
		"  finish_state: active\n" +
		"  next_step: continue implementation and update progress.md\n" +
		"  blockers: []\n" +
		"  expected_deliverables: []\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write status.yaml: %v", err)
	}

	if _, err := (adapter.Default{}).ReadStatus(path); err == nil {
		t.Fatalf("ReadStatus should fail for missing task_id")
	}
}

func TestSummarizeBlocksOnMissingOrInvalidFiles(t *testing.T) {
	result := summarize([]fileResult{
		{Status: "existing"},
		{Status: "created"},
		{Status: "missing"},
		{Status: "invalid"},
	})

	if result.Status != statusBlocked {
		t.Fatalf("status = %q, want %q", result.Status, statusBlocked)
	}
	if result.CreatedCount != 1 || result.ExistingCount != 1 || result.MissingCount != 1 || result.InvalidCount != 1 {
		t.Fatalf("unexpected summary counts: %+v", result)
	}
}

func TestClosureGateAcceptsPhysEditCompletedLifecycle(t *testing.T) {
	results := closureGateResults("status.yaml", adapter.StatusSnapshot{
		TaskID:                   "0064_project_progress_retirement_workspace_hygiene",
		Lifecycle:                "completed",
		FinishState:              "completed",
		NextStep:                 "archive compatibility bundle",
		ExpectedDeliverableCount: 1,
	})
	if len(results) != 0 {
		t.Fatalf("completed lifecycle should be accepted, got %+v", results)
	}
}

func TestValidateBundleCompleteFixture(t *testing.T) {
	bundle := adapter.ResolvedTaskBundle{
		Root: bundleFixturePath("complete"),
		RequiredFiles: []adapter.TaskFile{
			{Name: "task_plan.md"},
			{Name: "findings.md"},
			{Name: "progress.md"},
		},
		StatusFile: "status.yaml",
	}
	results := validateBundle(bundle.Root, bundle)

	for _, result := range results {
		if result.Status != "existing" {
			t.Fatalf("expected existing file result, got %+v", result)
		}
	}
}

func TestValidateBundleIncompleteFixture(t *testing.T) {
	bundle := adapter.ResolvedTaskBundle{
		Root: bundleFixturePath("incomplete"),
		RequiredFiles: []adapter.TaskFile{
			{Name: "task_plan.md"},
			{Name: "findings.md"},
			{Name: "progress.md"},
		},
		StatusFile: "status.yaml",
	}
	results := validateBundle(bundle.Root, bundle)

	missingCount := 0
	for _, result := range results {
		if result.Status == "missing" {
			missingCount++
		}
	}
	if missingCount != 2 {
		t.Fatalf("missing count = %d, want 2; results=%+v", missingCount, results)
	}
}

func TestReadStatusFromFixture(t *testing.T) {
	snapshot, err := adapter.Default{}.ReadStatus(filepath.Join(bundleFixturePath("complete"), "status.yaml"))
	if err != nil {
		t.Fatalf("ReadStatus returned error: %v", err)
	}

	if snapshot.TaskID != "task-20260416T120000Z" {
		t.Fatalf("task id = %q", snapshot.TaskID)
	}
}

func TestTaskStateCheckJSONUsesConfiguredTasksRoot(t *testing.T) {
	repoRoot := makeTaskStateTempRepoRoot(t)
	writeTaskStateFile(t, filepath.Join(repoRoot, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: .anton/state\n"+
		"threads:\n  default_project_strategy: repo-root\n",
	)
	writeTaskStateFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Entry\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withTaskStateWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"check", "--json"}, &stdout, &stderr, []string{"ANTON_TASK_ID=demo_task"})
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1 because files are intentionally missing", exitCode)
	}

	replacements := taskStateReplacements(repoRoot)
	assertTaskStateGoldenJSON(t, stdout.Bytes(), "task_state_check_blocked_missing.json", replacements)
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTaskStateInitJSONContract(t *testing.T) {
	repoRoot := makeTaskStateTempRepoRoot(t)
	writeTaskStateFile(t, filepath.Join(repoRoot, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: .anton/state\n"+
		"threads:\n  default_project_strategy: repo-root\n",
	)
	writeTaskStateFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Entry\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withTaskStateWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"init", "--json"}, &stdout, &stderr, []string{"ANTON_TASK_ID=demo_task"})
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	replacements := taskStateReplacements(repoRoot)
	assertTaskStateGoldenJSON(t, stdout.Bytes(), "task_state_init_success.json", replacements)
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTaskStateInitRequiresExplicitTaskIdentity(t *testing.T) {
	repoRoot := makeTaskStateTempRepoRoot(t)
	writeTaskStateFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Entry\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withTaskStateWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"init", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}

	var payload response
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, stdout.String())
	}
	if payload.OK {
		t.Fatalf("expected failure payload")
	}
	if payload.Error == nil || payload.Error.Code != "task-identity-required" {
		t.Fatalf("error = %#v", payload.Error)
	}
	message := payload.Error.Message
	for _, expected := range []string{"ANTON_TASK_ID", "task/<id_slug>", ".anton/tasks"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("message %q missing %q", message, expected)
		}
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".anton")); !os.IsNotExist(err) {
		t.Fatalf("task-state init should not write files without task identity, stat err=%v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTaskStateCheckJSONContractAfterInit(t *testing.T) {
	repoRoot := makeTaskStateTempRepoRoot(t)
	writeTaskStateFile(t, filepath.Join(repoRoot, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: .anton/state\n"+
		"threads:\n  default_project_strategy: repo-root\n",
	)
	writeTaskStateFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Entry\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withTaskStateWorkingDirectory(t, repoRoot, func() int {
		env := []string{"ANTON_TASK_ID=demo_task"}
		if code := Run([]string{"init", "--json"}, io.Discard, io.Discard, env); code != 0 {
			t.Fatalf("init exit code = %d, want 0", code)
		}
		return Run([]string{"check", "--json"}, &stdout, &stderr, env)
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	replacements := taskStateReplacements(repoRoot)
	assertTaskStateGoldenJSON(t, stdout.Bytes(), "task_state_check_success.json", replacements)
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTaskStateRunManifestPlanningModeRequiresRunManifest(t *testing.T) {
	repoRoot := makeTaskStateTempRepoRoot(t)
	writeTaskStateFile(t, filepath.Join(repoRoot, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: .anton/state\n  planning_mode: run_manifest\n"+
		"run:\n  enabled: true\n  manifest: run.json\n  receipts_dir: receipts\n"+
		"threads:\n  default_project_strategy: repo-root\n",
	)
	writeTaskStateFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Entry\n")
	env := []string{"ANTON_TASK_ID=demo_task"}

	var missingStdout bytes.Buffer
	var missingStderr bytes.Buffer
	missingExit := withTaskStateWorkingDirectory(t, repoRoot, func() int {
		if code := Run([]string{"init", "--json"}, io.Discard, io.Discard, env); code != 0 {
			t.Fatalf("init exit code = %d, want 0", code)
		}
		return Run([]string{"check", "--json"}, &missingStdout, &missingStderr, env)
	})
	if missingExit != 1 {
		t.Fatalf("exit code = %d, want 1 while run manifest is missing\nstdout=%s\nstderr=%s", missingExit, missingStdout.String(), missingStderr.String())
	}
	if !strings.Contains(missingStdout.String(), "anton run init") {
		t.Fatalf("missing run manifest guidance not found: %s", missingStdout.String())
	}

	bundleRoot := filepath.Join(repoRoot, ".anton/state/active/demo_task")
	writeTaskStateFile(t, filepath.Join(bundleRoot, "run.json"), "{}\n")

	var invalidStdout bytes.Buffer
	var invalidStderr bytes.Buffer
	invalidExit := withTaskStateWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"check", "--json"}, &invalidStdout, &invalidStderr, env)
	})
	if invalidExit != 1 {
		t.Fatalf("exit code = %d, want 1 while run manifest is invalid\nstdout=%s\nstderr=%s", invalidExit, invalidStdout.String(), invalidStderr.String())
	}
	if !strings.Contains(invalidStdout.String(), "run manifest is not valid") {
		t.Fatalf("invalid run manifest detail not found: %s", invalidStdout.String())
	}

	manifest, err := runstate.NewManifest("demo_task", time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if err := runstate.NewStore(bundleRoot).Save(manifest); err != nil {
		t.Fatalf("save manifest: %v", err)
	}

	var okStdout bytes.Buffer
	var okStderr bytes.Buffer
	okExit := withTaskStateWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"check", "--json"}, &okStdout, &okStderr, env)
	})
	if okExit != 0 {
		t.Fatalf("exit code = %d, want 0 once run manifest exists\nstdout=%s\nstderr=%s", okExit, okStdout.String(), okStderr.String())
	}
	var payload response
	if err := json.Unmarshal(okStdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, okStdout.String())
	}
	if !payload.OK {
		t.Fatalf("expected success payload: %+v", payload.Error)
	}
	for _, file := range payload.Data.Files {
		if strings.HasSuffix(file.Path, "task_plan.md") || strings.HasSuffix(file.Path, "findings.md") || strings.HasSuffix(file.Path, "progress.md") {
			t.Fatalf("run_manifest mode should not require planning file triad: %+v", file)
		}
	}
}

func TestTaskStateTopicLayerCheckResolvesFromEnv(t *testing.T) {
	repoRoot, _ := makeTopicLayerRepo(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withTaskStateWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"check", "--schema", "auto", "--json"}, &stdout, &stderr, []string{"ANTON_TASK_ID=demo_topic_task"})
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout=%s\nstderr=%s", exitCode, stdout.String(), stderr.String())
	}

	var payload response
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, stdout.String())
	}
	if !payload.OK {
		t.Fatalf("expected success payload: %+v", payload.Error)
	}
	if payload.Data == nil || payload.Data.TaskID != "demo_topic_task" {
		t.Fatalf("task id = %#v", payload.Data)
	}
	expected := filepath.Join(repoRoot, "project_progress", "Tooling", "tasks", "active", "demo_topic_task")
	if payload.Data.BundleRoot != expected {
		t.Fatalf("bundle root = %q, want %q", payload.Data.BundleRoot, expected)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTaskStateTopicLayerMissingTopicReportsIdentityRequired(t *testing.T) {
	repoRoot := makeTaskStateTempRepoRoot(t)
	writeTaskStateFile(t, filepath.Join(repoRoot, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: project_progress\n  layout: topic-layer\n  status_schema: physedit-v1\n"+
		"threads:\n  default_project_strategy: repo-root\n",
	)
	writeTaskStateFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Entry\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withTaskStateWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"init", "--json"}, &stdout, &stderr, []string{"ANTON_TASK_ID=demo_topic_task"})
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
	var payload response
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, stdout.String())
	}
	if payload.Error == nil || payload.Error.Code != "task-identity-required" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if !strings.Contains(payload.Error.Message, "ANTON_TASK_TOPIC") {
		t.Fatalf("message missing ANTON_TASK_TOPIC guidance: %q", payload.Error.Message)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTaskStateTopicLayerCheckResolvesFromCWDInsideBundle(t *testing.T) {
	_, bundleRoot := makeTopicLayerRepo(t)
	inside := filepath.Join(bundleRoot, "notes")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatalf("mkdir inside bundle: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withTaskStateWorkingDirectory(t, inside, func() int {
		return Run([]string{"check", "--schema", "physedit-v1", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout=%s\nstderr=%s", exitCode, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"task_id": "demo_topic_task"`) {
		t.Fatalf("stdout missing topic-layer task id: %s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTaskStateTopicLayerMutationsPreserveFieldsAndSyncCard(t *testing.T) {
	repoRoot, bundleRoot := makeTopicLayerRepo(t)
	env := []string{"ANTON_TASK_ID=demo_topic_task"}

	exitCode := withTaskStateWorkingDirectory(t, repoRoot, func() int {
		commands := [][]string{
			{"env", "--json", "--machine-type", "rjob", "--proxy", "on", "--cwd", filepath.Join(repoRoot, "work")},
			{"service", "add", "--json", "--name", "viewer", "--kind", "gradio", "--status", "up", "--reopen-hint", "run viewer"},
			{"freshness", "--json", "--canonical-truth", "status.yaml plus handoff", "--checked-at", "2026-05-13T12:34:56Z", "--current-lane", "implementation", "--last-human-confirmed-state", "unit test state", "--source-file", "progress.md"},
			{"sync-card", "--json"},
		}
		for _, command := range commands {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			if code := Run(command, &stdout, &stderr, env); code != 0 {
				t.Fatalf("%v exit code = %d, want 0\nstdout=%s\nstderr=%s", command, code, stdout.String(), stderr.String())
			}
		}
		return 0
	})
	if exitCode != 0 {
		t.Fatalf("unexpected wrapper exit code %d", exitCode)
	}

	payload := readTaskStateYAMLMap(t, filepath.Join(bundleRoot, "status.yaml"))
	if payload["custom_top"] != "keep-me" {
		t.Fatalf("custom_top not preserved: %#v", payload["custom_top"])
	}
	task := mustMap(t, payload["task"])
	if task["custom_task_field"] != "keep-task" {
		t.Fatalf("custom task field not preserved: %#v", task["custom_task_field"])
	}
	environment := mustMap(t, payload["environment"])
	if environment["machine_type"] != "rjob" || environment["proxy"] != "on" {
		t.Fatalf("environment not updated: %#v", environment)
	}
	execution := mustMap(t, payload["execution"])
	if execution["cwd"] != filepath.Join(repoRoot, "work") {
		t.Fatalf("execution.cwd = %#v", execution["cwd"])
	}
	services, ok := payload["services"].([]any)
	if !ok || len(services) != 1 {
		t.Fatalf("services = %#v", payload["services"])
	}
	service := mustMap(t, services[0])
	if service["name"] != "viewer" || service["reopen_hint"] != "run viewer" {
		t.Fatalf("service not updated: %#v", service)
	}

	cardPath := filepath.Join(repoRoot, "project_progress", "Tooling", "tasks", "active", "demo_topic_task.md")
	card := string(readTaskStateFile(t, cardPath))
	for _, expected := range []string{"KEEP INTRO PROSE", "KEEP TAIL PROSE", "status.yaml plus handoff", "unit test state", "progress.md"} {
		if !strings.Contains(card, expected) {
			t.Fatalf("card missing %q:\n%s", expected, card)
		}
	}
	if strings.Contains(card, "old truth") {
		t.Fatalf("card still contains old generated freshness block:\n%s", card)
	}
}

func TestTaskStateCanonicalLifecycleCommandsBlockConfiguredNonAntonSchema(t *testing.T) {
	repoRoot, _ := makeTopicLayerRepo(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withTaskStateWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"close", "--json", "--state", "review", "--next-step", "review"}, &stdout, &stderr, []string{"ANTON_TASK_ID=demo_topic_task"})
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
	var payload response
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, stdout.String())
	}
	if payload.Error == nil || payload.Error.Code != "unsupported-status-schema" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTaskStateCanonicalLifecycleCommandsDoNotRewriteForeignStatusShape(t *testing.T) {
	repoRoot, bundleRoot := makeTopicLayerRepo(t)
	writeTaskStateFile(t, filepath.Join(repoRoot, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: project_progress\n  topic_layer: true\n"+
		"threads:\n  default_project_strategy: repo-root\n",
	)
	statusPath := filepath.Join(bundleRoot, "status.yaml")
	before := string(readTaskStateFile(t, statusPath))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withTaskStateWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"close", "--json", "--state", "review", "--next-step", "review"}, &stdout, &stderr, []string{"ANTON_TASK_ID=demo_topic_task"})
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
	after := string(readTaskStateFile(t, statusPath))
	if after != before {
		t.Fatalf("status.yaml was rewritten\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if !strings.Contains(stdout.String(), "not an Anton native status schema") {
		t.Fatalf("stdout = %s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTaskStateCheckReportsUnknownConfiguredStatusSchema(t *testing.T) {
	repoRoot := makeTaskStateTempRepoRoot(t)
	writeTaskStateFile(t, filepath.Join(repoRoot, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: project_progress\n  layout: topic-layer\n  status_schema: made-up\n"+
		"threads:\n  default_project_strategy: repo-root\n",
	)
	writeTaskStateFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Entry\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withTaskStateWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"check", "--json"}, &stdout, &stderr, []string{"ANTON_TASK_ID=demo_topic_task"})
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
	var payload response
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, stdout.String())
	}
	if payload.OK || payload.Error == nil || !strings.Contains(payload.Error.Message, "tasks.status_schema") {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTaskStatePulseJSONContract(t *testing.T) {
	repoRoot := makeTaskStateTempRepoRoot(t)
	writeTaskStateFile(t, filepath.Join(repoRoot, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: .anton/state\n"+
		"threads:\n  default_project_strategy: repo-root\n",
	)
	writeTaskStateFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Entry\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withTaskStateWorkingDirectory(t, repoRoot, func() int {
		env := []string{"ANTON_TASK_ID=demo_task"}
		if code := Run([]string{"init", "--json"}, io.Discard, io.Discard, env); code != 0 {
			t.Fatalf("init exit code = %d, want 0", code)
		}
		return Run([]string{"pulse", "--json"}, &stdout, &stderr, env)
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	replacements := taskStateReplacements(repoRoot)
	assertTaskStateGoldenJSON(t, stdout.Bytes(), "task_state_pulse_success.json", replacements)
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTaskStateCheckUsageErrorExitCode(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"check", "--json", "--bad-flag"}, &stdout, &stderr, nil)
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}

	assertTaskStateGoldenJSON(t, stdout.Bytes(), "task_state_usage_error.json", nil)
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTaskStateCheckRuntimeFailureExitCode(t *testing.T) {
	repoRoot := makeTaskStateTempRepoRoot(t)
	writeTaskStateFile(t, filepath.Join(repoRoot, "anton.yaml"), "version: 2\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withTaskStateWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"check", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}

	replacements := taskStateReplacements(repoRoot)
	assertTaskStateGoldenJSON(t, stdout.Bytes(), "task_state_runtime_error.json", replacements)
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTaskStateCloseDoneRequiresValidationEvidence(t *testing.T) {
	repoRoot := makeTaskStateTempRepoRoot(t)
	writeTaskStateFile(t, filepath.Join(repoRoot, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: .anton/state\n"+
		"threads:\n  default_project_strategy: repo-root\n",
	)
	writeTaskStateFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Entry\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withTaskStateWorkingDirectory(t, repoRoot, func() int {
		env := []string{"ANTON_TASK_ID=demo_task"}
		if code := Run([]string{"init", "--json"}, io.Discard, io.Discard, env); code != 0 {
			t.Fatalf("init exit code = %d, want 0", code)
		}
		return Run([]string{"close", "--json", "--state", "done", "--deliverable", "final handoff", "--next-step", "archive task"}, &stdout, &stderr, env)
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stdout.String(), "evidence.validations") {
		t.Fatalf("stdout missing closure gate failure: %s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTaskStateCloseReviewJSON(t *testing.T) {
	repoRoot := makeTaskStateTempRepoRoot(t)
	writeTaskStateFile(t, filepath.Join(repoRoot, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: .anton/state\n"+
		"threads:\n  default_project_strategy: repo-root\n",
	)
	writeTaskStateFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Entry\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withTaskStateWorkingDirectory(t, repoRoot, func() int {
		env := []string{"ANTON_TASK_ID=demo_task"}
		if code := Run([]string{"init", "--json"}, io.Discard, io.Discard, env); code != 0 {
			t.Fatalf("init exit code = %d, want 0", code)
		}
		return Run([]string{"close", "--json", "--state", "review", "--deliverable", "PR opened", "--next-step", "request human review", "--artifact", "PR#1"}, &stdout, &stderr, env)
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			TaskID    string `json:"task_id"`
			Lifecycle struct {
				Lifecycle   string `json:"lifecycle"`
				FinishState string `json:"finish_state"`
			} `json:"lifecycle"`
			Evidence struct {
				ValidationCount int `json:"validation_count"`
			} `json:"evidence"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, stdout.String())
	}
	if !payload.OK {
		t.Fatalf("expected success payload")
	}
	if payload.Data.Lifecycle.Lifecycle != "review" || payload.Data.Lifecycle.FinishState != "review" {
		t.Fatalf("unexpected lifecycle: %+v", payload.Data.Lifecycle)
	}
	if payload.Data.Evidence.ValidationCount == 0 {
		t.Fatalf("expected validation evidence count > 0")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTaskStateRetargetJSON(t *testing.T) {
	repoRoot := makeTaskStateTempRepoRoot(t)
	writeTaskStateFile(t, filepath.Join(repoRoot, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: .anton/state\n"+
		"threads:\n  default_project_strategy: repo-root\n",
	)
	writeTaskStateFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Entry\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withTaskStateWorkingDirectory(t, repoRoot, func() int {
		env := []string{"ANTON_TASK_ID=demo_task"}
		if code := Run([]string{"init", "--json"}, io.Discard, io.Discard, env); code != 0 {
			t.Fatalf("init exit code = %d, want 0", code)
		}
		return Run([]string{"retarget", "--json", "--task-id", "demo_task_v2"}, &stdout, &stderr, env)
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, stdout.String())
	}
	if payload.Data.TaskID != "demo_task_v2" {
		t.Fatalf("task_id = %q, want demo_task_v2", payload.Data.TaskID)
	}
}

func TestTaskStateInitRejectsTraversalTaskID(t *testing.T) {
	repoRoot := makeTaskStateTempRepoRoot(t)
	writeTaskStateFile(t, filepath.Join(repoRoot, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: .anton/state\n"+
		"threads:\n  default_project_strategy: repo-root\n",
	)
	writeTaskStateFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Entry\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withTaskStateWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"init", "--json"}, &stdout, &stderr, []string{"ANTON_TASK_ID=../../escaped"})
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stdout.String(), "invalid task id") {
		t.Fatalf("stdout missing invalid task id error: %s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTaskStatePulsePreservesClosedLifecycle(t *testing.T) {
	repoRoot := makeTaskStateTempRepoRoot(t)
	writeTaskStateFile(t, filepath.Join(repoRoot, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: .anton/state\n"+
		"threads:\n  default_project_strategy: repo-root\n",
	)
	writeTaskStateFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Entry\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withTaskStateWorkingDirectory(t, repoRoot, func() int {
		env := []string{"ANTON_TASK_ID=demo_task"}
		if code := Run([]string{"init", "--json"}, io.Discard, io.Discard, env); code != 0 {
			t.Fatalf("init exit code = %d, want 0", code)
		}
		if code := Run([]string{"close", "--json", "--state", "review", "--deliverable", "PR opened", "--next-step", "request review", "--artifact", "PR#1"}, io.Discard, io.Discard, env); code != 0 {
			t.Fatalf("close exit code = %d, want 0", code)
		}
		return Run([]string{"pulse", "--json"}, &stdout, &stderr, env)
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Lifecycle struct {
				Lifecycle   string `json:"lifecycle"`
				FinishState string `json:"finish_state"`
			} `json:"lifecycle"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, stdout.String())
	}
	if !payload.OK {
		t.Fatalf("expected success payload")
	}
	if payload.Data.Lifecycle.Lifecycle != "review" || payload.Data.Lifecycle.FinishState != "review" {
		t.Fatalf("pulse should preserve review lifecycle, got %+v", payload.Data.Lifecycle)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTaskStateRetargetRejectsInvalidTaskID(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"retarget", "--json", "--task-id", "../../escaped"}, &stdout, &stderr, nil)
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
	if !strings.Contains(stdout.String(), "invalid --task-id") {
		t.Fatalf("stdout missing invalid task id message: %s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestPathWithinRoot(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "repo", ".anton", "tasks", "active")
	inside := filepath.Join(root, "demo_task")
	outside := filepath.Join(root, "..", "escaped")

	if !pathWithinRoot(root, inside) {
		t.Fatalf("inside path should be accepted")
	}
	if pathWithinRoot(root, outside) {
		t.Fatalf("outside path should be rejected")
	}
}

func taskStateReplacements(repoRoot string) map[string]string {
	return map[string]string{
		filepath.Clean(repoRoot):                            "<REPO_ROOT>",
		filepath.Clean(filepath.Join("/private", repoRoot)): "<REPO_ROOT>",
	}
}

func assertTaskStateGoldenJSON(t *testing.T, payload []byte, goldenName string, replacements map[string]string) {
	t.Helper()

	actual := normalizeTaskStateJSON(t, payload, replacements)
	expectedBytes, err := os.ReadFile(resolveTaskStateGoldenPath(t, goldenName))
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenName, err)
	}
	expected := normalizeTaskStateJSON(t, expectedBytes, nil)
	if actual != expected {
		t.Fatalf("json contract mismatch for %s\n--- actual ---\n%s\n--- expected ---\n%s", goldenName, actual, expected)
	}
}

func normalizeTaskStateJSON(t *testing.T, payload []byte, replacements map[string]string) string {
	t.Helper()

	normalized := string(payload)
	keys := make([]string, 0, len(replacements))
	for old := range replacements {
		keys = append(keys, old)
	}
	sort.Slice(keys, func(i int, j int) bool {
		return len(keys[i]) > len(keys[j])
	})
	for _, old := range keys {
		normalized = strings.ReplaceAll(normalized, old, replacements[old])
	}

	var value any
	if err := json.Unmarshal([]byte(normalized), &value); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, normalized)
	}

	canonical, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	return fmt.Sprintf("%s\n", canonical)
}

func resolveTaskStateGoldenPath(t *testing.T, name string) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve caller path for golden file %s", name)
	}
	return filepath.Join(filepath.Dir(thisFile), "testdata", "golden", name)
}

func bundleFixturePath(name string) string {
	return filepath.Join("testdata", "bundles", name)
}

func withTaskStateWorkingDirectory(t *testing.T, path string, fn func() int) int {
	t.Helper()

	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(path); err != nil {
		t.Fatalf("chdir %s: %v", path, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Fatalf("restore chdir: %v", err)
		}
	})
	return fn()
}

func makeTaskStateTempRepoRoot(t *testing.T) string {
	t.Helper()

	repoRoot := t.TempDir()
	gitDir := filepath.Join(repoRoot, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	writeTaskStateFile(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/main\n")
	return repoRoot
}

func writeTaskStateFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func makeTopicLayerRepo(t *testing.T) (string, string) {
	t.Helper()

	repoRoot := makeTaskStateTempRepoRoot(t)
	writeTaskStateFile(t, filepath.Join(repoRoot, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: project_progress\n  layout: topic-layer\n  status_schema: physedit-v1\n  card_sync: true\n"+
		"threads:\n  default_project_strategy: repo-root\n",
	)
	writeTaskStateFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Entry\n")

	bundleRoot := filepath.Join(repoRoot, "project_progress", "Tooling", "tasks", "active", "demo_topic_task")
	writeTaskStateFile(t, filepath.Join(bundleRoot, "task_plan.md"), "# Task Plan\n")
	writeTaskStateFile(t, filepath.Join(bundleRoot, "findings.md"), "# Findings\n")
	writeTaskStateFile(t, filepath.Join(bundleRoot, "progress.md"), "# Progress\n")
	status := strings.ReplaceAll(string(readTaskStateFixture(t, "topic-layer", "status.yaml")), "<REPO_ROOT>", filepath.ToSlash(repoRoot))
	writeTaskStateFile(t, filepath.Join(bundleRoot, "status.yaml"), status)
	card := strings.ReplaceAll(string(readTaskStateFixture(t, "topic-layer", "card.md")), "<REPO_ROOT>", filepath.ToSlash(repoRoot))
	writeTaskStateFile(t, filepath.Join(repoRoot, "project_progress", "Tooling", "tasks", "active", "demo_topic_task.md"), card)
	return repoRoot, bundleRoot
}

func readTaskStateFixture(t *testing.T, parts ...string) []byte {
	t.Helper()

	pathParts := append([]string{"testdata"}, parts...)
	return readTaskStateFile(t, filepath.Join(pathParts...))
}

func readTaskStateFile(t *testing.T, path string) []byte {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return content
}

func readTaskStateYAMLMap(t *testing.T, path string) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := yaml.Unmarshal(readTaskStateFile(t, path), &payload); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return payload
}

func mustMap(t *testing.T, value any) map[string]any {
	t.Helper()

	payload, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %#v", value)
	}
	return payload
}
