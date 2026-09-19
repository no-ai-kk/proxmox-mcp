package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gordcurrie/proxmox-mcp/internal/proxmox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestListVMs(t *testing.T) {
	t.Run("returns VMs as JSON", func(t *testing.T) {
		mock := &mockProxmoxClient{
			listVMsFn: func(_ context.Context, _ string) ([]proxmox.VM, error) {
				return []proxmox.VM{{VMID: 100, Name: "debian12", Status: "running"}}, nil
			},
		}
		cs, cleanup := connectTestServer(t, mock)
		defer cleanup()

		res := callTool(t, cs, "list_vms", map[string]any{"node": "pve1"})
		assertResultJSON(t, res)
	})

	t.Run("propagates error", func(t *testing.T) {
		mock := &mockProxmoxClient{
			listVMsFn: func(context.Context, string) ([]proxmox.VM, error) {
				return nil, errors.New("node offline")
			},
		}
		cs, cleanup := connectTestServer(t, mock)
		defer cleanup()

		res := callTool(t, cs, "list_vms", map[string]any{"node": "pve1"})
		assertError(t, res, "node offline")
	})
}

func TestGetVMStatus(t *testing.T) {
	t.Run("returns status as JSON", func(t *testing.T) {
		mock := &mockProxmoxClient{
			getVMStatusFn: func(_ context.Context, _ string, _ int) (map[string]any, error) {
				return map[string]any{"status": "running", "vmid": 100}, nil
			},
		}
		cs, cleanup := connectTestServer(t, mock)
		defer cleanup()

		res := callTool(t, cs, "get_vm_status", map[string]any{"node": "pve1", "vmid": 100})
		assertResultJSON(t, res)
	})

	t.Run("propagates error", func(t *testing.T) {
		mock := &mockProxmoxClient{
			getVMStatusFn: func(context.Context, string, int) (map[string]any, error) {
				return nil, errors.New("VM not found")
			},
		}
		cs, cleanup := connectTestServer(t, mock)
		defer cleanup()

		res := callTool(t, cs, "get_vm_status", map[string]any{"node": "pve1", "vmid": 999})
		assertError(t, res, "VM not found")
	})
}

func TestStartVM(t *testing.T) {
	t.Run("returns task ID", func(t *testing.T) {
		const upid = "UPID:pve1:000015E3:00000000:60F4B3A7:qmstart:100:root@pam:"
		mock := &mockProxmoxClient{
			startVMFn: func(context.Context, string, int) (string, error) {
				return upid, nil
			},
		}
		cs, cleanup := connectTestServer(t, mock)
		defer cleanup()

		res := callTool(t, cs, "start_vm", map[string]any{"node": "pve1", "vmid": 100})
		assertResultJSON(t, res)
	})

	t.Run("propagates error", func(t *testing.T) {
		mock := &mockProxmoxClient{
			startVMFn: func(context.Context, string, int) (string, error) {
				return "", errors.New("VM already running")
			},
		}
		cs, cleanup := connectTestServer(t, mock)
		defer cleanup()

		res := callTool(t, cs, "start_vm", map[string]any{"node": "pve1", "vmid": 100})
		assertError(t, res, "VM already running")
	})
}

func TestGetVMConfig(t *testing.T) {
	t.Run("returns config as JSON", func(t *testing.T) {
		mock := &mockProxmoxClient{
			getVMConfigFn: func(context.Context, string, int) (map[string]any, error) {
				return map[string]any{"cores": 2, "memory": 2048}, nil
			},
		}
		cs, cleanup := connectTestServer(t, mock)
		defer cleanup()

		res := callTool(t, cs, "get_vm_config", map[string]any{"node": "pve1", "vmid": 100})
		assertResultJSON(t, res)
	})

	t.Run("propagates error", func(t *testing.T) {
		mock := &mockProxmoxClient{
			getVMConfigFn: func(context.Context, string, int) (map[string]any, error) {
				return nil, errors.New("config read failed")
			},
		}
		cs, cleanup := connectTestServer(t, mock)
		defer cleanup()

		res := callTool(t, cs, "get_vm_config", map[string]any{"node": "pve1", "vmid": 100})
		assertError(t, res, "config read failed")
	})
}

func TestCreateVM(t *testing.T) {
	t.Run("passes pool to client", func(t *testing.T) {
		const upid = "UPID:pve1:000015E3:00000000:60F4B3A7:qmcreate:200:root@pam:"
		mock := &mockProxmoxClient{
			createVMFn: func(_ context.Context, node string, req *proxmox.CreateVMRequest) (string, error) {
				if node != "pve1" {
					t.Errorf("node: got %q, want pve1", node)
				}
				if req.Pool != "HermesManaged" {
					t.Errorf("pool: got %q, want HermesManaged", req.Pool)
				}
				if req.Memory != 1024 || req.Cores != 1 {
					t.Errorf("resources: got memory=%d cores=%d, want memory=1024 cores=1", req.Memory, req.Cores)
				}
				return upid, nil
			},
		}
		cs, cleanup := connectTestServer(t, mock)
		defer cleanup()

		res := callTool(t, cs, "create_vm", map[string]any{
			"node": "pve1", "vmid": 200, "name": "test-vm", "pool": "HermesManaged",
			"memory": 1024, "cores": 1,
		})
		assertResultJSON(t, res)
	})

	t.Run("propagates error", func(t *testing.T) {
		mock := &mockProxmoxClient{
			createVMFn: func(context.Context, string, *proxmox.CreateVMRequest) (string, error) {
				return "", errors.New("permission denied")
			},
		}
		cs, cleanup := connectTestServer(t, mock)
		defer cleanup()

		res := callTool(t, cs, "create_vm", map[string]any{"node": "pve1", "vmid": 200})
		assertError(t, res, "permission denied")
	})
}

func TestCloneVM(t *testing.T) {
	t.Run("passes explicit full mode and pool to client", func(t *testing.T) {
		const upid = "UPID:pve1:000015E3:00000000:60F4B3A7:qmclone:201:root@pam:"
		mock := &mockProxmoxClient{
			cloneVMFn: func(_ context.Context, node string, vmid int, req *proxmox.CloneVMRequest) (string, error) {
				if node != "pve1" || vmid != 901 || req.NewID != 201 || req.Pool != "HermesManaged" || !req.Full {
					t.Errorf("clone request: node=%q vmid=%d req=%+v", node, vmid, req)
				}
				return upid, nil
			},
		}
		cs, cleanup := connectTestServer(t, mock)
		defer cleanup()

		res := callTool(t, cs, "clone_vm", map[string]any{
			"node": "pve1", "vmid": 901, "newid": 201, "pool": "HermesManaged", "full": true,
		})
		assertResultJSON(t, res)
	})

	t.Run("passes explicit linked mode", func(t *testing.T) {
		mock := &mockProxmoxClient{
			cloneVMFn: func(_ context.Context, _ string, _ int, req *proxmox.CloneVMRequest) (string, error) {
				if req.Full {
					t.Errorf("full: got true, want false")
				}
				return "upid", nil
			},
		}
		cs, cleanup := connectTestServer(t, mock)
		defer cleanup()

		res := callTool(t, cs, "clone_vm", map[string]any{
			"node": "pve1", "vmid": 901, "newid": 202, "pool": "HermesManaged", "full": false,
		})
		assertResultJSON(t, res)
	})

	t.Run("rejects omitted clone mode", func(t *testing.T) {
		called := false
		mock := &mockProxmoxClient{
			cloneVMFn: func(context.Context, string, int, *proxmox.CloneVMRequest) (string, error) {
				called = true
				return "upid", nil
			},
		}
		cs, cleanup := connectTestServer(t, mock)
		defer cleanup()

		params := &mcp.CallToolParams{
			Name: "clone_vm",
			Arguments: map[string]any{
				"node": "pve1", "vmid": 901, "newid": 203, "pool": "HermesManaged",
			},
		}
		if _, err := cs.CallTool(context.Background(), params); err == nil || !strings.Contains(err.Error(), "missing properties: [\"full\"]") {
			t.Fatalf("expected omitted full to be rejected by schema, got %v", err)
		}
		if called {
			t.Fatal("clone client was called without an explicit clone mode")
		}
	})
}
