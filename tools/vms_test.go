package tools

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
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

func TestSetVMCloudInit(t *testing.T) {
	const sshKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIP6+4/2+exampleKeyMaterial= hermes@example"
	var got *proxmox.SetVMCloudInitRequest
	mock := &mockProxmoxClient{
		setVMCloudInitFn: func(_ context.Context, node string, vmid int, req *proxmox.SetVMCloudInitRequest) error {
			if node != "pve1" || vmid != 200 {
				t.Errorf("target: got node=%q vmid=%d", node, vmid)
			}
			got = req
			return nil
		},
	}
	cs, cleanup := connectTestServer(t, mock)
	defer cleanup()

	res := callTool(t, cs, "set_vm_cloudinit", map[string]any{
		"node": "pve1", "vmid": 200, "ciuser": "hermes", "sshkeys": sshKey, "ipconfig0": "ip=dhcp",
	})
	assertResultJSON(t, res)
	if got == nil || got.CIUser != "hermes" || got.SSHKeys != sshKey || got.IPConfig0 != "ip=dhcp" {
		t.Fatalf("unexpected cloud-init request: %#v", got)
	}
}

func TestGetVMGuestNetworkInterfaces(t *testing.T) {
	t.Run("returns compact IPv4 and IPv6 data", func(t *testing.T) {
		mock := &mockProxmoxClient{
			getVMGuestNetworkInterfacesFn: func(_ context.Context, node string, vmid int) ([]proxmox.GuestNetworkInterface, error) {
				if node != "pve1" || vmid != 200 {
					t.Errorf("target: got node=%q vmid=%d", node, vmid)
				}
				return []proxmox.GuestNetworkInterface{{
					Name:       "eth0",
					MACAddress: "52:54:00:12:34:56",
					IPAddresses: []proxmox.GuestIPAddress{
						{Address: "192.0.2.10", Family: "ipv4", PrefixLength: 24},
						{Address: "2001:db8::10", Family: "ipv6", PrefixLength: 64},
					},
				}}, nil
			},
		}
		cs, cleanup := connectTestServer(t, mock)
		defer cleanup()

		res := callTool(t, cs, "get_vm_guest_network_interfaces", map[string]any{"node": "pve1", "vmid": 200})
		assertResultJSON(t, res)
		text := res.Content[0].(*mcp.TextContent).Text
		var got []proxmox.GuestNetworkInterface
		if err := json.Unmarshal([]byte(text), &got); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if len(got) != 1 || len(got[0].IPAddresses) != 2 || got[0].IPAddresses[1].Family != "ipv6" {
			t.Fatalf("unexpected result: %#v", got)
		}
	})

	t.Run("reports guest agent unavailable clearly", func(t *testing.T) {
		mock := &mockProxmoxClient{
			getVMGuestNetworkInterfacesFn: func(context.Context, string, int) ([]proxmox.GuestNetworkInterface, error) {
				return nil, errors.New("QEMU guest agent is not running")
			},
		}
		cs, cleanup := connectTestServer(t, mock)
		defer cleanup()

		res := callTool(t, cs, "get_vm_guest_network_interfaces", map[string]any{"node": "pve1", "vmid": 200})
		assertError(t, res, "QEMU guest agent is not running")
	})
}

func TestProvisioningToolSchemas(t *testing.T) {
	cs, cleanup := connectTestServer(t, &mockProxmoxClient{})
	defer cleanup()

	result, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	wantProperties := map[string][]string{
		"set_vm_cloudinit":                {"ciuser", "ipconfig0", "node", "sshkeys", "vmid"},
		"get_vm_guest_network_interfaces": {"node", "vmid"},
	}
	for toolName, want := range wantProperties {
		var tool *mcp.Tool
		for _, candidate := range result.Tools {
			if candidate.Name == toolName {
				tool = candidate
				break
			}
		}
		if tool == nil {
			t.Fatalf("tool %q not registered", toolName)
		}
		schema, ok := tool.InputSchema.(map[string]any)
		if !ok {
			t.Fatalf("tool %q schema type: %T", toolName, tool.InputSchema)
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("tool %q properties type: %T", toolName, schema["properties"])
		}
		got := make([]string, 0, len(properties))
		for name := range properties {
			got = append(got, name)
		}
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("tool %q properties: got %v, want %v", toolName, got, want)
		}
		required, ok := schema["required"].([]any)
		if !ok {
			t.Fatalf("tool %q required type: %T", toolName, schema["required"])
		}
		gotRequired := make([]string, 0, len(required))
		for _, name := range required {
			gotRequired = append(gotRequired, name.(string))
		}
		slices.Sort(gotRequired)
		if !slices.Equal(gotRequired, want) {
			t.Errorf("tool %q required: got %v, want %v", toolName, gotRequired, want)
		}
		if schema["additionalProperties"] != false {
			t.Errorf("tool %q additionalProperties: got %v, want false", toolName, schema["additionalProperties"])
		}
	}
}
