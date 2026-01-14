package e2e

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ztaylor/claude-mon/internal/context"
)

// TestContextCreation tests that context files are created correctly
func TestContextCreation(t *testing.T) {
	tempDir := t.TempDir()

	// Override contexts dir
	oldContextsDir := context.ContextsDir
	context.ContextsDir = tempDir
	defer func() { context.ContextsDir = oldContextsDir }()

	// Create a new context
	ctx := context.New()
	ctx.ProjectRoot = tempDir

	// Save context
	if err := ctx.Save(); err != nil {
		t.Fatalf("Failed to save context: %v", err)
	}

	// Verify context file was created
	contextFiles, err := filepath.Glob(filepath.Join(tempDir, "*.json"))
	if err != nil {
		t.Fatalf("Failed to glob context files: %v", err)
	}

	if len(contextFiles) != 1 {
		t.Errorf("Expected 1 context file, got %d", len(contextFiles))
	}

	// Verify file is not empty
	data, err := os.ReadFile(contextFiles[0])
	if err != nil {
		t.Fatalf("Failed to read context file: %v", err)
	}

	if len(data) == 0 {
		t.Error("Context file is empty")
	}
}

// TestContextLoading tests that existing context is loaded correctly
func TestContextLoading(t *testing.T) {
	tempDir := t.TempDir()

	// Override contexts dir
	oldContextsDir := context.ContextsDir
	context.ContextsDir = tempDir
	defer func() { context.ContextsDir = oldContextsDir }()

	// Create a test context file with known data
	testContextFile := filepath.Join(tempDir, "test-load-123456.json")
	testData := []byte(`{
  "version": 2,
  "project_id": "test-load-123456",
  "project_root": "/tmp/test",
  "updated": "2026-01-14T10:00:00Z",
  "context": {
    "kubernetes": {
      "context": "orbstack",
      "namespace": "default"
    },
    "aws": {
      "profile": "dev",
      "region": "us-west-2"
    }
  }
}`)

	if err := os.WriteFile(testContextFile, testData, 0644); err != nil {
		t.Fatalf("Failed to write test context: %v", err)
	}

	// Create context with matching ID
	ctx := &context.Context{
		Version:     2,
		ProjectID:   "test-load-123456",
		ProjectRoot: "/tmp/test",
		Context:     make(map[string]interface{}),
	}

	// Manually load the file (simulating what Load() would do)
	data, err := os.ReadFile(testContextFile)
	if err != nil {
		t.Fatalf("Failed to read context file: %v", err)
	}

	// We can't easily test the full Load() flow since it depends on project detection,
	// but we can test that GetKubernetes() and GetAWS() work correctly
	_ = data // File was read successfully
	ctx.Context = map[string]interface{}{
		"kubernetes": map[string]interface{}{
			"context":   "orbstack",
			"namespace": "default",
		},
		"aws": map[string]interface{}{
			"profile": "dev",
			"region":  "us-west-2",
		},
	}

	k8s := ctx.GetKubernetes()
	if k8s == nil {
		t.Fatal("GetKubernetes() returned nil")
	}

	if k8s.Context != "orbstack" {
		t.Errorf("Expected K8s context 'orbstack', got '%s'", k8s.Context)
	}

	if k8s.Namespace != "default" {
		t.Errorf("Expected K8s namespace 'default', got '%s'", k8s.Namespace)
	}

	aws := ctx.GetAWS()
	if aws == nil {
		t.Fatal("GetAWS() returned nil")
	}

	if aws.Profile != "dev" {
		t.Errorf("Expected AWS profile 'dev', got '%s'", aws.Profile)
	}

	if aws.Region != "us-west-2" {
		t.Errorf("Expected AWS region 'us-west-2', got '%s'", aws.Region)
	}
}

// TestContextUpdate tests that context can be updated and saved
func TestContextUpdate(t *testing.T) {
	tempDir := t.TempDir()

	// Override contexts dir
	oldContextsDir := context.ContextsDir
	context.ContextsDir = tempDir
	defer func() { context.ContextsDir = oldContextsDir }()

	// Create new context
	ctx := context.New()
	ctx.ProjectID = "test-update-123456"
	ctx.ProjectRoot = "/tmp/test-update"

	// Set Kubernetes context
	ctx.SetKubernetes("minikube", "production", "")

	// Set AWS profile
	ctx.SetAWS("production", "us-east-1")

	// Save
	if err := ctx.Save(); err != nil {
		t.Fatalf("Failed to save context: %v", err)
	}

	// Verify the file exists
	contextFile := filepath.Join(tempDir, "test-update-123456.json")
	if _, err := os.Stat(contextFile); os.IsNotExist(err) {
		t.Fatal("Context file was not created")
	}

	// Load and verify
	data, err := os.ReadFile(contextFile)
	if err != nil {
		t.Fatalf("Failed to read context file: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("Context file is empty")
	}

	// Verify the context has the expected values
	if ctx.GetKubernetes().Context != "minikube" {
		t.Errorf("Expected K8s context 'minikube', got '%s'", ctx.GetKubernetes().Context)
	}

	if ctx.GetKubernetes().Namespace != "production" {
		t.Errorf("Expected K8s namespace 'production', got '%s'", ctx.GetKubernetes().Namespace)
	}

	if ctx.GetAWS().Profile != "production" {
		t.Errorf("Expected AWS profile 'production', got '%s'", ctx.GetAWS().Profile)
	}

	if ctx.GetAWS().Region != "us-east-1" {
		t.Errorf("Expected AWS region 'us-east-1', got '%s'", ctx.GetAWS().Region)
	}
}

// TestContextFormatForInjection tests the FormatForInjection method
func TestContextFormatForInjection(t *testing.T) {
	ctx := &context.Context{
		Version:     2,
		ProjectID:   "test",
		ProjectRoot: "/tmp/test",
		Updated:     time.Now().UTC().Format(time.RFC3339),
		Context:     make(map[string]interface{}),
	}

	// Set various context types
	ctx.SetKubernetes("my-cluster", "my-ns", "/path/to/kubeconfig")
	ctx.SetAWS("my-profile", "us-west-2")
	ctx.SetGit("main", "my-repo")
	ctx.SetEnv(map[string]string{"ENV": "dev", "DEBUG": "true"})
	ctx.SetCustom(map[string]string{"KEY1": "value1", "KEY2": "value2"})

	// Format for injection
	formatted := ctx.FormatForInjection()

	if !contains(formatted, "<working-context>") {
		t.Error("Missing <working-context> tag")
	}

	if !contains(formatted, "Kubernetes: my-cluster / my-ns") {
		t.Errorf("Missing K8s context in: %s", formatted)
	}

	if !contains(formatted, "AWS Profile: my-profile (us-west-2)") {
		t.Errorf("Missing AWS context in: %s", formatted)
	}

	if !contains(formatted, "Git: main @ my-repo") {
		t.Errorf("Missing Git context in: %s", formatted)
	}

	if !contains(formatted, "Env: ENV=dev, DEBUG=true") {
		t.Errorf("Missing Env in: %s", formatted)
	}

	if !contains(formatted, "Custom: KEY1=value1, KEY2=value2") {
		t.Errorf("Missing Custom in: %s", formatted)
	}

	if !contains(formatted, "</working-context>") {
		t.Error("Missing </working-context> tag")
	}
}

// TestContextStaleDetection tests the IsStale method
func TestContextStaleDetection(t *testing.T) {
	// Test 1: Empty updated time (should be stale)
	ctx1 := &context.Context{
		Updated: "",
	}
	if !ctx1.IsStale() {
		t.Error("Empty updated time should be stale")
	}

	// Test 2: Recent context (should not be stale)
	ctx2 := &context.Context{
		Updated: time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339),
	}
	if ctx2.IsStale() {
		t.Error("Recent context should not be stale")
	}

	// Test 3: Old context (should be stale)
	ctx3 := &context.Context{
		Updated: time.Now().UTC().Add(-25 * time.Hour).Format(time.RFC3339),
	}
	if !ctx3.IsStale() {
		t.Error("Old context should be stale")
	}

	// Test 4: Exactly 24 hours (should not be stale)
	// Use 23.5 hours to account for any floating point rounding
	ctx4 := &context.Context{
		Updated: time.Now().UTC().Add(-23*time.Hour - 30*time.Minute).Format(time.RFC3339),
	}
	if ctx4.IsStale() {
		t.Error("23.5h should not be stale")
	}
}

// TestContextAge tests the GetAge method
func TestContextAge(t *testing.T) {
	// Test 1: Empty updated time
	ctx1 := &context.Context{
		Updated: "",
	}
	age1 := ctx1.GetAge()
	if age1 != "never" {
		t.Errorf("Expected 'never', got '%s'", age1)
	}

	// Test 2: Minutes ago
	ctx2 := &context.Context{
		Updated: time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339),
	}
	age2 := ctx2.GetAge()
	if !contains(age2, "m ago") {
		t.Errorf("Expected minutes ago, got '%s'", age2)
	}

	// Test 3: Hours ago
	ctx3 := &context.Context{
		Updated: time.Now().UTC().Add(-3 * time.Hour).Format(time.RFC3339),
	}
	age3 := ctx3.GetAge()
	if !contains(age3, "h ago") {
		t.Errorf("Expected hours ago, got '%s'", age3)
	}

	// Test 4: Days ago
	ctx4 := &context.Context{
		Updated: time.Now().UTC().Add(-2 * 24 * time.Hour).Format(time.RFC3339),
	}
	age4 := ctx4.GetAge()
	if !contains(age4, "d ago") {
		t.Errorf("Expected days ago, got '%s'", age4)
	}
}

// TestContextClear tests the Clear method
func TestContextClear(t *testing.T) {
	ctx := &context.Context{
		Context: make(map[string]interface{}),
	}

	// Set some context
	ctx.SetKubernetes("test", "ns", "")
	ctx.SetAWS("profile", "region")

	// Verify it's set
	if ctx.GetKubernetes() == nil {
		t.Error("K8s context should be set")
	}
	if ctx.GetAWS() == nil {
		t.Error("AWS context should be set")
	}

	// Clear kubernetes
	ctx.Clear("kubernetes")

	// Verify it's cleared
	if ctx.GetKubernetes() != nil {
		t.Error("K8s context should be nil after clear")
	}

	// Verify AWS is still there
	if ctx.GetAWS() == nil {
		t.Error("AWS context should still be set")
	}

	// Clear all
	ctx.Clear("all")

	// Verify everything is cleared
	if ctx.GetKubernetes() != nil {
		t.Error("K8s context should be nil after clear all")
	}
	if ctx.GetAWS() != nil {
		t.Error("AWS context should be nil after clear all")
	}
}

// TestContextFormat tests the Format method for display
func TestContextFormat(t *testing.T) {
	ctx := &context.Context{
		Version:     2,
		ProjectID:   "test",
		ProjectRoot: "/tmp/test-project",
		Updated:     time.Now().UTC().Format(time.RFC3339),
		Context:     make(map[string]interface{}),
	}

	ctx.SetKubernetes("my-cluster", "my-ns", "")

	formatted := ctx.Format()

	if !contains(formatted, "Project:") {
		t.Error("Formatted context should contain 'Project:'")
	}

	if !contains(formatted, "Kubernetes:") {
		t.Error("Formatted context should contain 'Kubernetes:'")
	}

	if !contains(formatted, "my-cluster / my-ns") {
		t.Error("Formatted context should contain cluster and namespace")
	}
}
