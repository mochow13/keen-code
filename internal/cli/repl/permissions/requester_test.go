package permissions

import (
	"context"
	"testing"

	"github.com/mochow13/keen-code/internal/config"
)

func TestAutoApproveRequester_AllowsWithoutRequest(t *testing.T) {
	requester := NewAutoApproveRequester()

	allowed, err := requester.RequestPermission(context.Background(), "bash", "rm -rf tmp", "", true)
	if err != nil {
		t.Fatalf("RequestPermission() error = %v", err)
	}
	if !allowed {
		t.Fatal("expected auto-approved permission")
	}
	if requester.HasPendingRequest() {
		t.Fatal("expected no pending request")
	}

	select {
	case req := <-requester.GetRequestChan():
		t.Fatalf("expected no permission request, got %#v", req)
	default:
	}
}

func TestRequestPermission_ProjectAllowGrantsWithoutPrompt(t *testing.T) {
	perms := config.NewProjectPermissions()
	perms.Allow["bash"] = struct{}{}
	requester := NewRequester(perms)

	allowed, err := requester.RequestPermission(context.Background(), "bash", "rm -rf tmp", "", true)
	if err != nil {
		t.Fatalf("RequestPermission() error = %v", err)
	}
	if !allowed {
		t.Fatal("expected project-allow to grant without prompt")
	}
	select {
	case req := <-requester.GetRequestChan():
		t.Fatalf("expected no permission request, got %#v", req)
	default:
	}
}

func TestRequestPermission_NoOverridePromptsUser(t *testing.T) {
	requester := NewRequester(config.NewProjectPermissions())

	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan bool, 1)
	go func() {
		allowed, _ := requester.RequestPermission(ctx, "edit_file", "/tmp/foo", "/tmp/foo", false)
		resultCh <- allowed
	}()

	select {
	case req := <-requester.GetRequestChan():
		if req.ToolName != "edit_file" {
			t.Errorf("expected tool=edit_file, got %q", req.ToolName)
		}
	case <-resultCh:
		t.Fatal("expected request on channel before result")
	}

	cancel()
	if got := <-resultCh; got {
		t.Fatal("expected denial after context cancel")
	}
}
